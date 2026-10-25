package runtimestate

import (
	"context"
	"errors"
	"time"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/overlord/state"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

type hooksSuite struct{}

var _ = tc.InternalSuite(&hooksSuite{})

// hookFixture owns the runner and one ready install-hook task.
type hookFixture struct {
	st      *state.State
	runner  *state.TaskRunner
	manager *Manager
	task    *state.Task
}

// newHookFixture creates a ready hook without involving charm download services.
func newHookFixture(c *tc.C, backend state.Backend, runtime charmRuntime.Runtime) *hookFixture {
	st := state.New(backend)
	runner := state.NewTaskRunner(st)
	c.Cleanup(runner.Stop)
	manager := NewManager(st, runner, runtime)
	st.Lock()
	previous := st.NewTask("unpack", "Unpack charm")
	previous.Set("charm-sha256", "charm")
	previous.Set("charm-install-path", "/original/charm")
	previous.Set("hook-config", map[string]any{"status": "original"})
	previous.SetStatus(state.DoneStatus)
	task := InstallTask(st, previous)
	change := st.NewChange("install", "Install charm")
	change.AddTask(previous)
	change.AddTask(task)
	st.Unlock()
	return &hookFixture{st: st, runner: runner, manager: manager, task: task}
}

// run schedules available work and joins handlers before assertions inspect their results.
func (f *hookFixture) run(c *tc.C) {
	c.Assert(f.runner.Ensure(), tc.IsNil)

	f.runner.Wait()
}

// TestExecutionIsCheckpointedBeforeRuntime verifies the runtime can only see durable Pending.
func (*hooksSuite) TestExecutionIsCheckpointedBeforeRuntime(c *tc.C) {
	backend := &checkpointBackend{}
	var fixture *hookFixture
	fixture = newHookFixture(c, backend, runtimeFunc(func(ctx context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		fixture.st.Lock()
		progress, err := readHookState(fixture.task)
		fixture.st.Unlock()
		if err != nil || progress.Step != hookPending {
			return charmRuntime.Result{}, errors.New("missing execution intent")
		}
		if len(backend.snapshots) == 0 {
			return charmRuntime.Result{}, errors.New("missing durable checkpoint")
		}
		return charmRuntime.Result{Status: &charmRuntime.Status{Status: "active", Message: "saved"}}, nil
	}))
	fixture.run(c)
	fixture.st.Lock()
	defer fixture.st.Unlock()
	progress, err := readHookState(fixture.task)
	c.Assert(err, tc.IsNil)

	c.Check(progress.Step, tc.Equals, hookCommitted)
	c.Check(fixture.task.Status(), tc.Equals, state.DoneStatus)
	c.Check(progress.Result.Status.Message, tc.Equals, "saved")
	c.Check(progress.Invocation.Config["status"], tc.Equals, "original")
}

// TestLeadershipRetryDoesNotRecordExecution keeps queued work safe to reschedule.
func (*hooksSuite) TestLeadershipRetryDoesNotRecordExecution(c *tc.C) {
	calls := 0
	fixture := newHookFixture(c, nil, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		calls++
		return charmRuntime.Result{}, nil
	}))
	attempts := 0
	fixture.manager.Leadership = func(context.Context, *state.Task) (bool, error) {
		attempts++
		if attempts == 1 {
			return false, &state.Retry{}
		}
		return true, nil
	}
	fixture.run(c)
	fixture.st.Lock()
	progress, err := readHookState(fixture.task)
	fixture.st.Unlock()
	c.Assert(err, tc.IsNil)

	c.Check(progress.Step, tc.Equals, hookQueued)
	c.Check(calls, tc.Equals, 0)

	fixture.run(c)
	c.Check(calls, tc.Equals, 1)
}

// TestSkipCommitsWithoutExecution preserves the existing stale-election behaviour.
func (*hooksSuite) TestSkipCommitsWithoutExecution(c *tc.C) {
	fixture := newHookFixture(c, nil, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		return charmRuntime.Result{}, errors.New("skipped hook was executed")
	}))
	fixture.manager.Leadership = func(context.Context, *state.Task) (bool, error) {
		return false, ErrSkipHook
	}
	fixture.run(c)
	fixture.st.Lock()
	defer fixture.st.Unlock()
	progress, err := readHookState(fixture.task)
	c.Assert(err, tc.IsNil)

	c.Check(fixture.task.Status(), tc.Equals, state.DoneStatus)
	c.Check(progress.Step, tc.Equals, hookCommitted)
}

// TestGracefulCancellationRequeuesOnlyAfterRuntimeStops retains retryable shutdown work.
func (*hooksSuite) TestGracefulCancellationRequeuesOnlyAfterRuntimeStops(c *tc.C) {
	entered := make(chan struct{})
	fixture := newHookFixture(c, nil, runtimeFunc(func(ctx context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		close(entered)
		<-ctx.Done()
		return charmRuntime.Result{}, ctx.Err()
	}))
	c.Assert(fixture.runner.Ensure(), tc.IsNil)

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		c.Fatal("runtime did not start")
	}
	fixture.runner.Stop()
	fixture.st.Lock()
	progress, err := readHookState(fixture.task)
	data, marshalErr := fixture.st.MarshalJSON()
	fixture.st.Unlock()
	c.Assert(err, tc.IsNil)
	c.Assert(marshalErr, tc.IsNil)

	c.Check(progress.Step, tc.Equals, hookQueued)
	calls := 0
	restored := restoreHookFixture(c, data, fixture.task.ID(), runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		calls++
		return charmRuntime.Result{}, nil
	}))
	restored.run(c)
	c.Check(calls, tc.Equals, 1)
}

// TestInvalidInputsNeverReachRuntime verifies corrupted task data fails closed.
func (*hooksSuite) TestInvalidInputsNeverReachRuntime(c *tc.C) {
	for _, invalid := range []string{"step", "state", "directory", "config", "name", "dependency", "completed"} {
		calls := 0
		fixture := newHookFixture(c, nil, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
			calls++
			return charmRuntime.Result{}, nil
		}))
		fixture.st.Lock()
		switch invalid {
		case "step":
			fixture.task.Set("hook-state", hookState{Step: "unknown"})
		case "state":
			fixture.task.Set("hook-state", 42)
		case "directory":
			fixture.task.WaitTasks()[0].Set("charm-install-path", "")
		case "config":
			fixture.task.WaitTasks()[0].Set("hook-config", 42)
		case "name":
			fixture.task.Set("hook-name", "")
		case "dependency":
			fixture.task.WaitFor(fixture.st.NewTask("extra", "Extra dependency"))
			fixture.task.WaitTasks()[1].SetStatus(state.DoneStatus)
		case "completed":
			fixture.task.Set("hook-state", hookState{Step: hookDone})
		}
		fixture.st.Unlock()
		fixture.run(c)
		c.Check(calls, tc.Equals, 0, tc.Commentf("invalid %s", invalid))

		fixture.st.Lock()
		status := fixture.task.Status()
		fixture.st.Unlock()
		c.Check(status, tc.Equals, state.ErrorStatus, tc.Commentf("invalid %s", invalid))
	}
}

// TestCancellationDuringPreparationNeverStartsRuntime leaves unexecuted work queued.
func (*hooksSuite) TestCancellationDuringPreparationNeverStartsRuntime(c *tc.C) {
	entered := make(chan struct{})
	calls := 0
	fixture := newHookFixture(c, nil, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		calls++
		return charmRuntime.Result{}, nil
	}))
	fixture.manager.Leadership = func(ctx context.Context, _ *state.Task) (bool, error) {
		close(entered)
		<-ctx.Done()
		return false, nil
	}
	c.Assert(fixture.runner.Ensure(), tc.IsNil)

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		c.Fatal("preparation did not start")
	}
	fixture.runner.Stop()
	c.Check(calls, tc.Equals, 0)

	fixture.st.Lock()
	defer fixture.st.Unlock()
	progress, err := readHookState(fixture.task)
	c.Assert(err, tc.IsNil)

	c.Check(progress.Step, tc.Equals, hookQueued)
}

// TestUnrecognizedShutdownFailureRemainsPending avoids treating arbitrary failures
// as evidence that the runtime was safely canceled.
func (*hooksSuite) TestUnrecognizedShutdownFailureRemainsPending(c *tc.C) {
	entered := make(chan struct{})
	fixture := newHookFixture(c, nil, runtimeFunc(func(ctx context.Context, _ charmRuntime.Hook) (charmRuntime.Result, error) {
		close(entered)
		<-ctx.Done()
		return charmRuntime.Result{}, errors.New("unrelated runtime failure")
	}))
	c.Assert(fixture.runner.Ensure(), tc.IsNil)

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		c.Fatal("runtime did not start")
	}
	fixture.runner.Stop()
	fixture.st.Lock()
	defer fixture.st.Unlock()
	progress, err := readHookState(fixture.task)
	c.Assert(err, tc.IsNil)

	c.Check(progress.Step, tc.Equals, hookPending)
}
