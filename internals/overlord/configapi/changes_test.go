package configapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/SimonRichardson/charmd/internals/overlord/runtimestate"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
	"github.com/SimonRichardson/charmd/internals/runtime/legacy"
	"github.com/canonical/tc"
)

type changesSuite struct{}

var _ = tc.InternalSuite(&changesSuite{})

// TestEveryNotificationCreatesChange verifies the service owns value comparison:
// even identical snapshots produce separate changes and config-changed hooks.
func (*changesSuite) TestEveryNotificationCreatesChange(c *tc.C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	client := &fakeClient{values: map[string]any{"greeting": "first"}}
	manager := NewManager(st, runner, client)
	defer manager.Stop()
	var hooks []charmRuntime.Hook
	runtimestate.NewManager(st, runner, runtimeFunc(func(ctx context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		hooks = append(hooks, hook)
		return charmRuntime.Result{}, nil
	}))
	checksum := strings.Repeat("a", 64)
	st.Lock()
	install := installed(st, checksum, "/charm")
	st.Unlock()
	settle(c, manager, runner)
	c.Check(hooks, tc.HasLen, 0)
	c.Check(client.gets, tc.Equals, 0)
	for range 2 {
		manager.changed(checksum)
		settle(c, manager, runner)
	}
	c.Assert(hooks, tc.HasLen, 2)

	c.Check(hooks[0].Name, tc.Equals, "config-changed")
	c.Check(hooks[0].CharmDir, tc.Equals, "/charm")
	c.Check(hooks[0].Config, tc.DeepEquals, map[string]any{"greeting": "first"})
	c.Check(hooks[1].Config, tc.DeepEquals, hooks[0].Config)
	st.Lock()
	c.Check(install.Change().Tasks(), tc.HasLen, 2)
	c.Check(st.Changes(), tc.HasLen, 3)
	for _, change := range st.Changes() {
		if change == install.Change() {
			continue
		}
		c.Check(change.Kind(), tc.Equals, "change-config")
		c.Check(change.Tasks(), tc.HasLen, 2)
		c.Check(change.Err(), tc.IsNil)
	}
	st.Unlock()
	client.set(map[string]any{"greeting": "updated"})
	manager.changed(checksum)
	settle(c, manager, runner)
	c.Assert(hooks, tc.HasLen, 3)

	c.Check(hooks[0].Config["greeting"], tc.Equals, "first")
	c.Check(hooks[2].Config["greeting"], tc.Equals, "updated")
}

// TestIncompleteInstallNeverRunsConfig verifies notifications survive pending or
// failed installation and run only after its install hook succeeds.
func (*changesSuite) TestIncompleteInstallNeverRunsConfig(c *tc.C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	client := &fakeClient{values: map[string]any{}}
	manager := NewManager(st, runner, client)
	defer manager.Stop()
	runtimestate.NewManager(st, runner, runtimeFunc(discardRuntime))
	checksum := strings.Repeat("a", 64)
	st.Lock()
	install := installed(st, checksum, "/charm")
	install.SetStatus(state.DoStatus)
	st.Unlock()
	manager.changed(checksum)
	c.Assert(manager.Ensure(), tc.IsNil)

	st.Lock()
	c.Check(st.Changes(), tc.HasLen, 1)
	install.SetStatus(state.ErrorStatus)
	st.Unlock()
	c.Assert(manager.Ensure(), tc.IsNil)

	st.Lock()
	c.Check(st.Changes(), tc.HasLen, 1)
	install.SetStatus(state.DoneStatus)
	st.Unlock()
	settle(c, manager, runner)
	c.Check(client.gets, tc.Equals, 1)
}

// TestFetchFailureDoesNotRunHook verifies GET failures fail their own changes;
// they do not damage installation or prevent a later notification from succeeding.
func (*changesSuite) TestFetchFailureDoesNotRunHook(c *tc.C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	client := &fakeClient{err: errors.New("service unavailable")}
	manager := NewManager(st, runner, client)
	defer manager.Stop()
	calls := 0
	runtimestate.NewManager(st, runner, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		calls++
		return charmRuntime.Result{}, nil
	}))
	checksum := strings.Repeat("a", 64)
	st.Lock()
	install := installed(st, checksum, "/charm")
	st.Unlock()
	manager.changed(checksum)
	settle(c, manager, runner)
	c.Check(calls, tc.Equals, 0)
	st.Lock()
	for _, change := range st.Changes() {
		if change != install.Change() {
			c.Check(change.Err(), tc.NotNil)
		}
	}
	c.Check(install.Status(), tc.Equals, state.DoneStatus)
	st.Unlock()
	client.mu.Lock()
	client.err = nil
	client.mu.Unlock()
	manager.changed(checksum)
	settle(c, manager, runner)
	c.Check(calls, tc.Equals, 1)
}

// TestNotificationBurstPreservesEveryChange verifies active work does not cause
// charmd to coalesce or discard notifications supplied by the service.
func (*changesSuite) TestNotificationBurstPreservesEveryChange(c *tc.C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	client := &fakeClient{values: map[string]any{}}
	manager := NewManager(st, runner, client)
	defer manager.Stop()
	calls := 0
	runtimestate.NewManager(st, runner, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		calls++
		return charmRuntime.Result{}, nil
	}))
	checksum := strings.Repeat("a", 64)
	st.Lock()
	installed(st, checksum, "/charm")
	st.Unlock()
	for range 3 {
		manager.changed(checksum)
	}
	settle(c, manager, runner)
	c.Check(calls, tc.Equals, 3)
	c.Check(client.gets, tc.Equals, 3)
	st.Lock()
	defer st.Unlock()
	c.Check(st.Changes(), tc.HasLen, 4)
}

// TestRealHookReadsServiceConfig verifies config-get and dispatch environment
// match the Juju config-changed interface without rewriting charm files.
func (*changesSuite) TestRealHookReadsServiceConfig(c *tc.C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	manager := NewManager(st, runner, &fakeClient{values: map[string]any{"greeting": "from-service"}})
	defer manager.Stop()
	exe, err := os.Executable()
	c.Assert(err, tc.IsNil)

	runtimestate.NewManager(st, runner, &legacy.Runtime{Executable: exe})
	dir := c.MkDir()
	files := map[string]string{
		"metadata.yaml": "name: config-test\n",
		"config.yaml":   "options:\n  greeting:\n    default: default-value\n  untouched:\n    default: kept\n  unset:\n    type: string\n",
		"dispatch": `#!/bin/sh
set -eu
[ "$JUJU_HOOK_NAME" = config-changed ]
[ "$JUJU_DISPATCH_PATH" = hooks/config-changed ]
[ "$JUJU_CHARM_DIR" = "$PWD" ]
[ "$CHARM_DIR" = "$PWD" ]
[ -n "$JUJU_CONTEXT_ID" ]
[ "$JUJU_AGENT_SOCKET_NETWORK" = unix ]
[ -S "$JUJU_AGENT_SOCKET_ADDRESS" ]
[ "$(config-get greeting)" = from-service ]
[ "$(config-get untouched)" = kept ]
[ "$(config-get --format=json)" = '{"greeting":"from-service","untouched":"kept"}' ]
[ "$(config-get --all --format=json)" = '{"greeting":"from-service","unset":null,"untouched":"kept"}' ]
status-set --application=False active world
printf '%s' "$JUJU_HOOK_NAME" > observed
`,
	}
	for name, body := range files {
		c.Assert(os.WriteFile(filepath.Join(dir, name), []byte(body), 0755), tc.IsNil)
	}
	checksum := strings.Repeat("a", 64)
	st.Lock()
	installed(st, checksum, dir)
	st.Unlock()
	manager.changed(checksum)
	settle(c, manager, runner)
	observed, err := os.ReadFile(filepath.Join(dir, "observed"))
	c.Assert(err, tc.IsNil)

	c.Check(string(observed), tc.Equals, "config-changed")
	defaults, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	c.Assert(err, tc.IsNil)

	c.Check(string(defaults), tc.Equals, files["config.yaml"])
}

// TestPendingEventsSurviveRestart verifies notifications persist independently
// from task history and are not regenerated by repeated ensure passes.
func (*changesSuite) TestPendingEventsSurviveRestart(c *tc.C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	client := &fakeClient{values: map[string]any{"value": "original"}}
	manager := NewManager(st, runner, client)
	defer manager.Stop()
	checksum := strings.Repeat("a", 64)
	manager.changed(checksum)
	st.Lock()
	data, err := json.Marshal(st)
	st.Unlock()
	c.Assert(err, tc.IsNil)

	restored, err := state.ReadState(nil, bytes.NewReader(data))
	c.Assert(err, tc.IsNil)

	restoredRunner := state.NewTaskRunner(restored)
	defer restoredRunner.Stop()
	restoredManager := NewManager(restored, restoredRunner, client)
	defer restoredManager.Stop()
	calls := 0
	runtimestate.NewManager(restored, restoredRunner, runtimeFunc(func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		calls++
		return charmRuntime.Result{}, nil
	}))
	restored.Lock()
	installed(restored, checksum, "/charm")
	restored.Unlock()
	settle(c, restoredManager, restoredRunner)
	c.Check(calls, tc.Equals, 1)
	settle(c, restoredManager, restoredRunner)
	c.Check(calls, tc.Equals, 1)
}
