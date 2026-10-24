package config

import (
	"strings"
	"testing"

	"github.com/canonical/tc"
)

func Test(t *testing.T) { tc.InternalTestingT(t) }

type configSuite struct{}

var _ = tc.InternalSuite(&configSuite{})

func (s *configSuite) TestDecode(c *tc.C) {
	rawConfig := "services:\n  charm-api:\n" +
		"    url: https://agent.example.test/charm\n"
	config, err := Decode(strings.NewReader(rawConfig))
	c.Assert(err, tc.IsNil)
	c.Check(config.Services.CharmAPI.URL, tc.Equals, "https://agent.example.test/charm")
}

func (s *configSuite) TestDecodeLocalCharmAPI(c *tc.C) {
	config, err := Decode(strings.NewReader("services:\n  charm-api:\n    path: /srv/charms\n"))
	c.Assert(err, tc.IsNil)
	c.Check(config.Services.CharmAPI.Path, tc.Equals, "/srv/charms")
}

func (s *configSuite) TestDecodeRejectsInvalidCharmAPI(c *tc.C) {
	tests := []struct {
		name   string
		config string
		want   string
	}{
		{name: "relative", config: "services:\n  charm-api:\n    url: /v1/charms\n", want: "must use http or https"},
		{
			name: "credentials", config: "services:\n  charm-api:\n" +
				"    url: https://user:password@agent.example.test\n",
			want: "must not include user credentials",
		},
		{name: "missing source", config: "services: {}\n", want: "URL or path is required"},
		{name: "relative path", config: "services:\n  charm-api:\n    path: charms\n", want: "must be absolute"},
		{
			name: "both sources", config: "services:\n  charm-api:\n" +
				"    url: https://agent.example.test\n    path: /srv/charms\n",
			want: "mutually exclusive",
		},
		{
			name: "unknown field", config: "services:\n  charm-api:\n" +
				"    endpoint: https://agent.example.test\n",
			want: "field endpoint not found",
		},
	}
	for _, test := range tests {
		_, err := Decode(strings.NewReader(test.config))
		c.Check(err, tc.ErrorMatches, "(?s).*"+test.want+".*", tc.Commentf("case %q", test.name))
	}
}
