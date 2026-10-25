//go:build !linux

package configapi

import (
	"context"
	"github.com/canonical/tc"
)

type watchSuite struct{}

var _ = tc.InternalSuite(&watchSuite{})

// TestUnsupportedWatcher verifies non-Linux callers receive an actionable error.
func (*watchSuite) TestUnsupportedWatcher(c *tc.C) {
	c.Check(watchFile(context.Background(), "/config.yaml", func() error { c.Error("callback executed"); return nil }), tc.ErrorMatches, "local config API watching requires Linux inotify")
}
