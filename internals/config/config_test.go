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
		"    url: https://agent.example.test/charm\n  config-api:\n    path: /srv/config.yaml\n"
	config, err := Decode(strings.NewReader(rawConfig))
	c.Assert(err, tc.IsNil)

	c.Check(config.Services.CharmAPI.URL, tc.Equals, "https://agent.example.test/charm")
}

func (s *configSuite) TestDecodeLocalCharmAPI(c *tc.C) {
	config, err := Decode(strings.NewReader("services:\n  charm-api:\n    path: /srv/charms\n  config-api:\n    path: /srv/config.yaml\n"))
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

// TestConfigAPIService verifies the required service is loaded independently of the charm source.
func (*configSuite) TestConfigAPIService(c *tc.C) {
	cfg, err := Decode(strings.NewReader("services:\n  charm-api:\n    path: /srv/charms\n  config-api:\n    url: https://config.example.test/base\n"))
	c.Assert(err, tc.IsNil)

	c.Check(cfg.Services.ConfigAPI.URL, tc.Equals, "https://config.example.test/base")
	c.Check(cfg.Services.CharmAPI.Path, tc.Equals, "/srv/charms")
}

// TestInvalidConfigAPIService verifies only HTTP endpoints can be configured.
func (*configSuite) TestInvalidConfigAPIService(c *tc.C) {
	for _, service := range []string{"path: relative", "url: ftp://config.example.test", "url: https://user:secret@config.example.test", "url: /relative"} {
		_, err := Decode(strings.NewReader("services:\n  charm-api:\n    path: /srv/charms\n  config-api:\n    " + service + "\n"))
		c.Check(err, tc.ErrorMatches, ".*services.config-api.*")
	}
}

// TestLocalConfigAPIService verifies local and HTTP config sources are mutually exclusive.
func (*configSuite) TestLocalConfigAPIService(c *tc.C) {
	cfg, err := Decode(strings.NewReader("services:\n  charm-api:\n    path: /srv/charms\n  config-api:\n    path: /srv/config.yaml\n"))
	c.Assert(err, tc.IsNil)

	c.Check(cfg.Services.ConfigAPI.Path, tc.Equals, "/srv/config.yaml")
	_, err = Decode(strings.NewReader("services:\n  charm-api:\n    path: /srv/charms\n  config-api:\n    path: /srv/config.yaml\n    url: http://config.example.test\n"))
	c.Check(err, tc.ErrorMatches, ".*services.config-api.*mutually exclusive")
}

// TestMissingConfigAPIService verifies startup requires an explicit config source.
func (*configSuite) TestMissingConfigAPIService(c *tc.C) {
	for _, section := range []string{"", "  config-api: {}\n", "  config-api:\n"} {
		_, err := Decode(strings.NewReader("services:\n  charm-api:\n    path: /srv/charms\n" + section))
		c.Check(err, tc.ErrorMatches, "services.config-api: URL or path is required")
	}
}
