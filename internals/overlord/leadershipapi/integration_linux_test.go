//go:build linux

package leadershipapi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SimonRichardson/charmd/internals/config"
	service "github.com/SimonRichardson/charmd/internals/leadershipapi"
	"github.com/SimonRichardson/charmd/internals/overlord/runtimestate"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
	"github.com/SimonRichardson/charmd/internals/runtime/legacy"
	"github.com/canonical/tc"
	"golang.org/x/sys/unix"
)

// observedClient exposes completed notifications so integration tests need no
// sleeps to guess whether the filesystem watcher has delivered a change.
type observedClient struct {
	service.Client
	events chan struct{}
}

// Watch forwards notifications before acknowledging their delivery to the test.
func (c *observedClient) Watch(ctx context.Context, notify func(string)) error {
	return c.Client.Watch(ctx, func(sha string) { notify(sha); c.events <- struct{}{} })
}

// readyOutput signals when a real shell reaches its explicit blocking point.
type readyOutput struct {
	once  sync.Once
	ready chan struct{}
}

// Write observes readiness output without changing the command's write contract.
func (w *readyOutput) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "waiting") {
		w.once.Do(func() { close(w.ready) })
	}
	return len(data), nil
}

type integrationSuite struct{}

var _ = tc.InternalSuite(&integrationSuite{})

// TestFileChangeDuringRealHook verifies file->inotify->manager->scheduler->hookd:
// the active hook sees false twice, then election precedes the queued config hook.
func (*integrationSuite) TestFileChangeDuringRealHook(c *tc.C) {
	checksum := strings.Repeat("a", 64)
	dir := c.MkDir()
	source := filepath.Join(dir, "leadership.yaml")
	c.Assert(os.WriteFile(source, []byte("charms: {"+checksum+": false}\n"), 0644), tc.IsNil)

	client, err := service.New(config.Endpoint{Path: source})
	c.Assert(err, tc.IsNil)

	observed := &observedClient{Client: client, events: make(chan struct{}, 8)}
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	manager := NewManager(st, runner, observed)
	c.Cleanup(func() { runner.Stop(); manager.Stop() })
	exe, err := os.Executable()
	c.Assert(err, tc.IsNil)

	runtime := &legacy.Runtime{Executable: exe}
	output := &readyOutput{ready: make(chan struct{})}
	runtimeManager := runtimestate.NewManager(st, runner, runtimeFunc(func(ctx context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		hook.Stdout = output
		return runtime.RunHook(ctx, hook)
	}))
	runtimeManager.Leadership = manager.HookLeadership
	for name, body := range map[string]string{
		"metadata.yaml": "name: leadership-test\n",
		"dispatch": `#!/bin/sh
set -eu
printf '%s:%s\n' "$JUJU_HOOK_NAME" "$(is-leader --format=json)" >> history
if [ "$JUJU_HOOK_NAME" = config-changed ]; then
 if [ ! -e once ]; then
  touch once
  exec 3<> gate
  echo waiting
  read release <&3
  exec 3>&-
 fi
 printf '%s:%s\n' "$JUJU_HOOK_NAME" "$(is-leader)" >> history
fi
`,
	} {
		c.Assert(os.WriteFile(filepath.Join(dir, name), []byte(body), 0755), tc.IsNil)
	}
	c.Assert(unix.Mkfifo(filepath.Join(dir, "gate"), 0600), tc.IsNil)

	st.Lock()
	unpack := st.NewTask("unpack", "Unpack charm")
	unpack.Set("charm-sha256", checksum)
	unpack.Set("charm-install-path", dir)
	unpack.SetStatus(state.DoneStatus)
	install := runtimestate.InstallTask(st, unpack)
	change := st.NewChange("install-charm", "Install")
	change.AddTask(unpack)
	change.AddTask(install)
	st.Unlock()
	c.Assert(manager.StartUp(), tc.IsNil)

	waitEvent(c, observed.events)
	settle(c, manager, runner)
	st.Lock()
	configHook(st, install)
	st.Unlock()
	c.Assert(runner.Ensure(), tc.IsNil)

	select {
	case <-output.ready:
	case <-time.After(5 * time.Second):
		c.Fatal("real hook did not block")
	}
	c.Assert(os.WriteFile(source+".new", []byte("charms: {"+checksum+": true}\n"), 0644), tc.IsNil)

	c.Assert(os.Rename(source+".new", source), tc.IsNil)

	waitEvent(c, observed.events)
	c.Assert(manager.Ensure(), tc.IsNil)

	st.Lock()
	configHook(st, install)
	st.Unlock()
	c.Assert(runner.Ensure(), tc.IsNil)

	// A nonblocking writer fails unless the original hook is still reading the FIFO.
	fd, err := unix.Open(filepath.Join(dir, "gate"), unix.O_WRONLY|unix.O_NONBLOCK, 0)
	c.Assert(err, tc.IsNil)

	_, err = unix.Write(fd, []byte("continue\n"))
	unix.Close(fd)
	c.Assert(err, tc.IsNil)

	runner.Wait()
	settle(c, manager, runner)
	history, err := os.ReadFile(filepath.Join(dir, "history"))
	c.Assert(err, tc.IsNil)

	c.Check(string(history), tc.Equals, "install:false\nconfig-changed:false\nconfig-changed:false\nleader-elected:true\nconfig-changed:true\nconfig-changed:true\n")
	st.Lock()
	for _, change := range st.Changes() {
		c.Check(change.Status(), tc.Equals, state.DoneStatus)
	}
	st.Unlock()
}

// waitEvent waits for an acknowledged watcher delivery with a bounded failure.
func waitEvent(c *tc.C, events <-chan struct{}) {
	select {
	case <-events:
	case <-time.After(5 * time.Second):
		c.Fatal("filesystem notification missing")
	}
}
