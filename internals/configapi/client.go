// Package configapi provides HTTP and local-file configuration service clients.
package configapi

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"time"

	"github.com/SimonRichardson/charmd/internals/config"
)

// Client fetches config snapshots and observes notifications supplied by the service.
type Client interface {
	// Get returns a complete set of overrides for a SHA-addressed charm.
	Get(context.Context, string) (map[string]any, error)
	// Watch blocks until the stream disconnects or its context is canceled.
	Watch(context.Context, func(string)) error
}

// apiClient owns the configured service address and its streaming transport.
type apiClient struct {
	base *url.URL
	http *http.Client
}

// New constructs an HTTP client or a local file client using the configured source.
func New(endpoint config.Endpoint) (Client, error) {
	if endpoint.Path != "" {
		if endpoint.URL != "" {
			return nil, fmt.Errorf("config API URL and path are mutually exclusive")
		}
		if !filepath.IsAbs(endpoint.Path) {
			return nil, fmt.Errorf("config API path must be absolute")
		}
		return &fileClient{path: endpoint.Path}, nil
	}
	if err := config.ValidateEndpoint(endpoint); err != nil {
		return nil, fmt.Errorf("config API URL: %w", err)
	}
	base, err := url.Parse(endpoint.URL)
	if err != nil {
		return nil, err
	}
	return &apiClient{base: base, http: &http.Client{}}, nil
}

// request starts an HTTP GET beneath the service base path without inheriting URL queries.
func (c *apiClient) request(ctx context.Context, accept string, parts ...string) (*http.Response, error) {
	endpoint := *c.base
	endpoint.Path = path.Join(append([]string{endpoint.Path}, parts...)...)
	endpoint.RawPath, endpoint.RawQuery = "", ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	response, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("config API: unexpected HTTP status %s", response.Status)
	}
	return response, nil
}

// Get fetches a bounded JSON object. Its deadline does not limit the SSE stream.
func (c *apiClient) Get(ctx context.Context, checksum string) (map[string]any, error) {
	if err := validateChecksum(checksum); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := c.request(ctx, "application/json", "v1", "charms", checksum, "config")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	const limit = 4 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("config response exceeds %d bytes", limit)
	}
	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if values == nil {
		return nil, fmt.Errorf("config response must be a JSON object")
	}
	return values, nil
}

// validateChecksum prevents malformed identities from being used in paths or notifications.
func validateChecksum(checksum string) error {
	if len(checksum) != 64 {
		return fmt.Errorf("invalid charm SHA-256 %q", checksum)
	}
	if _, err := hex.DecodeString(checksum); err != nil {
		return fmt.Errorf("invalid charm SHA-256 %q", checksum)
	}
	return nil
}
