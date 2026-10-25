//go:build !linux

package legacy

import (
	"context"
	"os/exec"

	"github.com/canonical/tc"
)

type processSuite struct{}

var _ = tc.InternalSuite(&processSuite{})

// TestPreservesDefaultCancellation checks the fallback keeps the standard library's cancellation callback.
func (*processSuite) TestPreservesDefaultCancellation(c *tc.C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "exit 0")
	configureProcess(cmd)
	c.Check(cmd.Cancel, tc.NotNil)
	c.Check(cmd.SysProcAttr, tc.IsNil)

	cancel()
	c.Check(cmd.Run(), tc.ErrorIs, context.Canceled)
}

// TestDoesNotAddCancellation checks a command without a context retains its original process settings.
func (*processSuite) TestDoesNotAddCancellation(c *tc.C) {
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	configureProcess(cmd)
	c.Check(cmd.Cancel, tc.IsNil)
	c.Check(cmd.SysProcAttr, tc.IsNil)
}
