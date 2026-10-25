package runtimestate

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/overlord/state"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

// Test connects the tc suites to the Go test runner.
func Test(t *testing.T) { tc.InternalTestingT(t) }

type managerSuite struct{}

var _ = tc.InternalSuite(&managerSuite{})

type runtimeFunc func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error)

// RunHook adapts a test function to the runtime interface.
func (f runtimeFunc) RunHook(ctx context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
	return f(ctx, hook)
}

// TestInstallTask verifies dependency ordering, hook output, and persisted hook results.
func (s *managerSuite) TestInstallTask(c *tc.C) { s.testInstallTask(c, false) }

// TestInstallTaskFailure verifies that a runtime error fails the hook task.
func (s *managerSuite) TestInstallTaskFailure(c *tc.C) { s.testInstallTask(c, true) }

// testInstallTask exercises task execution against a controlled runtime.
func (*managerSuite) testInstallTask(c *tc.C, fail bool) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	calls := 0
	var gotHook charmRuntime.Hook
	NewManager(st, runner, runtimeFunc(func(ctx context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		calls++
		gotHook = hook
		fmt.Fprintln(hook.Stdout, "hook output")
		result := charmRuntime.Result{Status: &charmRuntime.Status{Status: "maintenance", Message: "installing"}}
		if fail {
			return result, errors.New("hook failed")
		}
		return result, nil
	}))
	st.Lock()
	install := st.NewTask("install-charm", "Unpack charm")
	install.Set("charm-install-path", "/installed/charm")
	task := InstallTask(st, install)
	change := st.NewChange("install", "Install charm")
	change.AddTask(install)
	change.AddTask(task)
	st.Unlock()
	c.Assert(runner.Ensure(), tc.IsNil)

	runner.Wait()
	c.Check(calls, tc.Equals, 0)
	st.Lock()
	install.SetStatus(state.DoneStatus)
	st.Unlock()
	c.Assert(runner.Ensure(), tc.IsNil)

	runner.Wait()
	c.Check(calls, tc.Equals, 1)
	c.Check(gotHook.CharmDir, tc.Equals, "/installed/charm")
	c.Check(gotHook.Name, tc.Equals, "install")
	st.Lock()
	defer st.Unlock()
	want := state.DoneStatus
	if fail {
		want = state.ErrorStatus
	}
	c.Check(task.Status(), tc.Equals, want)
	var result charmRuntime.Result
	c.Assert(task.Get("hook-result", &result), tc.IsNil)

	c.Assert(result.Status, tc.NotNil)

	c.Check(result.Status.Message, tc.Equals, "installing")
	c.Check(len(task.Log()) > 0, tc.Equals, true)
}
