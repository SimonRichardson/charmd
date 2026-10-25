package legacy

import (
	"os"
	"strings"

	"github.com/canonical/tc"
)

type environmentSuite struct{}

var _ = tc.InternalSuite(&environmentSuite{})

// environmentValues rejects duplicate variables while decoding an execution environment for assertions.
func environmentValues(c *tc.C, env []string) map[string]string {
	values := make(map[string]string)
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		c.Assert(ok, tc.Equals, true)

		_, duplicate := values[key]
		c.Check(duplicate, tc.Equals, false, tc.Commentf("duplicate variable: %s", key))
		values[key] = value
	}
	return values
}

// TestInvocationIsolation checks stale hook variables are replaced, normal host values survive, and the parent environment stays unchanged.
func (*environmentSuite) TestInvocationIsolation(c *tc.C) {
	c.Setenv("JUJU_RELATION_ID", "stale")
	c.Setenv("JUJU_CONTEXT_ID", "parent-context")
	c.Setenv("CHARM_DIR", "/old-charm")
	c.Setenv("PATH", "/host/bin")
	c.Setenv("CHARMD_ENV_TEST", "retained=value")
	before := os.Environ()
	ctx, err := NewHookContext(charm(c, "", ""), nil)
	c.Assert(err, tc.IsNil)

	paths := Paths{CharmDir: "/charm", ToolsDir: "/tools", Socket: "/hookd.sock"}
	values := environmentValues(c, ctx.Environment(paths, "install"))
	c.Check(values["PATH"], tc.Equals, "/tools"+string(os.PathListSeparator)+"/host/bin")
	c.Check(values["CHARMD_ENV_TEST"], tc.Equals, "retained=value")
	c.Check(values["JUJU_CONTEXT_ID"], tc.Equals, ctx.ID())
	c.Check(values["CHARM_DIR"], tc.Equals, paths.CharmDir)
	c.Check(values["JUJU_CHARM_DIR"], tc.Equals, paths.CharmDir)
	c.Check(values["JUJU_AGENT_SOCKET_ADDRESS"], tc.Equals, paths.Socket)
	c.Check(values["JUJU_AGENT_SOCKET_NETWORK"], tc.Equals, "unix")
	c.Check(values["JUJU_DISPATCH_PATH"], tc.Equals, "hooks/install")
	c.Check(values["JUJU_UNIT_NAME"], tc.Equals, "test/0")
	_, stale := values["JUJU_RELATION_ID"]
	c.Check(stale, tc.Equals, false)
	c.Check(os.Environ(), tc.DeepEquals, before)
}

// TestMissingHostPath checks that tools and standard system binaries remain reachable without an inherited PATH.
func (*environmentSuite) TestMissingHostPath(c *tc.C) {
	c.Setenv("PATH", "")
	values := environmentValues(c, hookEnvironment("/tools", "/charm", "/socket", "context", "test", "install"))
	c.Check(values["PATH"], tc.Equals, "/tools"+string(os.PathListSeparator)+"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	c.Check(os.Getenv("PATH"), tc.Equals, "")
}
