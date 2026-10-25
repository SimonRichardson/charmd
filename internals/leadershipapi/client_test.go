package leadershipapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SimonRichardson/charmd/internals/config"
	"github.com/canonical/tc"
)

// Test connects focused tc suites to the Go runner.
func Test(t *testing.T) { tc.InternalTestingT(t) }

type clientSuite struct{}

var _ = tc.InternalSuite(&clientSuite{})

// TestGetContract verifies service path, headers, true and false results, and
// bounded decoding. Missing or malformed leadership must never become false success.
func (*clientSuite) TestGetContract(c *tc.C) {
	checksum := strings.Repeat("a", 64)
	body := `{"leader":true}`
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, "GET")
		c.Check(r.URL.Path, tc.Equals, "/base/v1/charms/"+checksum+"/leadership")
		c.Check(r.URL.RawQuery, tc.Equals, "")
		c.Check(r.Header.Get("Accept"), tc.Equals, "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	defer server.Close()
	client, err := New(config.Endpoint{URL: server.URL + "/base?ignored=yes"})
	c.Assert(err, tc.IsNil)

	for _, leader := range []bool{true, false} {
		body = fmt.Sprintf(`{"leader":%t}`, leader)
		got, err := client.Get(context.Background(), checksum)
		c.Assert(err, tc.IsNil)

		c.Check(got, tc.Equals, leader)
	}
	for _, invalid := range []string{`{}`, `null`, `[]`, `{"leader":null}`, `{"leader":"false"}`, `{"leader":true} {}`, strings.Repeat("x", 4*1024*1024+1)} {
		body = invalid
		_, err := client.Get(context.Background(), checksum)
		c.Check(err, tc.NotNil)
	}
	status = http.StatusServiceUnavailable
	_, err = client.Get(context.Background(), checksum)
	c.Check(err, tc.ErrorMatches, ".*503.*")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Get(ctx, checksum)
	c.Check(err, tc.ErrorIs, context.Canceled)
}

// TestInvalidSourcesAndIdentities verifies construction rejects ambiguous sources
// and malformed checksums are rejected before any network request.
func (*clientSuite) TestInvalidSourcesAndIdentities(c *tc.C) {
	for _, endpoint := range []config.Endpoint{{}, {URL: "ftp://host"}, {Path: "relative"}, {Path: "/file", URL: "http://host"}} {
		_, err := New(endpoint)
		c.Check(err, tc.NotNil)
	}
	client, err := New(config.Endpoint{URL: "http://127.0.0.1:1"})
	c.Assert(err, tc.IsNil)

	for _, checksum := range []string{"", strings.Repeat("z", 64), "../file"} {
		_, err := client.Get(context.Background(), checksum)
		c.Check(err, tc.ErrorMatches, "invalid charm SHA-256.*")
	}
	local, err := New(config.Endpoint{Path: "/leadership.yaml"})
	c.Assert(err, tc.IsNil)

	c.Check(local, tc.FitsTypeOf, &fileClient{})
}
