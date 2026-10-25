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
	c.Check((&fileClient{path: "/config.yaml"}).Watch(context.Background(), func(string) { c.Error("callback executed") }), tc.ErrorMatches, "file watching requires Linux inotify")
}
