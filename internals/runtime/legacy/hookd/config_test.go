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

// configContext supplies unset options without changing the shared command fixture.
type configContext struct {
	testContext
	values map[string]any
}

// Config returns the fixture's persistent settings to detect accidental mutation.
func (c *configContext) Config() map[string]any { return c.values }

// TestAllOptionsPreserveContext verifies Juju's --all filtering semantics and
// that ordinary config-get never deletes unset options from subsequent queries.
func (*configSuite) TestAllOptionsPreserveContext(c *tc.C) {
	ctx := &configContext{values: map[string]any{"greeting": "hello", "unset": nil, "--all": "literal"}}
	command, err := NewCommand(ctx, "config-get")
	c.Assert(err, tc.IsNil)

	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"--format=json"}, "{\"--all\":\"literal\",\"greeting\":\"hello\"}\n"},
		{[]string{"--all", "--format=json"}, "{\"--all\":\"literal\",\"greeting\":\"hello\",\"unset\":null}\n"},
		{[]string{"-f", "json", "-a"}, "{\"--all\":\"literal\",\"greeting\":\"hello\",\"unset\":null}\n"},
		{[]string{"--format=json", "unset"}, "null\n"},
		{[]string{"--", "--all"}, "literal\n"},
	} {
		output, err := command.Run(test.args)
		c.Assert(err, tc.IsNil)

		c.Check(string(output), tc.Equals, test.want)
		c.Check(ctx.values, tc.HasLen, 3)
	}
	_, err = command.Run([]string{"--all", "greeting"})
	c.Check(err, tc.ErrorMatches, "config-get cannot use --all with a key")
	_, err = command.Run([]string{"--format", "--all"})
	c.Check(err, tc.ErrorMatches, `unsupported output format "--all"`)
}
