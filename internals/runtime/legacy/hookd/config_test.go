package hookd

import (
	"github.com/canonical/tc"
)

type configSuite struct{}

var _ = tc.InternalSuite(&configSuite{})

// TestConfigGet checks whole-config and single-key output, missing keys, and argument errors.
func (*configSuite) TestConfigGet(c *tc.C) {
	cases := []struct {
		args   []string
		output string
		error  string
	}{
		{nil, "greeting: hello\n", ""},
		{[]string{"-"}, "greeting: hello\n", ""},
		{[]string{"greeting"}, "hello\n", ""},
		{[]string{"missing"}, "", ""},
		{[]string{"--format=json"}, "{\"greeting\":\"hello\"}\n", ""},
		{[]string{"-f", "json", "greeting"}, "\"hello\"\n", ""},
		{[]string{"--format", "yaml", "greeting"}, "hello\n", ""},
		{[]string{"--format=json", "missing"}, "null\n", ""},
		{[]string{"--", "greeting"}, "hello\n", ""},
		{[]string{"greeting", "extra"}, "", "config-get accepts at most one key"},
		{[]string{"--format"}, "", "missing output format"},
		{[]string{"--unknown"}, "", `unsupported option "--unknown"`},
		{[]string{"--format=xml"}, "", `unsupported output format "xml"`},
	}
	command, err := NewCommand(&testContext{}, "config-get")
	c.Assert(err, tc.IsNil)

	for _, test := range cases {
		output, err := command.Run(test.args)
		if test.error != "" {
			c.Check(err, tc.ErrorMatches, test.error, tc.Commentf("args: %q", test.args))
		} else {
			c.Assert(err, tc.IsNil, tc.Commentf("args: %q", test.args))

			c.Check(string(output), tc.Equals, test.output, tc.Commentf("args: %q", test.args))
		}
	}
}

// TestReadsAndFailuresPreserveStatus checks configuration commands cannot change unrelated workload state.
func (*configSuite) TestReadsAndFailuresPreserveStatus(c *tc.C) {
	ctx := &testContext{}
	before := ctx.Status()
	command, err := NewCommand(ctx, "config-get")
	c.Assert(err, tc.IsNil)

	for _, args := range [][]string{nil, {"greeting"}, {"missing"}, {"extra", "argument"}, {"--invalid"}} {
		_, _ = command.Run(args)
		c.Check(ctx.Status(), tc.Equals, before)
		c.Check(ctx.Config(), tc.DeepEquals, map[string]any{"greeting": "hello"})
	}
}
