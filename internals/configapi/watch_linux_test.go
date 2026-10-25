//go:build linux

package configapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/canonical/tc"
)

type watchSuite struct{}

var _ = tc.InternalSuite(&watchSuite{})

// startFileWatch starts the real inotify backend and joins it during cleanup.
func startFileWatch(c *tc.C, client *fileClient) <-chan string {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan string, 16)
	done := make(chan error, 1)
	go func() { done <- client.Watch(ctx, func(checksum string) { events <- checksum }) }()
	c.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			c.Check(err, tc.Equals, context.Canceled)
		case <-time.After(5 * time.Second):
			c.Error("inotify watch did not stop")
		}
	})
	return events
}

// expectFileEvent waits for a notification from the real file watcher.
func expectFileEvent(c *tc.C, events <-chan string, want string) {
	select {
	case got := <-events:
		c.Assert(got, tc.Equals, want)

	case <-time.After(5 * time.Second):
		c.Fatal("file event was not emitted")
	}
}

// expectNoFileEvent verifies filesystem writes alone do not imply config changes.
func expectNoFileEvent(c *tc.C, events <-chan string) {
	select {
	case got := <-events:
		c.Fatalf("unexpected config notification for %s", got)
	case <-time.After(75 * time.Millisecond):
	}
}

// TestSemanticChangesOnly verifies equivalent YAML does not emit a notification,
// while changed values and atomic file replacements do.
func (*watchSuite) TestSemanticChangesOnly(c *tc.C) {
	checksum := strings.Repeat("a", 64)
	path := filepath.Join(c.MkDir(), "config.yaml")
	original := "charms:\n  " + checksum + ":\n    greeting: hello\n    count: 2\n"
	c.Assert(os.WriteFile(path, []byte(original), 0644), tc.IsNil)

	client := &fileClient{path: path}
	events := startFileWatch(c, client)
	expectFileEvent(c, events, checksum)
	equivalent := "# reordered without semantic changes\ncharms:\n  " + checksum + ":\n    count: 2\n    greeting: hello\n"
	c.Assert(os.WriteFile(path, []byte(equivalent), 0644), tc.IsNil)

	expectNoFileEvent(c, events)
	changed := strings.Replace(original, "hello", "updated", 1)
	c.Assert(os.WriteFile(path, []byte(changed), 0644), tc.IsNil)

	expectFileEvent(c, events, checksum)
	replacement := path + ".new"
	c.Assert(os.WriteFile(replacement, []byte(original), 0644), tc.IsNil)

	c.Assert(os.Rename(replacement, path), tc.IsNil)

	expectFileEvent(c, events, checksum)
	expectNoFileEvent(c, events)
}

// TestInvalidUpdatesRetainLastGoodSnapshot verifies malformed writes are ignored,
// repairs compare against the last good values, and removal restores defaults.
func (*watchSuite) TestInvalidUpdatesRetainLastGoodSnapshot(c *tc.C) {
	checksum := strings.Repeat("a", 64)
	path := filepath.Join(c.MkDir(), "config.yaml")
	original := "charms:\n  " + checksum + ":\n    value: good\n"
	c.Assert(os.WriteFile(path, []byte(original), 0644), tc.IsNil)

	client := &fileClient{path: path}
	events := startFileWatch(c, client)
	expectFileEvent(c, events, checksum)
	c.Assert(os.WriteFile(path, []byte("charms: ["), 0644), tc.IsNil)

	expectNoFileEvent(c, events)
	c.Assert(os.WriteFile(path, []byte(original), 0644), tc.IsNil)

	expectNoFileEvent(c, events)
	c.Assert(os.WriteFile(path, []byte("charms: {}\n"), 0644), tc.IsNil)

	expectFileEvent(c, events, checksum)
	values, err := client.Get(context.Background(), checksum)
	c.Assert(err, tc.IsNil)

	c.Check(values, tc.DeepEquals, map[string]any{})
}

// TestOnlyAffectedCharmIsNotified verifies per-charm comparison within one file.
func (*watchSuite) TestOnlyAffectedCharmIsNotified(c *tc.C) {
	first, second := strings.Repeat("a", 64), strings.Repeat("b", 64)
	path := filepath.Join(c.MkDir(), "config.yaml")
	original := "charms:\n  " + first + ": {value: first}\n  " + second + ": {value: second}\n"
	c.Assert(os.WriteFile(path, []byte(original), 0644), tc.IsNil)

	events := startFileWatch(c, &fileClient{path: path})
	seen := map[string]bool{}
	for range 2 {
		select {
		case checksum := <-events:
			seen[checksum] = true
		case <-time.After(5 * time.Second):
			c.Fatal("initial configs not emitted")
		}
	}
	c.Check(seen, tc.DeepEquals, map[string]bool{first: true, second: true})
	c.Assert(os.WriteFile(path, []byte(strings.Replace(original, "value: second", "value: updated", 1)), 0644), tc.IsNil)

	expectFileEvent(c, events, second)
	expectNoFileEvent(c, events)
}

// TestWatchSetupFailures verifies cancellation, missing directories, and initial
// read errors are returned without leaving watcher resources running.
func (*watchSuite) TestWatchSetupFailures(c *tc.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Check(watchFile(ctx, "/unused", func() error { return nil }), tc.Equals, context.Canceled)
	c.Check(watchFile(context.Background(), filepath.Join(c.MkDir(), "missing", "config"), func() error { return nil }), tc.NotNil)
	failure := errors.New("initial read failed")
	c.Check(watchFile(context.Background(), filepath.Join(c.MkDir(), "config"), func() error { return failure }), tc.Equals, failure)
	client := &fileClient{path: filepath.Join(c.MkDir(), "missing")}
	c.Check(client.Watch(context.Background(), func(string) {}), tc.NotNil)
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
		done <- watchFile(ctx, filepath.Join(dir, "config"), func() error { close(ready); return nil })
	}()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		c.Fatal("watch did not start")
	}
	c.Assert(os.Remove(dir), tc.IsNil)

	select {
	case err := <-done:
		c.Check(err, tc.ErrorMatches, "config directory watch was removed")
	case <-time.After(5 * time.Second):
		c.Fatal("watch did not report directory removal")
	}
}
