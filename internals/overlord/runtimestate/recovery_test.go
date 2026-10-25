package runtimestate

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/overlord/state"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

type recoverySuite struct{}

var _ = tc.InternalSuite(&recoverySuite{})

// checkpointBackend captures real Unlock checkpoints for deterministic crash-boundary recovery.
type checkpointBackend struct{ snapshots [][]byte }

// Checkpoint retains immutable copies of every durable transition.
func (b *checkpointBackend) Checkpoint(data []byte) error {
	b.snapshots = append(b.snapshots, bytes.Clone(data))
	return nil
}

// EnsureBefore leaves scheduling under test control.
func (*checkpointBackend) EnsureBefore(time.Duration) {}

// NeedsCheckpoint enables production-style checkpointing on every modified unlock.
func (*checkpointBackend) NeedsCheckpoint() bool { return true }

// restoreHookFixture loads a checkpoint into a fresh runner and manager, as startup does.
func restoreHookFixture(c *tc.C, data []byte, id string, runtime charmRuntime.Runtime) *hookFixture {
	st, err := state.ReadState(nil, bytes.NewReader(data))
	c.Assert(err, tc.IsNil)

	runner := state.NewTaskRunner(st)
	c.Cleanup(runner.Stop)
	manager := NewManager(st, runner, runtime)
	st.Lock()
	task := st.Task(id)
	st.Unlock()
	c.Assert(task, tc.NotNil)

	return &hookFixture{st: st, runner: runner, manager: manager, task: task}
}

// TestEveryCheckpointRecovers verifies queued, interrupted, executed, and committed boundaries.
func (*recoverySuite) TestEveryCheckpointRecovers(c *tc.C) {
	backend := &checkpointBackend{}
	originalCalls := 0
	fixture := newHookFixture(c, backend, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		originalCalls++
		return charmRuntime.Result{Status: &charmRuntime.Status{Status: "active", Message: "original result"}}, nil
	}))
	fixture.run(c)
	c.Assert(originalCalls, tc.Equals, 1)

	seen := make(map[hookStep]bool)
	for _, snapshot := range backend.snapshots {
		// Manager construction may checkpoint an empty state before task creation.
		checkpoint, err := state.ReadState(nil, bytes.NewReader(snapshot))
		c.Assert(err, tc.IsNil)

		checkpoint.Lock()
		present := checkpoint.Task(fixture.task.ID()) != nil
		checkpoint.Unlock()
		if !present {
			continue
		}
		calls, leadershipCalls := 0, 0
		restored := restoreHookFixture(c, snapshot, fixture.task.ID(), runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
			calls++
			return charmRuntime.Result{}, nil
		}))
		restored.manager.Leadership = func(context.Context, *state.Task) (bool, error) {
			leadershipCalls++
			return false, nil
		}
		restored.st.Lock()
		before, err := readHookState(restored.task)
		alreadyDone := restored.task.Status() == state.DoneStatus
		restored.st.Unlock()
		c.Assert(err, tc.IsNil)

		seen[before.Step] = true
		restored.run(c)
		restored.st.Lock()
		status := restored.task.Status()
		after, err := readHookState(restored.task)
		restored.st.Unlock()
		c.Assert(err, tc.IsNil)

		if before.Step == hookQueued && !alreadyDone {
			c.Check(calls, tc.Equals, 1)
			c.Check(status, tc.Equals, state.DoneStatus)
		} else if before.Step == hookPending {
			c.Check(calls, tc.Equals, 0)
			c.Check(status, tc.Equals, state.ErrorStatus)
		} else {
			c.Check(calls, tc.Equals, 0)
			c.Check(leadershipCalls, tc.Equals, 0)
			c.Check(status, tc.Equals, state.DoneStatus)
			c.Assert(after.Result.Status, tc.NotNil)

			c.Check(after.Result.Status.Message, tc.Equals, "original result")
		}
	}
	c.Check(seen, tc.DeepEquals, map[hookStep]bool{hookQueued: true, hookPending: true, hookDone: true, hookCommitted: true})
}

// TestCommitUsesSavedInvocation ignores changed predecessors and unavailable live APIs.
func (*recoverySuite) TestCommitUsesSavedInvocation(c *tc.C) {
	fixture := newHookFixture(c, nil, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		return charmRuntime.Result{}, errors.New("commit reran charm")
	}))
	fixture.st.Lock()
	fixture.task.Set("hook-state", hookState{Step: hookDone, Invocation: invocation{
		Dir: "/saved/charm", Name: "install", Config: map[string]any{"status": "saved"}, Leader: true,
	}, Result: charmRuntime.Result{Status: &charmRuntime.Status{Status: "active"}}})
	previous := fixture.task.WaitTasks()[0]
	previous.Set("charm-install-path", "/changed/charm")
	previous.Set("hook-config", map[string]any{"status": "changed"})
	fixture.st.Unlock()
	fixture.manager.Leadership = func(context.Context, *state.Task) (bool, error) {
		return false, errors.New("leadership API is offline")
	}
	fixture.run(c)
	fixture.st.Lock()
	defer fixture.st.Unlock()
	var path string
	var config map[string]any
	c.Assert(fixture.task.Get("charm-install-path", &path), tc.IsNil)
	c.Assert(fixture.task.Get("hook-config", &config), tc.IsNil)

	c.Check(path, tc.Equals, "/saved/charm")
	c.Check(config["status"], tc.Equals, "saved")
	c.Check(fixture.task.Status(), tc.Equals, state.DoneStatus)
}

// TestLegacyInFlightTaskIsNotReplayed treats historical partial results as ambiguous.
func (*recoverySuite) TestLegacyInFlightTaskIsNotReplayed(c *tc.C) {
	fixture := newHookFixture(c, nil, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		return charmRuntime.Result{}, nil
	}))
	fixture.st.Lock()
	fixture.task.Set("hook-state", nil)
	fixture.task.Set("hook-result", charmRuntime.Result{Status: &charmRuntime.Status{Status: "active"}})
	fixture.task.SetStatus(state.DoingStatus)
	data, err := fixture.st.MarshalJSON()
	fixture.st.Unlock()
	c.Assert(err, tc.IsNil)

	calls := 0
	restored := restoreHookFixture(c, data, fixture.task.ID(), runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		calls++
		return charmRuntime.Result{}, nil
	}))
	restored.run(c)
	c.Check(calls, tc.Equals, 0)

	restored.st.Lock()
	defer restored.st.Unlock()
	c.Check(restored.task.Status(), tc.Equals, state.ErrorStatus)
}

// TestFailureGatesOnlyItsCharm prevents later independent config changes bypassing an error.
func (*recoverySuite) TestFailureGatesOnlyItsCharm(c *tc.C) {
	fixture := newHookFixture(c, nil, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		return charmRuntime.Result{}, errors.New("install failed")
	}))
	fixture.run(c)
	fixture.st.Lock()
	previous := fixture.st.NewTask("unpack", "Independent completed input")
	previous.Set("charm-sha256", "charm")
	previous.Set("charm-install-path", "/original/charm")
	previous.SetStatus(state.DoneStatus)
	blocked := HookTask(fixture.st, previous, "config-changed")
	change := fixture.st.NewChange("config", "Another config notification")
	change.AddTask(blocked)
	other := HookTask(fixture.st, previous, "install")
	other.Set("charm-sha256", "unrelated")
	change = fixture.st.NewChange("install", "Unrelated charm")
	change.AddTask(other)
	fixture.st.Unlock()
	fixture.run(c)
	fixture.st.Lock()
	defer fixture.st.Unlock()
	c.Check(blocked.Status(), tc.Equals, state.DoStatus)
	c.Check(other.Status(), tc.Equals, state.ErrorStatus)
}
