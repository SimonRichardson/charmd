//go:build linux

package leadershipapi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/canonical/tc"
)

type watchSuite struct{}

var _ = tc.InternalSuite(&watchSuite{})

// TestSemanticChanges verifies initial notification, equivalent writes, malformed
// updates, atomic replacement and removal against the real shared inotify backend.
func (*watchSuite) TestSemanticChanges(c *tc.C) {
	checksum := strings.Repeat("a", 64)
	path := filepath.Join(c.MkDir(), "leadership.yaml")
	original := "charms:\n  " + checksum + ": false\n"
	c.Assert(os.WriteFile(path, []byte(original), 0644), tc.IsNil)

	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan string, 16)
	done := make(chan error, 1)
	go func() { done <- (&fileClient{path: path}).Watch(ctx, func(sha string) { events <- sha }) }()
	c.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			c.Check(err, tc.Equals, context.Canceled)
		case <-time.After(5 * time.Second):
			c.Error("watch did not stop")
		}
	})
	expect := func() {
		select {
		case sha := <-events:
			c.Check(sha, tc.Equals, checksum)
		case <-time.After(5 * time.Second):
			c.Fatal("leadership notification missing")
		}
	}
	quiet := func() {
		select {
		case sha := <-events:
			c.Fatalf("unexpected notification %s", sha)
		case <-time.After(50 * time.Millisecond):
		}
	}
	expect()
	c.Assert(os.WriteFile(path, []byte("# unchanged\n"+original), 0644), tc.IsNil)

	quiet()
	c.Assert(os.WriteFile(path, []byte("charms: ["), 0644), tc.IsNil)

	quiet()
	c.Assert(os.WriteFile(path, []byte(original), 0644), tc.IsNil)

	quiet()
	c.Assert(os.WriteFile(path+".new", []byte(strings.Replace(original, "false", "true", 1)), 0644), tc.IsNil)

	c.Assert(os.Rename(path+".new", path), tc.IsNil)

	expect()
	c.Assert(os.WriteFile(path, []byte("charms: {}\n"), 0644), tc.IsNil)

	expect()
	quiet()
}
