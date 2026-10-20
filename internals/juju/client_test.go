package juju

import (
	"testing"

	"github.com/canonical/tc"
)

func Test(t *testing.T) { tc.InternalTestingT(t) }

type clientSuite struct{}

var _ = tc.InternalSuite(&clientSuite{})

func (s *clientSuite) TestDownloadBuildsSHA256BlobURL(c *tc.C) {
	client, err := New(&Options{URL: "https://api.example.test/charm-store/"})
	c.Assert(err, tc.IsNil)
	checksum := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	download, err := client.Download(c.Context(), checksum)
	c.Assert(err, tc.IsNil)
	c.Check(download, tc.DeepEquals, Download{
		URL:    "https://api.example.test/charm-store/sha256/" + checksum,
		SHA256: checksum,
	})
}

func (s *clientSuite) TestDownloadRejectsInvalidChecksum(c *tc.C) {
	client, err := New(&Options{URL: "https://api.example.test"})
	c.Assert(err, tc.IsNil)
	_, err = client.Download(c.Context(), "bad")
	c.Check(err, tc.ErrorMatches, `invalid SHA-256 checksum "bad"`)
}

func (s *clientSuite) TestNewRejectsInvalidURLs(c *tc.C) {
	for _, rawURL := range []string{
		"",
		"ftp://api.example.test",
		"/relative",
		"https://user:password@api.example.test",
		"https://api.example.test#fragment",
		" https://api.example.test",
	} {
		client, err := New(&Options{URL: rawURL})
		c.Check(client, tc.IsNil)
		c.Check(err, tc.NotNil, tc.Commentf("URL %q", rawURL))
	}
}

func (s *clientSuite) TestEndpointDoesNotExposeMutableState(c *tc.C) {
	client, err := New(&Options{URL: "https://api.example.test/charm-store"})
	c.Assert(err, tc.IsNil)
	endpoint := client.Endpoint()
	endpoint.Host = "changed.example.test"
	endpoint = client.Endpoint()
	c.Check(endpoint.String(), tc.Equals, "https://api.example.test/charm-store")
}
