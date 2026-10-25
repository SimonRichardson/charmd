//go:build linux

package inotify

import (
	"context"
	"errors"
	"github.com/canonical/tc"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Test connects watcher tests to the Go runner.
func Test(t *testing.T) { tc.InternalTestingT(t) }

type watchSuite struct{}

var _ = tc.InternalSuite(&watchSuite{})

// TestWatchSetupFailures verifies cancellation, missing directories, and initial
// read errors are returned without leaving watcher resources running.
func (*watchSuite) TestWatchSetupFailures(c *tc.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Check(WatchFile(ctx, "/unused", func() error { return nil }), tc.Equals, context.Canceled)
	c.Check(WatchFile(context.Background(), filepath.Join(c.MkDir(), "missing", "config"), func() error { return nil }), tc.NotNil)
	failure := errors.New("initial read failed")
	c.Check(WatchFile(context.Background(), filepath.Join(c.MkDir(), "config"), func() error { return failure }), tc.Equals, failure)

}

// TestDirectoryRemovalStopsWatch verifies watch invalidation is reported for reconnect.
func (*watchSuite) TestDirectoryRemovalStopsWatch(c *tc.C) {
	dir := filepath.Join(c.MkDir(), "watched")
	c.Assert(os.Mkdir(dir, 0755), tc.IsNil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WatchFile(ctx, filepath.Join(dir, "config"), func() error { close(ready); return nil })
	}()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		c.Fatal("watch did not start")
	}
	c.Assert(os.Remove(dir), tc.IsNil)

	select {
	case err := <-done:
		c.Check(err, tc.ErrorMatches, "parent directory watch was removed")
	case <-time.After(5 * time.Second):
		c.Fatal("watch did not report directory removal")
	}
}

// TestReplacementFilteringAndCancellation verifies directory watches survive
// atomic replacement, ignore unrelated files, and shut down while idle.
func (*watchSuite) TestReplacementFilteringAndCancellation(c *tc.C) {
	dir := c.MkDir()
	target := filepath.Join(dir, "watched")
	events := make(chan struct{}, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- WatchFile(ctx, target, func() error { events <- struct{}{}; return nil }) }()
	receive := func() {
		select {
		case <-events:
		case <-time.After(5 * time.Second):
			c.Fatal("watch callback missing")
		}
	}
	receive()
	c.Assert(os.WriteFile(filepath.Join(dir, "unrelated"), []byte("ignored"), 0644), tc.IsNil)

	c.Assert(os.WriteFile(target, []byte("first"), 0644), tc.IsNil)

	receive()
	c.Assert(os.WriteFile(target+".new", []byte("second"), 0644), tc.IsNil)

	c.Assert(os.Rename(target+".new", target), tc.IsNil)

	receive()
	cancel()
	select {
	case err := <-done:
		c.Check(err, tc.Equals, context.Canceled)
	case <-time.After(5 * time.Second):
		c.Fatal("watch did not cancel")
	}
	c.Check(events, tc.HasLen, 0)
}
