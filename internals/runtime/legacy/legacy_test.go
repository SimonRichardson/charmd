package legacy

import (
	"bytes"
	"context"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
	"github.com/canonical/tc"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type legacySuite struct{}

var _ = tc.InternalSuite(&legacySuite{})

// TestInstallHookTools verifies the hook environment, command round trips, output, status, and socket cleanup.
func (*legacySuite) TestInstallHookTools(c *tc.C) {
	dir := charm(c, `
[ "$PWD" = "$JUJU_CHARM_DIR" ]
[ "$CHARM_DIR" = "$JUJU_CHARM_DIR" ]
[ "$JUJU_HOOK_NAME" = install ]
[ "$JUJU_DISPATCH_PATH" = hooks/install ]
[ "$JUJU_UNIT_NAME" = test/0 ]
[ -z "${JUJU_RELATION_ID+x}" ]
[ "$(config-get greeting)" = hello ]
[ "$(config-get --format=json greeting)" = '"hello"' ]
[ "$(config-get --format=json missing)" = null ]
status-set maintenance 'installing test'
[ "$(status-get)" = maintenance ]
juju-log -l INFO 'from hook tool'
printf '%s' "$JUJU_AGENT_SOCKET_ADDRESS" > socket-path
printf 'installed\n'
printf 'diagnostic\n' >&2
`, "hooks/install")
	c.Assert(os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("options:\n  greeting:\n    type: string\n    default: hello\n"), 0644), tc.IsNil)

	c.Setenv("JUJU_RELATION_ID", "stale")
	var output bytes.Buffer
	exe, err := os.Executable()
	c.Assert(err, tc.IsNil)

	result, err := (&Runtime{Executable: exe}).RunHook(context.Background(), charmRuntime.Hook{CharmDir: dir, Name: "install", Stdout: &output, Stderr: &output})
	c.Assert(err, tc.IsNil, tc.Commentf("output: %s", output.String()))

	c.Assert(result.Status, tc.NotNil)

	c.Check(*result.Status, tc.Equals, charmRuntime.Status{Status: "maintenance", Message: "installing test"})
	for _, expected := range []string{"installed", "diagnostic", "from hook tool"} {
		c.Check(strings.Contains(output.String(), expected), tc.Equals, true, tc.Commentf("missing %q in %q", expected, output.String()))
	}
	socket, err := os.ReadFile(filepath.Join(dir, "socket-path"))
	c.Assert(err, tc.IsNil)

	_, err = os.Stat(string(socket))
	c.Check(os.IsNotExist(err), tc.Equals, true)
}

// TestMissingHook verifies that charms without an install handler succeed.
func (*legacySuite) TestMissingHook(c *tc.C) {
	_, err := (&Runtime{}).RunHook(context.Background(), charmRuntime.Hook{CharmDir: charm(c, "", ""), Name: "install"})
	c.Check(err, tc.IsNil)
}

// TestHookFailures verifies that process and hook-tool errors fail the hook.
func (*legacySuite) TestHookFailures(c *tc.C) {
	exe, err := os.Executable()
	c.Assert(err, tc.IsNil)

	for _, script := range []string{"exit 17", "config-get --invalid", "status-set invalid", "missing-hook-tool"} {
		_, err := (&Runtime{Executable: exe}).RunHook(context.Background(), charmRuntime.Hook{CharmDir: charm(c, script, "hooks/install"), Name: "install"})
		c.Check(err, tc.NotNil, tc.Commentf("script: %s", script))
	}
}

// TestUnsupportedHooksHaveNoSideEffects checks invalid events are rejected before context loading or execution.
func (*legacySuite) TestUnsupportedHooksHaveNoSideEffects(c *tc.C) {
	calls := 0
	runtime := &Runtime{Executor: executorFunc(func(context.Context, Command) error { calls++; return nil })}
	result, err := runtime.RunHook(context.Background(), charmRuntime.Hook{CharmDir: "/missing-charm", Name: "../install"})
	c.Check(err, tc.ErrorMatches, `unsupported legacy hook "../install"`)
	c.Check(result.Status, tc.IsNil)
	c.Check(calls, tc.Equals, 0)
}

// TestInvalidCharmCannotExecute checks context loading failures leave runtime execution untouched.
func (*legacySuite) TestInvalidCharmCannotExecute(c *tc.C) {
	calls := 0
	runtime := &Runtime{Executor: executorFunc(func(context.Context, Command) error { calls++; return nil })}
	result, err := runtime.RunHook(context.Background(), charmRuntime.Hook{CharmDir: c.MkDir(), Name: "install"})
	c.Check(err, tc.NotNil)
	c.Check(result.Status, tc.IsNil)
	c.Check(calls, tc.Equals, 0)
}

// TestHooksAreSerialized verifies different charms share one execution slot and completion releases it.
func (*legacySuite) TestHooksAreSerialized(c *tc.C) {
	exe, err := os.Executable()
	c.Assert(err, tc.IsNil)

	entered := make(chan string, 2)
	release := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runtime := &Runtime{Executable: exe, Executor: executorFunc(func(ctx context.Context, command Command) error {
		entered <- command.Dir
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})}
	first := charm(c, "exit 0", "hooks/install")
	second := charm(c, "exit 0", "dispatch")
	done := make(chan error, 2)
	go func() {
		_, err := runtime.RunHook(ctx, charmRuntime.Hook{CharmDir: first, Name: "install"})
		done <- err
	}()
	select {
	case dir := <-entered:
		c.Assert(dir, tc.Equals, first)

	case <-ctx.Done():
		c.Fatal("first hook did not start")
	}
	go func() {
		_, err := runtime.RunHook(ctx, charmRuntime.Hook{CharmDir: second, Name: "install"})
		done <- err
	}()
	select {
	case <-entered:
		c.Fatal("second hook started while the first was running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	for range 2 {
		select {
		case err := <-done:
			c.Assert(err, tc.IsNil)

		case <-ctx.Done():
			c.Fatal("hooks did not complete")
		}
	}
	c.Check(<-entered, tc.Equals, second)
}

// TestCanceledHookCannotExecute verifies cancellation prevents preparation and execution.
func (*legacySuite) TestCanceledHookCannotExecute(c *tc.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime := &Runtime{Executor: executorFunc(func(context.Context, Command) error {
		c.Error("canceled hook executed")
		return nil
	})}
	_, err := runtime.RunHook(ctx, charmRuntime.Hook{CharmDir: "/missing", Name: "install"})
	c.Check(err, tc.Equals, context.Canceled)
}

// TestLifecycleHooks verifies discovery and environment for each supported lifecycle event.
func (*legacySuite) TestLifecycleHooks(c *tc.C) {
	exe, err := os.Executable()
	c.Assert(err, tc.IsNil)

	runtime := &Runtime{Executable: exe}
	for _, name := range []string{"install", "config-changed", "leader-elected", "start"} {
		for _, handler := range []string{"dispatch", "hooks/" + name} {
			dir := charm(c, `
[ "$JUJU_DISPATCH_PATH" = "hooks/$JUJU_HOOK_NAME" ]
printf '%s' "$JUJU_HOOK_NAME"
`, handler)
			var output bytes.Buffer
			_, err := runtime.RunHook(context.Background(), charmRuntime.Hook{CharmDir: dir, Name: name, Stdout: &output})
			c.Assert(err, tc.IsNil)

			c.Check(output.String(), tc.Equals, name)
		}
		_, err := runtime.RunHook(context.Background(), charmRuntime.Hook{CharmDir: charm(c, "", ""), Name: name})
		c.Check(err, tc.IsNil)
	}
}

// TestUnknownConfigCannotExecute verifies service options are checked before starting charm code.
func (*legacySuite) TestUnknownConfigCannotExecute(c *tc.C) {
	calls := 0
	runtime := &Runtime{Executor: executorFunc(func(context.Context, Command) error { calls++; return nil })}
	_, err := runtime.RunHook(context.Background(), charmRuntime.Hook{CharmDir: charm(c, "exit 0", "dispatch"), Name: "config-changed", Config: map[string]any{"unknown": "value"}})
	c.Check(err, tc.ErrorMatches, `unknown charm config option "unknown"`)
	c.Check(calls, tc.Equals, 0)
}

// TestLeadershipSnapshotPerInvocation verifies real hookd commands see the same
// leadership throughout each invocation and cannot leak it into the next hook.
func (*legacySuite) TestLeadershipSnapshotPerInvocation(c *tc.C) {
	dir := charm(c, `
first=$(is-leader --format=json)
[ "$(is-leader)" = "$first" ]
printf '%s\n' "$first"
`, "dispatch")
	exe, err := os.Executable()
	c.Assert(err, tc.IsNil)

	runtime := &Runtime{Executable: exe}
	for _, leader := range []bool{true, false, true} {
		var output bytes.Buffer
		_, err := runtime.RunHook(context.Background(), charmRuntime.Hook{CharmDir: dir, Name: "leader-elected", Leader: leader, Stdout: &output, Stderr: &output})
		c.Assert(err, tc.IsNil)

		expected := "false\n"
		if leader {
			expected = "true\n"
		}
		c.Check(output.String(), tc.Equals, expected)
	}
}
