package charmapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/config"
)

func Test(t *testing.T) { tc.InternalTestingT(t) }

type clientSuite struct{}

var _ = tc.InternalSuite(&clientSuite{})

func (s *clientSuite) TestNew(c *tc.C) {
	client, err := New(config.Endpoint{Type: BackendJuju, URL: "https://agent.example.test/charm"})
	c.Assert(err, tc.IsNil)
	juju, ok := client.(*jujuCharmClient)
	c.Assert(ok, tc.Equals, true)
	endpoint := juju.client.Endpoint()
	c.Check(endpoint.String(), tc.Equals, "https://agent.example.test/charm")
}

func (s *clientSuite) TestNewUsesCharmhubForEmptyEndpoint(c *tc.C) {
	client, err := New(config.Endpoint{})
	c.Assert(err, tc.IsNil)
	_, ok := client.(*charmhubClient)
	c.Check(ok, tc.Equals, true)
}

func (s *clientSuite) TestNewRejectsInvalidEndpoint(c *tc.C) {
	_, err := New(config.Endpoint{Type: BackendJuju, URL: "ftp://agent.example.test"})
	c.Check(err, tc.NotNil)
}

func (s *clientSuite) TestNewRejectsUnknownBackend(c *tc.C) {
	_, err := New(config.Endpoint{Type: "other"})
	c.Check(err, tc.ErrorMatches, `unknown charm API type "other"`)
}

func (s *clientSuite) TestCharmhubRejectsResolvedChecksumDifferentFromRequest(c *tc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload struct {
			Actions []struct {
				InstanceKey string `json:"instance-key"`
			} `json:"actions"`
		}
		c.Assert(json.NewDecoder(request.Body).Decode(&payload), tc.IsNil)
		c.Assert(payload.Actions, tc.HasLen, 1)
		response := `{"results":[{"instance-key":%q,"result":"download",` +
			`"charm":{"download":{"url":"https://downloads.example.test/postgresql-k8s.charm",` +
			`"hash-sha-256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}}]}`
		_, err := fmt.Fprintf(w, response, payload.Actions[0].InstanceKey)
		c.Check(err, tc.IsNil)
	}))
	defer server.Close()

	client, err := New(config.Endpoint{Type: BackendCharmhub, URL: server.URL})
	c.Assert(err, tc.IsNil)
	_, err = client.GetCharmDownloadInfo(c.Context(), DownloadRequest{
		Name:   "postgresql-k8s",
		SHA256: strings.Repeat("f", 64),
	})
	c.Check(err, tc.ErrorMatches, `Charmhub checksum .* does not match requested checksum .*`)
}

func (s *clientSuite) TestDownloadCharmChecksChecksumBeforeRename(c *tc.C) {
	archive := []byte("charm archive")
	checksumBytes := sha256.Sum256(archive)
	checksum := hex.EncodeToString(checksumBytes[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		c.Check(request.Method, tc.Equals, http.MethodGet)
		_, err := w.Write(archive)
		c.Check(err, tc.IsNil)
	}))
	defer server.Close()

	destination := filepath.Join(c.MkDir(), "postgresql-k8s.charm")
	c.Assert(downloadCharm(c.Context(), server.Client(), server.URL, checksum, destination), tc.IsNil)
	data, err := os.ReadFile(destination)
	c.Assert(err, tc.IsNil)
	c.Check(data, tc.DeepEquals, archive)

	err = downloadCharm(c.Context(), server.Client(), server.URL, strings.Repeat("0", 64), destination)
	c.Check(err, tc.ErrorMatches, `downloaded charm checksum does not match .*`)
	data, err = os.ReadFile(destination)
	c.Assert(err, tc.IsNil)
	c.Check(data, tc.DeepEquals, archive)
}

func (s *clientSuite) TestDownloadCharmRejectsInvalidInputsWithoutRequest(c *tc.C) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()
	destination := filepath.Join(c.MkDir(), "postgresql-k8s.charm")
	checksum := strings.Repeat("0", 64)

	err := downloadCharm(c.Context(), server.Client(), "ftp://downloads.example.test/charm", checksum, destination)
	c.Check(err, tc.ErrorMatches, `invalid charm download URL .*`)
	err = downloadCharm(c.Context(), server.Client(), server.URL, "bad", destination)
	c.Check(err, tc.ErrorMatches, `invalid SHA-256 checksum "bad"`)
	c.Check(requests, tc.Equals, 0)
	_, err = os.Stat(destination)
	c.Check(os.IsNotExist(err), tc.Equals, true)
}

func (s *clientSuite) TestDownloadCharmRejectsHTTPFailureWithoutCreatingFile(c *tc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	destination := filepath.Join(c.MkDir(), "postgresql-k8s.charm")

	err := downloadCharm(c.Context(), server.Client(), server.URL, strings.Repeat("0", 64), destination)
	c.Check(err, tc.ErrorMatches, `download charm: unexpected HTTP status 503 Service Unavailable`)
	_, err = os.Stat(destination)
	c.Check(os.IsNotExist(err), tc.Equals, true)
}
