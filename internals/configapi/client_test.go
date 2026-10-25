package configapi

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

// Test connects the focused suites to Go's test runner.
func Test(t *testing.T) { tc.InternalTestingT(t) }

type clientSuite struct{}

var _ = tc.InternalSuite(&clientSuite{})

// TestGet verifies the service base path, request headers, and independent snapshots.
func (*clientSuite) TestGet(c *tc.C) {
	checksum := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.URL.Path, tc.Equals, "/base/v1/charms/"+checksum+"/config")
		c.Check(r.URL.RawQuery, tc.Equals, "")
		c.Check(r.Header.Get("Accept"), tc.Equals, "application/json")
		fmt.Fprint(w, `{"greeting":"hello","count":2}`)
	}))
	defer server.Close()
	client, err := New(config.Endpoint{URL: server.URL + "/base?discard=true"})
	c.Assert(err, tc.IsNil)

	first, err := client.Get(context.Background(), checksum)
	c.Assert(err, tc.IsNil)

	first["greeting"] = "changed"
	second, err := client.Get(context.Background(), checksum)
	c.Assert(err, tc.IsNil)

	c.Check(second, tc.DeepEquals, map[string]any{"greeting": "hello", "count": float64(2)})
}

// TestInvalidEndpoint verifies unsupported services fail before any network activity.
func (*clientSuite) TestInvalidEndpoint(c *tc.C) {
	for _, endpoint := range []config.Endpoint{{}, {URL: "ftp://host"}, {URL: "http://user:secret@host"}, {Path: "relative"}} {
		_, err := New(endpoint)
		c.Check(err, tc.NotNil)
	}
}

// TestGetFailures checks malformed snapshots, statuses, identities, and cancellation.
func (*clientSuite) TestGetFailures(c *tc.C) {
	for _, body := range []string{"null", "[]", "invalid", "{} {}", strings.Repeat(" ", 4*1024*1024+1)} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		client, err := New(config.Endpoint{URL: server.URL})
		c.Assert(err, tc.IsNil)

		_, err = client.Get(context.Background(), strings.Repeat("a", 64))
		c.Check(err, tc.NotNil)
		server.Close()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	client, err := New(config.Endpoint{URL: server.URL})
	c.Assert(err, tc.IsNil)

	for _, checksum := range []string{"../bad", strings.Repeat("z", 64), strings.Repeat("a", 64)} {
		_, err := client.Get(context.Background(), checksum)
		c.Check(err, tc.NotNil)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Get(ctx, strings.Repeat("a", 64))
	c.Check(err, tc.NotNil)
}
