package charmhub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canonical/tc"
)

func Test(t *testing.T) { tc.InternalTestingT(t) }

type clientSuite struct{}

var _ = tc.InternalSuite(&clientSuite{})

func (s *clientSuite) TestDownload(c *tc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		c.Check(request.Method, tc.Equals, http.MethodPost)
		c.Check(request.URL.Path, tc.Equals, "/api/v2/charms/refresh")
		c.Check(request.Header.Get("Accept"), tc.Equals, "application/json")
		c.Check(request.Header.Get("Content-Type"), tc.Equals, "application/json")

		var body refreshRequest
		c.Assert(json.NewDecoder(request.Body).Decode(&body), tc.IsNil)
		c.Check(body.Context, tc.DeepEquals, []any{})
		c.Assert(body.Actions, tc.HasLen, 1)
		c.Check(body.Actions[0].Action, tc.Equals, "download")
		c.Check(body.Actions[0].InstanceKey, tc.Not(tc.Equals), "postgresql-k8s")
		c.Check(len(body.Actions[0].InstanceKey), tc.Equals, 32)
		c.Check(body.Actions[0].Name, tc.Equals, "postgresql-k8s")
		c.Check(body.Actions[0].Revision, tc.DeepEquals, intPtr(42))
		c.Check(body.Actions[0].Channel, tc.Equals, "14/stable")
		c.Check(body.Fields, tc.DeepEquals, []string{"download"})

		_, err := fmt.Fprintf(w, `{"results":[{"instance-key":%q,"result":"download","charm":{"download":{"url":"https://downloads.example.test/postgresql-k8s.charm","hash-sha-256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}}]}`, body.Actions[0].InstanceKey)
		c.Check(err, tc.IsNil)
	}))
	defer server.Close()

	client, err := New(&Options{URL: server.URL + "/api/", HTTPClient: server.Client()})
	c.Assert(err, tc.IsNil)
	download, err := client.Download(c.Context(), "postgresql-k8s", DownloadOptions{
		Revision: intPtr(42),
		Channel:  "14/stable",
	})
	c.Assert(err, tc.IsNil)
	c.Check(download, tc.DeepEquals, Download{
		URL:    "https://downloads.example.test/postgresql-k8s.charm",
		SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
}

func (s *clientSuite) TestEndpointUsesDefaultAndDoesNotExposeMutableState(c *tc.C) {
	client, err := New(nil)
	c.Assert(err, tc.IsNil)
	endpoint := client.Endpoint()
	c.Check(endpoint.String(), tc.Equals, defaultServerURL+"/v2/charms/refresh")
	endpoint.Host = "changed.example.test"
	endpoint = client.Endpoint()
	c.Check(endpoint.String(), tc.Equals, defaultServerURL+"/v2/charms/refresh")
}

func (s *clientSuite) TestNewRejectsInvalidURL(c *tc.C) {
	for _, rawURL := range []string{
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

func (s *clientSuite) TestDownloadRejectsInvalidCharmWithoutRequest(c *tc.C) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	client, err := New(&Options{URL: server.URL, HTTPClient: server.Client()})
	c.Assert(err, tc.IsNil)
	for _, charm := range []string{"", " \t", " postgresql-k8s", "postgresql-k8s\n"} {
		download, err := client.Download(c.Context(), charm, DownloadOptions{})
		c.Check(download, tc.DeepEquals, Download{})
		c.Check(err, tc.NotNil, tc.Commentf("charm %q", charm))
	}
	c.Check(requests, tc.Equals, 0)
}

func (s *clientSuite) TestDownloadRejectsInvalidResponses(c *tc.C) {
	tests := []struct {
		name string
		body func(string) string
		want string
	}{
		{name: "no results", body: func(string) string { return `{}` }, want: "expected one result, got 0"},
		{name: "multiple results", body: func(string) string { return `{"results":[{},{}]}` }, want: "expected one result, got 2"},
		{name: "different instance key", body: func(string) string { return `{"results":[{"instance-key":"other","result":"download"}]}` }, want: `response instance key "other" does not match`},
		{name: "unexpected result", body: func(key string) string {
			return fmt.Sprintf(`{"results":[{"instance-key":%q,"result":"install"}]}`, key)
		}, want: `unexpected result "install"`},
		{name: "missing download", body: func(key string) string {
			return fmt.Sprintf(`{"results":[{"instance-key":%q,"result":"download","charm":{"download":{}}}]}`, key)
		}, want: `invalid download URL ""`},
		{name: "invalid checksum", body: func(key string) string {
			return fmt.Sprintf(`{"results":[{"instance-key":%q,"result":"download","charm":{"download":{"url":"https://downloads.example.test/charm","hash-sha-256":"bad"}}}]}`, key)
		}, want: `invalid SHA-256 checksum "bad"`},
		{name: "API error", body: func(string) string { return `{"error-list":[{"message":"charm not found"}]}` }, want: "charm not found"},
	}

	for _, test := range tests {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			c.Check(request.URL.Path, tc.Equals, "/v2/charms/refresh")
			var body refreshRequest
			c.Assert(json.NewDecoder(request.Body).Decode(&body), tc.IsNil)
			c.Assert(body.Actions, tc.HasLen, 1)
			_, err := w.Write([]byte(test.body(body.Actions[0].InstanceKey)))
			c.Check(err, tc.IsNil)
		}))
		client, err := New(&Options{URL: server.URL, HTTPClient: server.Client()})
		c.Assert(err, tc.IsNil)
		download, err := client.Download(c.Context(), "postgresql-k8s", DownloadOptions{})
		server.Close()

		c.Check(download, tc.DeepEquals, Download{}, tc.Commentf("case %q", test.name))
		c.Check(err, tc.ErrorMatches, ".*"+test.want+".*", tc.Commentf("case %q", test.name))
	}
}

func (s *clientSuite) TestDownloadRejectsUnexpectedHTTPStatus(c *tc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := New(&Options{URL: server.URL, HTTPClient: server.Client()})
	c.Assert(err, tc.IsNil)
	download, err := client.Download(c.Context(), "postgresql-k8s", DownloadOptions{})
	c.Check(download, tc.DeepEquals, Download{})
	c.Check(err, tc.ErrorMatches, ".*unexpected HTTP status 503 Service Unavailable: unavailable")
}

func (s *clientSuite) TestDownloadRejectsInvalidSelectorsWithoutRequest(c *tc.C) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	client, err := New(&Options{URL: server.URL, HTTPClient: server.Client()})
	c.Assert(err, tc.IsNil)
	for _, options := range []DownloadOptions{
		{Revision: intPtr(-1)},
		{Channel: " latest/stable"},
		{Channel: "latest/stable\n"},
	} {
		download, err := client.Download(c.Context(), "postgresql-k8s", options)
		c.Check(download, tc.DeepEquals, Download{})
		c.Check(err, tc.NotNil)
	}
	c.Check(requests, tc.Equals, 0)
}

func intPtr(value int) *int { return &value }
