package hookd

import (
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
	"github.com/canonical/tc"
)

type statusSuite struct{}

var _ = tc.InternalSuite(&statusSuite{})

// TestStatusCommands checks status updates and ensures malformed commands do not change status.
func (*statusSuite) TestStatusCommands(c *tc.C) {
	ctx := &testContext{status: charmRuntime.Status{Status: "maintenance"}}
	set, err := NewCommand(ctx, "status-set")
	c.Assert(err, tc.IsNil)

	get, err := NewCommand(ctx, "status-get")
	c.Assert(err, tc.IsNil)

	for _, status := range []string{"maintenance", "blocked", "waiting", "active"} {
		output, err := set.Run([]string{status, "workload", "message"})
		c.Assert(err, tc.IsNil)

		c.Check(output, tc.HasLen, 0)
		c.Check(ctx.status, tc.Equals, charmRuntime.Status{Status: status, Message: "workload message"})
		output, err = get.Run([]string{"--format=json"})
		c.Assert(err, tc.IsNil)

		c.Check(string(output), tc.Equals, "\""+status+"\"\n")
	}

	for _, args := range [][]string{nil, {"invalid"}} {
		_, err := set.Run(args)
		c.Check(err, tc.NotNil)
		c.Check(ctx.status.Status, tc.Equals, "active")
	}

	for _, args := range [][]string{{"extra"}, {"--invalid"}, {"-f"}} {
		_, err := get.Run(args)
		c.Check(err, tc.NotNil, tc.Commentf("args: %q", args))
	}
}

// TestOpsUnitStatusFlags verifies Ops' explicit application=False invocation and
// ensures unsupported or malformed application requests cannot mutate unit status.
func (*statusSuite) TestOpsUnitStatusFlags(c *tc.C) {
	ctx := &testContext{}
	command, err := NewCommand(ctx, "status-set")
	c.Assert(err, tc.IsNil)

	for _, flag := range []string{"--application=False", "--application=false", "--application=0"} {
		_, err := command.Run([]string{flag, "active", "world"})
		c.Assert(err, tc.IsNil)

		c.Check(ctx.Status(), tc.Equals, charmRuntime.Status{Status: "active", Message: "world"})
	}
	for _, args := range [][]string{{"--application", "blocked"}, {"--application=True", "blocked"}, {"--application=invalid", "blocked"}, {"--application=False"}} {
		_, err := command.Run(args)
		c.Check(err, tc.NotNil)
		c.Check(ctx.Status(), tc.Equals, charmRuntime.Status{Status: "active", Message: "world"})
	}
	_, err = command.Run([]string{"--", "active", "--application=False"})
	c.Assert(err, tc.IsNil)

	c.Check(ctx.Status().Message, tc.Equals, "--application=False")
}
