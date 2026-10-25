package legacy

import (
	"context"
	"errors"
	"os"

	"github.com/canonical/tc"

	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

type runnerSuite struct{}

var _ = tc.InternalSuite(&runnerSuite{})

type executorFunc func(context.Context, Command) error

// Execute lets runner tests observe prepared processes without launching a shell.
func (f executorFunc) Execute(ctx context.Context, command Command) error { return f(ctx, command) }

// TestExecutionFailurePreservesResult checks executor errors retain their cause and session cleanup precedes result return.
func (*runnerSuite) TestExecutionFailurePreservesResult(c *tc.C) {
	dir := charm(c, "exit 0", "hooks/install")
	ctx, err := NewHookContext(dir, nil)
	c.Assert(err, tc.IsNil)

	exe, err := os.Executable()
	c.Assert(err, tc.IsNil)

	failure := errors.New("executor failed")
	var got Command
	executor := executorFunc(func(_ context.Context, command Command) error {
		got = command
		ctx.SetStatus(charmRuntime.Status{Status: "blocked", Message: "installation failed"})
		return failure
	})
	runner := NewRunner(ctx, dir, exe, executor)
	result, err := runner.RunHook(context.Background(), charmRuntime.Hook{Name: "install"})
	c.Check(err, tc.ErrorIs, failure)
	c.Assert(result.Status, tc.NotNil)

	c.Check(result.Status.Status, tc.Equals, "blocked")
	values := environmentValues(c, got.Env)
	c.Check(got.Dir, tc.Equals, dir)
	c.Check(values["JUJU_CONTEXT_ID"], tc.Equals, ctx.ID())
	_, err = os.Stat(values["JUJU_AGENT_SOCKET_ADDRESS"])
	c.Check(os.IsNotExist(err), tc.Equals, true)
}

// TestMissingAndInvalidHandlersNeverExecute checks discovery and preparation failures cannot invoke the executor or change status.
func (*runnerSuite) TestMissingAndInvalidHandlersNeverExecute(c *tc.C) {
	for _, handler := range []string{"", "hooks/install"} {
		dir := charm(c, "exit 0", handler)
		ctx, err := NewHookContext(dir, nil)
		c.Assert(err, tc.IsNil)

		calls := 0
		executor := executorFunc(func(context.Context, Command) error { calls++; return nil })
		runner := NewRunner(ctx, dir, "", executor)
		result, err := runner.RunHook(context.Background(), charmRuntime.Hook{Name: "install"})
		if handler == "" {
			c.Check(err, tc.IsNil)
		} else {
			c.Check(err, tc.ErrorMatches, "hook tool executable is required")
		}
		c.Check(calls, tc.Equals, 0)
		c.Check(result.Status, tc.IsNil)
	}
}

// TestDefaultExecutor checks a nil executor selects real local process execution.
func (*runnerSuite) TestDefaultExecutor(c *tc.C) {
	ctx, err := NewHookContext(charm(c, "", ""), nil)
	c.Assert(err, tc.IsNil)

	runner := NewRunner(ctx, "/charm", "/binary", nil)
	_, ok := runner.executor.(processExecutor)
	c.Check(ok, tc.Equals, true)
}

// TestInvalidHandlerCannotExecute checks discovery errors cannot run the executor or change the context.
func (*runnerSuite) TestInvalidHandlerCannotExecute(c *tc.C) {
	dir := charm(c, "exit 0", "dispatch")
	c.Assert(os.Chmod(dir+"/dispatch", 0644), tc.IsNil)

	ctx, err := NewHookContext(dir, nil)
	c.Assert(err, tc.IsNil)

	calls := 0
	executor := executorFunc(func(context.Context, Command) error { calls++; return nil })
	result, err := NewRunner(ctx, dir, "/binary", executor).RunHook(context.Background(), charmRuntime.Hook{Name: "install"})
	c.Check(err, tc.ErrorMatches, `hook handler "dispatch" is not executable`)
	c.Check(calls, tc.Equals, 0)
	c.Check(result.Status, tc.IsNil)
}
