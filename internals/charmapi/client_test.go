package charmapi

import (
	"net/url"
	"testing"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/config"
)

func Test(t *testing.T) { tc.InternalTestingT(t) }

type clientSuite struct{}

var _ = tc.InternalSuite(&clientSuite{})

func (s *clientSuite) TestNew(c *tc.C) {
	client, err := New(config.Endpoint{URL: "https://agent.example.test/charm"})
	c.Assert(err, tc.IsNil)
	endpoint := client.Endpoint()
	c.Check(endpoint.String(), tc.Equals, "https://agent.example.test/charm")
}

func (s *clientSuite) TestNewUsesInternalClientForEmptyEndpoint(c *tc.C) {
	client, err := New(config.Endpoint{})
	c.Assert(err, tc.IsNil)
	c.Check(client.endpoint, tc.IsNil)
	c.Check(client.Endpoint(), tc.DeepEquals, url.URL{})
}

func (s *clientSuite) TestNewRejectsInvalidEndpoint(c *tc.C) {
	_, err := New(config.Endpoint{URL: "ftp://agent.example.test"})
	c.Check(err, tc.NotNil)
}
