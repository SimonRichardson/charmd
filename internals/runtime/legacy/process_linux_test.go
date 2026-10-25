package legacy

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/canonical/tc"
)

type processSuite struct{}

var _ = tc.InternalSuite(&processSuite{})

// TestDedicatedProcessGroup checks configuration isolates hook signals from the daemon's process group.
func (*processSuite) TestDedicatedProcessGroup(c *tc.C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 30 & wait")
	configureProcess(cmd)
	c.Assert(cmd.Start(), tc.IsNil)

	defer func() { _ = cmd.Cancel(); _ = cmd.Wait() }()
	group, err := syscall.Getpgid(cmd.Process.Pid)
	c.Assert(err, tc.IsNil)

	c.Check(group, tc.Equals, cmd.Process.Pid)
	c.Check(group != syscall.Getpgrp(), tc.Equals, true)
	c.Check(cmd.Cancel(), tc.IsNil)
	c.Check(cmd.Wait(), tc.NotNil)
	c.Check(cmd.Cancel(), tc.ErrorIs, syscall.ESRCH)
}

// TestCancellationKillsDescendants checks a shell child cannot retain inherited output pipes after cancellation.
func (*processSuite) TestCancellationKillsDescendants(c *tc.C) {
	dir := charm(c, "sleep 30 &\nwait", "hooks/install")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := (processExecutor{}).Execute(ctx, Command{
		Path: filepath.Join(dir, "hooks/install"), Stdout: io.Discard, Stderr: io.Discard,
	})
	c.Check(errors.Is(err, context.DeadlineExceeded), tc.Equals, true)
	c.Check(time.Since(started) < 3*time.Second, tc.Equals, true)
}
