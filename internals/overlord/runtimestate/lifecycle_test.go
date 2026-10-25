package runtimestate

import (
	"bytes"
	"context"
	"errors"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/overlord/state"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

type lifecycleSuite struct{}

var _ = tc.InternalSuite(&lifecycleSuite{})

// queueConfig adds an independent configuration task after this installation.
func (f *hookFixture) queueConfig(value string) *state.Task {
	f.st.Lock()
	defer f.st.Unlock()
	fetch := f.st.NewTask("fetch-config", "Completed config snapshot")
	fetch.Set("charm-install-path", "/original/charm")
	fetch.Set("charm-sha256", "charm")
	fetch.Set("install-task", f.task.ID())
	fetch.Set("hook-config", map[string]any{"status": value})
	fetch.WaitFor(f.task)
	fetch.SetStatus(state.DoneStatus)
	config := HookTask(f.st, fetch, "config-changed")
	change := f.st.NewChange("config", "Configure charm")
	change.AddTask(fetch)
	change.AddTask(config)
	return config
}

// settleHooks runs dependencies and dynamically appended lifecycle tasks to completion.
func (f *hookFixture) settleHooks(c *tc.C) {
	for range 7 {
		f.run(c)
	}
}

// TestConfigStartConfigOrder verifies exactly one start uses the first config snapshot.
func (*lifecycleSuite) TestConfigStartConfigOrder(c *tc.C) {
	var calls []charmRuntime.Hook
	fixture := newHookFixture(c, nil, runtimeFunc(func(_ context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		calls = append(calls, hook)
		return charmRuntime.Result{}, nil
	}))
	fixture.run(c)
	fixture.queueConfig("first")
	fixture.queueConfig("second")
	fixture.settleHooks(c)
	c.Assert(calls, tc.HasLen, 4)

	c.Check(calls[0].Name, tc.Equals, "install")
	c.Check(calls[1].Name, tc.Equals, "config-changed")
	c.Check(calls[2].Name, tc.Equals, "start")
	c.Check(calls[2].Config, tc.DeepEquals, calls[1].Config)
	c.Check(calls[3].Name, tc.Equals, "config-changed")

	fixture.st.Lock()
	defer fixture.st.Unlock()
	current, err := fixture.manager.readLifecycle(fixture.task.ID())
	c.Assert(err, tc.IsNil)

	c.Check(current.Installed, tc.Equals, true)
	c.Check(current.Started, tc.Equals, true)
	c.Check(current.StartTask, tc.Not(tc.Equals), "")
}

// TestFailureNeverAcknowledgesStartup verifies failed config cannot start and a
// failed start blocks subsequent config even when it arrives in another change.
func (*lifecycleSuite) TestFailureNeverAcknowledgesStartup(c *tc.C) {
	for _, failed := range []string{"config-changed", "start"} {
		var calls []string
		fixture := newHookFixture(c, nil, runtimeFunc(func(_ context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
			calls = append(calls, hook.Name)
			if hook.Name == failed {
				return charmRuntime.Result{}, errors.New("intentional failure")
			}
			return charmRuntime.Result{}, nil
		}))
		fixture.run(c)
		fixture.queueConfig("first")
		fixture.settleHooks(c)
		later := fixture.queueConfig("later")
		fixture.settleHooks(c)
		fixture.st.Lock()
		current, err := fixture.manager.readLifecycle(fixture.task.ID())
		status := later.Status()
		fixture.st.Unlock()
		c.Assert(err, tc.IsNil)

		c.Check(current.Started, tc.Equals, false)
		c.Check(status, tc.Equals, state.DoStatus)

		if failed == "config-changed" {
			c.Check(calls, tc.DeepEquals, []string{"install", "config-changed"})
			c.Check(current.StartTask, tc.Equals, "")
		} else {
			c.Check(calls, tc.DeepEquals, []string{"install", "config-changed", "start"})
		}
	}
}

// TestStartupSurvivesEveryCheckpoint restores real checkpoints on both sides of
// config/start commits, proving start creation and acknowledgement are idempotent.
func (*lifecycleSuite) TestStartupSurvivesEveryCheckpoint(c *tc.C) {
	backend := &checkpointBackend{}
	fixture := newHookFixture(c, backend, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		return charmRuntime.Result{}, nil
	}))
	fixture.run(c)
	backend.snapshots = nil
	config := fixture.queueConfig("initial")
	fixture.settleHooks(c)
	for _, snapshot := range backend.snapshots {
		var calls []string
		restored := restoreHookFixture(c, snapshot, config.ID(), runtimeFunc(func(_ context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
			calls = append(calls, hook.Name)
			return charmRuntime.Result{}, nil
		}))
		restored.st.Lock()
		interrupted := false
		for _, task := range restored.st.Tasks() {
			if task.Kind() != RunHookTask {
				continue
			}
			progress, err := readHookState(task)
			if err == nil && progress.Step == hookPending {
				interrupted = true
			}
		}
		restored.st.Unlock()
		restored.settleHooks(c)
		restored.st.Lock()
		starts := 0
		for _, task := range restored.st.Tasks() {
			var name string
			if task.Get("hook-name", &name) == nil && name == "start" {
				starts++
			}
		}
		current, err := restored.manager.readLifecycle(fixture.task.ID())
		restored.st.Unlock()
		c.Assert(err, tc.IsNil)

		c.Check(current.Started, tc.Equals, !interrupted)
		c.Check(starts <= 1, tc.Equals, true)

		startCalls := 0
		for _, name := range calls {
			if name == "start" {
				startCalls++
			}
		}
		c.Check(startCalls <= 1, tc.Equals, true)

		if current.Started {
			c.Check(starts, tc.Equals, 1)
		}
	}
}

// TestStartedStatePreventsStartAfterRestart preserves startup across new config notifications.
func (*lifecycleSuite) TestStartedStatePreventsStartAfterRestart(c *tc.C) {
	fixture := newHookFixture(c, nil, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		return charmRuntime.Result{}, nil
	}))
	fixture.run(c)
	fixture.queueConfig("initial")
	fixture.settleHooks(c)
	fixture.st.Lock()
	data, err := fixture.st.MarshalJSON()
	fixture.st.Unlock()
	c.Assert(err, tc.IsNil)

	var calls []string
	restored := restoreHookFixture(c, data, fixture.task.ID(), runtimeFunc(func(_ context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		calls = append(calls, hook.Name)
		return charmRuntime.Result{}, nil
	}))
	restored.queueConfig("after-restart")
	restored.settleHooks(c)
	c.Check(calls, tc.DeepEquals, []string{"config-changed"})
}

// TestLegacyConfigDependencyFindsInstallation preserves identity for older queued tasks.
func (*lifecycleSuite) TestLegacyConfigDependencyFindsInstallation(c *tc.C) {
	fixture := newHookFixture(c, nil, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		return charmRuntime.Result{}, nil
	}))
	fixture.run(c)
	config := fixture.queueConfig("initial")
	fixture.st.Lock()
	config.Set("hook-install-task", nil)
	fixture.task.Set("hook-install-task", nil)
	data, err := fixture.st.MarshalJSON()
	fixture.st.Unlock()
	c.Assert(err, tc.IsNil)

	restoredState, err := state.ReadState(nil, bytes.NewReader(data))
	c.Assert(err, tc.IsNil)

	restoredState.Lock()
	id, err := installationID(restoredState.Task(config.ID()))
	restoredState.Unlock()
	c.Assert(err, tc.IsNil)

	c.Check(id, tc.Equals, fixture.task.ID())
}

// TestInvalidLifecycleCannotCommitStart rejects orphan hooks and incomplete installs.
func (*lifecycleSuite) TestInvalidLifecycleCannotCommitStart(c *tc.C) {
	for _, invalid := range []string{"orphan", "incomplete", "unexpected-start", "corrupt"} {
		fixture := newHookFixture(c, nil, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
			return charmRuntime.Result{}, nil
		}))
		fixture.run(c)
		config := fixture.queueConfig("initial")
		fixture.st.Lock()
		name := "config-changed"
		switch invalid {
		case "orphan":
			config.Set("hook-install-task", "missing")
		case "incomplete":
			pending := fixture.st.NewTask(RunHookTask, "Incomplete install")
			pending.Set("hook-name", "install")
			fixture.st.NewChange("install", "Incomplete installation").AddTask(pending)
			config.Set("hook-install-task", pending.ID())
		case "unexpected-start":
			name = "start"
		case "corrupt":
			fixture.st.Set("charm-lifecycle-"+fixture.task.ID(), 42)
		}
		err := fixture.manager.commitLifecycle(config, name)
		count := len(config.Change().Tasks())
		fixture.st.Unlock()
		c.Check(err, tc.NotNil, tc.Commentf("invalid %s", invalid))
		c.Check(count, tc.Equals, 2)
	}
}
