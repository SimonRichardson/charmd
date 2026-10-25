package legacy

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/canonical/tc"
)

type executorSuite struct{}

var _ = tc.InternalSuite(&executorSuite{})

// TestExecute checks working-directory, environment and independent output-stream delivery.
func (*executorSuite) TestExecute(c *tc.C) {
	before, err := os.Getwd()
	c.Assert(err, tc.IsNil)

	c.Setenv("EXECUTOR_VALUE", "parent")
	dir := charm(c, `printf '%s' "$EXECUTOR_VALUE"; printf error >&2; pwd > working-directory`, "hooks/install")
	var stdout, stderr bytes.Buffer
	err = (processExecutor{}).Execute(context.Background(), Command{
		Path: filepath.Join(dir, "hooks/install"), Dir: dir,
		Env: []string{"EXECUTOR_VALUE=provided"}, Stdout: &stdout, Stderr: &stderr,
	})
	c.Assert(err, tc.IsNil)

	c.Check(stdout.String(), tc.Equals, "provided")
	c.Check(stderr.String(), tc.Equals, "error")
	stored, err := os.ReadFile(filepath.Join(dir, "working-directory"))
	c.Assert(err, tc.IsNil)

	c.Check(string(stored), tc.Equals, dir+"\n")
	after, err := os.Getwd()
	c.Assert(err, tc.IsNil)

	c.Check(after, tc.Equals, before)
	c.Check(os.Getenv("EXECUTOR_VALUE"), tc.Equals, "parent")
}

// TestProcessFailures checks missing executables, invalid working directories, and nonzero exits retain their causes.
func (*executorSuite) TestProcessFailures(c *tc.C) {
	dir := charm(c, "exit 23", "hooks/install")
	err := (processExecutor{}).Execute(context.Background(), Command{Path: filepath.Join(dir, "hooks/install"), Dir: dir})
	var exit *exec.ExitError
	c.Assert(errors.As(err, &exit), tc.Equals, true)

	c.Check(exit.ExitCode(), tc.Equals, 23)

	err = (processExecutor{}).Execute(context.Background(), Command{Path: filepath.Join(dir, "missing")})
	c.Check(err, tc.NotNil)

	err = (processExecutor{}).Execute(context.Background(), Command{Path: filepath.Join(dir, "hooks/install"), Dir: filepath.Join(dir, "missing")})
	c.Check(err, tc.NotNil)
}

// TestCanceledExecution checks that an already canceled command cannot run hook side effects.
func (*executorSuite) TestCanceledExecution(c *tc.C) {
	dir := charm(c, "printf should-not-run", "hooks/install")
	var output bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := (processExecutor{}).Execute(ctx, Command{Path: filepath.Join(dir, "hooks/install"), Stdout: &output})
	c.Check(err, tc.ErrorIs, context.Canceled)
	c.Check(output.Len(), tc.Equals, 0)
}
