//go:build !linux

package inotify

import (
	"context"
	"github.com/canonical/tc"
	"testing"
)

type watchSuite struct{}

var _ = tc.InternalSuite(&watchSuite{})

// TestUnsupportedWatcher verifies non-Linux callers receive an actionable error.
func (*watchSuite) TestUnsupportedWatcher(c *tc.C) {
	c.Check(WatchFile(context.Background(), "/config.yaml", func() error { c.Error("callback executed"); return nil }), tc.ErrorMatches, "file watching requires Linux inotify")
}

// Test connects watcher tests to the Go runner.
func Test(t *testing.T) { tc.InternalTestingT(t) }
