package hookd

import "github.com/canonical/tc"

type formatSuite struct{}

var _ = tc.InternalSuite(&formatSuite{})

// TestOutputFormats checks scalar and map representations without changing the supplied value.
func (*formatSuite) TestOutputFormats(c *tc.C) {
	value := map[string]any{"greeting": "hello"}
	for _, test := range []struct{ format, expected string }{
		{"json", "{\"greeting\":\"hello\"}\n"},
		{"yaml", "greeting: hello\n"},
		{"smart", "greeting: hello\n"},
	} {
		output, err := formatOutput(test.format, value)
		c.Assert(err, tc.IsNil)

		c.Check(string(output), tc.Equals, test.expected)
		c.Check(value, tc.DeepEquals, map[string]any{"greeting": "hello"})
	}

	output, err := formatOutput("smart", nil)
	c.Assert(err, tc.IsNil)

	c.Check(output, tc.HasLen, 0)
	_, err = formatOutput("xml", value)
	c.Check(err, tc.ErrorMatches, `unsupported output format "xml"`)
	_, err = formatOutput("json", make(chan int))
	c.Check(err, tc.NotNil)
}

// TestArgumentIsolation checks parsing preserves caller arguments and honors the explicit end of options.
func (*formatSuite) TestArgumentIsolation(c *tc.C) {
	args := []string{"-f", "json", "--", "--literal"}
	before := append([]string(nil), args...)
	format, positional, err := outputArgs(args)
	c.Assert(err, tc.IsNil)

	c.Check(format, tc.Equals, "json")
	c.Check(positional, tc.DeepEquals, []string{"--literal"})
	c.Check(args, tc.DeepEquals, before)

	for _, args := range [][]string{{"-f"}, {"--format"}, {"--unknown"}} {
		_, _, err := outputArgs(args)
		c.Check(err, tc.NotNil)
	}
}
