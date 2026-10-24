package charmapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/config"
)

func Test(t *testing.T) { tc.InternalTestingT(t) }

type clientSuite struct{}

var _ = tc.InternalSuite(&clientSuite{})

func (s *clientSuite) TestAPIClientDiscoversAndDownloadsCharm(c *tc.C) {
	archive := []byte("charm archive")
	digest := sha256.Sum256(archive)
	checksum := hex.EncodeToString(digest[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/charms/" + checksum:
			c.Check(request.Method, tc.Equals, http.MethodGet)
			c.Check(request.Header.Get("Accept"), tc.Equals, "application/json")
			w.Header().Set("Content-Type", "application/json")
			_, err := fmt.Fprintf(
				w,
				`{"name":"postgresql-k8s","sha256":%q,"download-url":%q}`,
				checksum, "http://"+request.Host+"/downloads/"+checksum,
			)
			c.Check(err, tc.IsNil)
		case "/downloads/" + checksum:
			_, err := w.Write(archive)
			c.Check(err, tc.IsNil)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	client, err := New(config.Endpoint{URL: server.URL})
	c.Assert(err, tc.IsNil)
	info, err := client.GetCharmDownloadInfo(c.Context(), checksum)
	c.Assert(err, tc.IsNil)
	destination := filepath.Join(c.MkDir(), "charm")
	c.Assert(client.DownloadCharm(c.Context(), info, destination), tc.IsNil)
	data, err := os.ReadFile(destination)
	c.Assert(err, tc.IsNil)
	c.Check(data, tc.DeepEquals, archive)
}

func (s *clientSuite) TestAPIClientRejectsMismatchedChecksum(c *tc.C) {
	checksum := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		response := `{"name":"postgresql-k8s","sha256":"bad",` +
			`"download-url":"https://downloads.example.test/charm"}`
		_, err := fmt.Fprint(w, response)
		c.Check(err, tc.IsNil)
	}))
	defer server.Close()
	client, err := New(config.Endpoint{URL: server.URL})
	c.Assert(err, tc.IsNil)
	_, err = client.GetCharmDownloadInfo(c.Context(), checksum)
	c.Check(err, tc.ErrorMatches, `discovered checksum "bad" does not match requested checksum .*`)
}

func (s *clientSuite) TestFileClientReadsManifestAndMovesArchive(c *tc.C) {
	directory := c.MkDir()
	archive := []byte("charm archive")
	digest := sha256.Sum256(archive)
	checksum := hex.EncodeToString(digest[:])
	c.Assert(os.WriteFile(filepath.Join(directory, checksum), archive, 0644), tc.IsNil)
	manifest := "name: postgresql-k8s\nmetadata:\n  revision: 42\n"
	c.Assert(os.WriteFile(filepath.Join(directory, checksum+".manifest"), []byte(manifest), 0644), tc.IsNil)

	client, err := New(config.Endpoint{Path: directory})
	c.Assert(err, tc.IsNil)
	info, err := client.GetCharmDownloadInfo(c.Context(), checksum)
	c.Assert(err, tc.IsNil)
	c.Check(info.Name, tc.Equals, "postgresql-k8s")
	c.Check(info.SHA256, tc.Equals, checksum)
	c.Check(info.Metadata, tc.DeepEquals, map[string]any{"revision": 42})
	destination := filepath.Join(c.MkDir(), "charm")
	c.Assert(client.DownloadCharm(c.Context(), info, destination), tc.IsNil)
	data, err := os.ReadFile(destination)
	c.Assert(err, tc.IsNil)
	c.Check(data, tc.DeepEquals, archive)
	_, err = os.Stat(filepath.Join(directory, checksum))
	c.Check(os.IsNotExist(err), tc.Equals, true)
}

func (s *clientSuite) TestFileClientRejectsMissingManifestWithoutMove(c *tc.C) {
	directory := c.MkDir()
	checksum := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	archivePath := filepath.Join(directory, checksum)
	c.Assert(os.WriteFile(archivePath, []byte("charm archive"), 0644), tc.IsNil)
	client, err := New(config.Endpoint{Path: directory})
	c.Assert(err, tc.IsNil)
	_, err = client.GetCharmDownloadInfo(c.Context(), checksum)
	c.Check(err, tc.ErrorMatches, `read charm manifest: .*`)
	_, err = os.Stat(archivePath)
	c.Check(err, tc.IsNil)
}
