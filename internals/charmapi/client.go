// Package charmapi provides configured charm download clients.
package charmapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/SimonRichardson/charmd/internals/charmhub"
	"github.com/SimonRichardson/charmd/internals/config"
	"github.com/SimonRichardson/charmd/internals/juju"
)

const (
	// BackendCharmhub selects Charmhub's refresh API.
	BackendCharmhub = "charmhub"
	// BackendJuju selects Juju's S3-compatible charm blob endpoint.
	BackendJuju = "juju"
)

// DownloadRequest identifies the charm archive to fetch. The supported
// selectors depend on the configured backend.
type DownloadRequest struct {
	// Name is the charm name used by backends that resolve charm metadata.
	Name string
	// SHA256 optionally constrains Charmhub and is required by Juju.
	SHA256 string
	// Revision optionally selects a Charmhub revision.
	Revision *int
	// Channel optionally selects a Charmhub channel.
	Channel string
}

// DownloadInfo identifies a resolved charm archive and its expected checksum.
type DownloadInfo struct {
	// URL is the absolute archive URL returned by the selected backend.
	URL string
	// SHA256 is the expected archive SHA-256 checksum.
	SHA256 string
}

// CharmClient resolves charm archives and downloads verified archive files.
type CharmClient interface {
	// GetCharmDownloadInfo resolves the requested charm to an archive URL and
	// its expected SHA-256 checksum.
	GetCharmDownloadInfo(ctx context.Context, request DownloadRequest) (DownloadInfo, error)
	// DownloadCharm downloads downloadURL to destination and only replaces the
	// destination when its SHA-256 checksum matches checksum.
	DownloadCharm(ctx context.Context, downloadURL, checksum, destination string) error
}

// New constructs the charm client selected by configuration. An omitted type
// selects Charmhub.
func New(endpoint config.Endpoint) (CharmClient, error) {
	switch endpoint.Type {
	case "", BackendCharmhub:
		client, err := charmhub.New(&charmhub.Options{URL: endpoint.URL})
		if err != nil {
			return nil, fmt.Errorf("create Charmhub client: %w", err)
		}
		return &charmhubClient{client: client, httpClient: http.DefaultClient}, nil
	case BackendJuju:
		if err := config.ValidateEndpoint(endpoint); err != nil {
			return nil, fmt.Errorf("Juju endpoint: %w", err)
		}
		client, err := juju.New(&juju.Options{URL: endpoint.URL})
		if err != nil {
			return nil, fmt.Errorf("create Juju client: %w", err)
		}
		return &jujuCharmClient{client: client, httpClient: http.DefaultClient}, nil
	default:
		return nil, fmt.Errorf("unknown charm API type %q", endpoint.Type)
	}
}

type charmhubClient struct {
	client     *charmhub.Client
	httpClient *http.Client
}

var _ CharmClient = (*charmhubClient)(nil)

// GetCharmDownloadInfo resolves a Charmhub archive for request.
func (c *charmhubClient) GetCharmDownloadInfo(
	ctx context.Context, request DownloadRequest,
) (DownloadInfo, error) {
	download, err := c.client.Download(ctx, request.Name, charmhub.DownloadOptions{
		Revision: request.Revision,
		Channel:  request.Channel,
	})
	if err != nil {
		return DownloadInfo{}, err
	}
	if request.SHA256 != "" && !strings.EqualFold(request.SHA256, download.SHA256) {
		return DownloadInfo{}, fmt.Errorf(
			"Charmhub checksum %q does not match requested checksum %q",
			download.SHA256, request.SHA256,
		)
	}
	return DownloadInfo{URL: download.URL, SHA256: download.SHA256}, nil
}

// DownloadCharm downloads and verifies a Charmhub archive.
func (c *charmhubClient) DownloadCharm(
	ctx context.Context, downloadURL, checksum, destination string,
) error {
	return downloadCharm(ctx, c.httpClient, downloadURL, checksum, destination)
}

type jujuCharmClient struct {
	client     *juju.Client
	httpClient *http.Client
}

var _ CharmClient = (*jujuCharmClient)(nil)

// GetCharmDownloadInfo resolves a Juju archive for request.
func (c *jujuCharmClient) GetCharmDownloadInfo(
	ctx context.Context, request DownloadRequest,
) (DownloadInfo, error) {
	download, err := c.client.Download(ctx, request.SHA256)
	if err != nil {
		return DownloadInfo{}, fmt.Errorf("Juju charm checksum: %w", err)
	}
	return DownloadInfo{URL: download.URL, SHA256: download.SHA256}, nil
}

// DownloadCharm downloads and verifies a Juju archive.
func (c *jujuCharmClient) DownloadCharm(
	ctx context.Context, downloadURL, checksum, destination string,
) error {
	return downloadCharm(ctx, c.httpClient, downloadURL, checksum, destination)
}

func downloadCharm(ctx context.Context, client *http.Client, downloadURL, checksum, destination string) error {
	if err := validateDownloadURL(downloadURL); err != nil {
		return err
	}
	if err := validateSHA256(checksum); err != nil {
		return err
	}
	if destination == "" {
		return fmt.Errorf("charm download destination is required")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("create charm download request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download charm: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("download charm: unexpected HTTP status %s", response.Status)
	}

	temporary, err := os.CreateTemp(filepath.Dir(destination), "."+filepath.Base(destination)+"-")
	if err != nil {
		return fmt.Errorf("create temporary charm download: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	digest := sha256.New()
	if _, err := io.Copy(io.MultiWriter(temporary, digest), response.Body); err != nil {
		temporary.Close()
		return fmt.Errorf("write charm download: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close charm download: %w", err)
	}
	if !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), checksum) {
		return fmt.Errorf("downloaded charm checksum does not match %q", checksum)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("store charm download: %w", err)
	}
	return nil
}

func validateDownloadURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid charm download URL %q", rawURL)
	}
	return nil
}

func validateSHA256(checksum string) error {
	if len(checksum) != sha256.Size*2 {
		return fmt.Errorf("invalid SHA-256 checksum %q", checksum)
	}
	if _, err := hex.DecodeString(checksum); err != nil {
		return fmt.Errorf("invalid SHA-256 checksum %q", checksum)
	}
	return nil
}
