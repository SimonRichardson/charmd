package hookd

import (
	"errors"
	"github.com/canonical/tc"
)

type logSuite struct{}

var _ = tc.InternalSuite(&logSuite{})

type logContext struct {
	testContext
	messages []string
	err      error
}

// Log captures the command message and returns the configured destination error.
func (ctx *logContext) Log(message string) error {
	ctx.messages = append(ctx.messages, message)
	return ctx.err
}

// TestLog checks legacy log-level flags, message joining, and destination errors.
func (*logSuite) TestLog(c *tc.C) {
	ctx := &logContext{}
	command, err := NewCommand(ctx, "juju-log")
	c.Assert(err, tc.IsNil)

	for _, args := range [][]string{{"hello", "world"}, {"-l", "INFO", "hello", "world"}, {"--log-level", "DEBUG", "hello", "world"}, {"--log-level=WARNING", "--", "hello", "world"}} {
		_, err := command.Run(args)
		c.Assert(err, tc.IsNil)

		c.Check(ctx.messages[len(ctx.messages)-1], tc.Equals, "hello world")
	}

	before := len(ctx.messages)
	for _, args := range [][]string{nil, {"-l"}, {"--log-level"}, {"--"}, {"--log-level=INFO"}} {
		_, err := command.Run(args)
		c.Check(err, tc.NotNil, tc.Commentf("args: %q", args))
	}

	c.Check(ctx.messages, tc.HasLen, before)

	ctx.err = errors.New("log destination unavailable")
	_, err = command.Run([]string{"message"})
	c.Check(err, tc.ErrorIs, ctx.err)
}
