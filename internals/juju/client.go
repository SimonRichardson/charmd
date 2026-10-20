// Package juju provides the minimal Juju charm blob client used by charmd.
package juju

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"strings"
)

// Options configures a Client.
type Options struct {
	URL string
}

// Download identifies an archive in Juju's S3-compatible charm blob store.
type Download struct {
	URL    string
	SHA256 string
}

// Client resolves SHA-256 addressed charm archives from Juju.
type Client struct {
	endpoint *url.URL
}

// New constructs a Juju charm blob client.
func New(options *Options) (*Client, error) {
	if options == nil {
		return nil, fmt.Errorf("Juju URL is required")
	}
	endpoint, err := parseURL(options.URL)
	if err != nil {
		return nil, err
	}
	return &Client{endpoint: endpoint}, nil
}

// Endpoint returns a copy of the configured blob endpoint.
func (c *Client) Endpoint() url.URL { return *c.endpoint }

// Download resolves checksum to the S3-compatible sha256/<checksum> blob URL.
func (c *Client) Download(_ context.Context, checksum string) (Download, error) {
	if err := validateSHA256(checksum); err != nil {
		return Download{}, err
	}
	archiveURL := *c.endpoint
	archiveURL.Path = path.Join(archiveURL.Path, "sha256", checksum)
	archiveURL.RawPath = ""
	archiveURL.RawQuery = ""
	return Download{URL: archiveURL.String(), SHA256: checksum}, nil
}

func parseURL(rawURL string) (*url.URL, error) {
	if strings.TrimSpace(rawURL) != rawURL || rawURL == "" {
		return nil, fmt.Errorf("Juju URL is required and must not have surrounding whitespace")
	}
	endpoint, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse Juju URL: %w", err)
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, fmt.Errorf("Juju URL must use http or https")
	}
	if endpoint.Host == "" {
		return nil, fmt.Errorf("Juju URL must include a host")
	}
	if endpoint.User != nil {
		return nil, fmt.Errorf("Juju URL must not include user credentials")
	}
	if endpoint.Fragment != "" {
		return nil, fmt.Errorf("Juju URL must not include a fragment")
	}
	return endpoint, nil
}

func validateSHA256(checksum string) error {
	if len(checksum) != 64 {
		return fmt.Errorf("invalid SHA-256 checksum %q", checksum)
	}
	if _, err := hex.DecodeString(checksum); err != nil {
		return fmt.Errorf("invalid SHA-256 checksum %q", checksum)
	}
	return nil
}
