package legacy

import (
	"bytes"
	"context"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
	"github.com/canonical/tc"
	"os"
	"path/filepath"
	"strings"
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
