// Package charmapi provides Charmd's SHA-addressed charm discovery clients.
package charmapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/SimonRichardson/charmd/internals/config"
)

// DownloadInfo identifies a discovered charm archive.
type DownloadInfo struct {
	Name        string         `json:"name" yaml:"name"`
	SHA256      string         `json:"sha256" yaml:"sha256"`
	DownloadURL string         `json:"download-url,omitempty" yaml:"download-url,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// CharmClient discovers SHA-addressed charm archives and places them in the
// Charmd archive cache.
type CharmClient interface {
	// GetCharmDownloadInfo resolves sha256 to charm metadata and download details.
	GetCharmDownloadInfo(ctx context.Context, sha256 string) (DownloadInfo, error)
	// DownloadCharm places info's archive at destination and verifies its checksum.
	DownloadCharm(ctx context.Context, info DownloadInfo, destination string) error
}

// New constructs a configured HTTP discovery client or a file-backed client.
func New(endpoint config.Endpoint) (CharmClient, error) {
	if endpoint.URL != "" {
		baseURL, err := url.Parse(endpoint.URL)
		if err != nil {
			return nil, fmt.Errorf("parse charm API URL: %w", err)
		}
		return &apiClient{baseURL: baseURL, httpClient: http.DefaultClient}, nil
	}
	return &fileClient{directory: endpoint.Path}, nil
}

type apiClient struct {
	baseURL    *url.URL
	httpClient *http.Client
}

var _ CharmClient = (*apiClient)(nil)

// GetCharmDownloadInfo retrieves discovery metadata from the configured service.
func (c *apiClient) GetCharmDownloadInfo(ctx context.Context, checksum string) (DownloadInfo, error) {
	if err := validateSHA256(checksum); err != nil {
		return DownloadInfo{}, err
	}
	endpoint := *c.baseURL
	endpoint.Path = path.Join(endpoint.Path, "v1", "charms", checksum)
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return DownloadInfo{}, fmt.Errorf("create charm discovery request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return DownloadInfo{}, fmt.Errorf("get charm download info: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return DownloadInfo{}, fmt.Errorf(
			"get charm download info: unexpected HTTP status %s: %s",
			response.Status, strings.TrimSpace(string(data)),
		)
	}
	var info DownloadInfo
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		return DownloadInfo{}, fmt.Errorf("decode charm download info: %w", err)
	}
	if info.SHA256 != checksum {
		return DownloadInfo{}, fmt.Errorf(
			"discovered checksum %q does not match requested checksum %q", info.SHA256, checksum,
		)
	}
	if strings.TrimSpace(info.Name) == "" {
		return DownloadInfo{}, fmt.Errorf("discovered charm name is required")
	}
	if err := validateDownloadURL(info.DownloadURL); err != nil {
		return DownloadInfo{}, err
	}
	return info, nil
}

// DownloadCharm downloads a discovered archive into destination.
func (c *apiClient) DownloadCharm(ctx context.Context, info DownloadInfo, destination string) error {
	return downloadCharm(ctx, c.httpClient, info, destination)
}

type fileClient struct {
	directory string
}

var _ CharmClient = (*fileClient)(nil)

// GetCharmDownloadInfo reads the manifest beside a local SHA-addressed archive.
func (c *fileClient) GetCharmDownloadInfo(_ context.Context, checksum string) (DownloadInfo, error) {
	if err := validateSHA256(checksum); err != nil {
		return DownloadInfo{}, err
	}
	data, err := os.ReadFile(filepath.Join(c.directory, checksum+".manifest"))
	if err != nil {
		return DownloadInfo{}, fmt.Errorf("read charm manifest: %w", err)
	}
	var info DownloadInfo
	if err := yaml.Unmarshal(data, &info); err != nil {
		return DownloadInfo{}, fmt.Errorf("decode charm manifest: %w", err)
	}
	if strings.TrimSpace(info.Name) == "" {
		return DownloadInfo{}, fmt.Errorf("charm manifest name is required")
	}
	info.SHA256 = checksum
	return info, nil
}

// DownloadCharm moves a local SHA-addressed archive into destination.
func (c *fileClient) DownloadCharm(_ context.Context, info DownloadInfo, destination string) error {
	if err := validateSHA256(info.SHA256); err != nil {
		return err
	}
	if destination == "" {
		return fmt.Errorf("charm download destination is required")
	}
	if err := os.Rename(filepath.Join(c.directory, info.SHA256), destination); err != nil {
		return fmt.Errorf("move local charm archive: %w", err)
	}
	return nil
}

// downloadCharm streams an archive into a temporary file and verifies its digest before publishing it.
func downloadCharm(ctx context.Context, client *http.Client, info DownloadInfo, destination string) error {
	if err := validateSHA256(info.SHA256); err != nil {
		return err
	}
	if err := validateDownloadURL(info.DownloadURL); err != nil {
		return err
	}
	if destination == "" {
		return fmt.Errorf("charm download destination is required")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, info.DownloadURL, nil)
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
	if !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), info.SHA256) {
		return fmt.Errorf("downloaded charm checksum does not match %q", info.SHA256)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("store charm download: %w", err)
	}
	return nil
}

// validateSHA256 rejects checksums that are not 64 hexadecimal characters.
func validateSHA256(checksum string) error {
	if len(checksum) != sha256.Size*2 {
		return fmt.Errorf("invalid SHA-256 checksum %q", checksum)
	}
	if _, err := hex.DecodeString(checksum); err != nil {
		return fmt.Errorf("invalid SHA-256 checksum %q", checksum)
	}
	return nil
}

// validateDownloadURL requires an absolute HTTP or HTTPS download URL.
func validateDownloadURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid charm download URL %q", rawURL)
	}
	return nil
}
