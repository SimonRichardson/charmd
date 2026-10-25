package hookd

import (
	"github.com/canonical/tc"
)

type commandSuite struct{}

var _ = tc.InternalSuite(&commandSuite{})

// TestToolRegistry checks that every advertised tool can be constructed and unknown tools fail.
func (*commandSuite) TestToolRegistry(c *tc.C) {
	c.Check(ToolNames(), tc.DeepEquals, []string{"config-get", "juju-log", "status-get", "status-set"})
	for _, name := range ToolNames() {
		c.Check(IsTool(name), tc.Equals, true)
		command, err := NewCommand(&testContext{}, name)
		c.Assert(err, tc.IsNil)

		c.Check(command, tc.NotNil)
	}

	c.Check(IsTool("unknown"), tc.Equals, false)
	_, err := NewCommand(&testContext{}, "unknown")
	c.Check(err, tc.ErrorMatches, `unsupported hook tool "unknown"`)
}

// TestToolNamesAreIndependent checks a caller cannot mutate the registry through its advertised command list.
func (*commandSuite) TestToolNamesAreIndependent(c *tc.C) {
	names := ToolNames()
	names[0] = "corrupted"
	c.Check(IsTool("corrupted"), tc.Equals, false)
	c.Check(IsTool("config-get"), tc.Equals, true)
	c.Check(ToolNames(), tc.DeepEquals, []string{"config-get", "juju-log", "status-get", "status-set"})
}
