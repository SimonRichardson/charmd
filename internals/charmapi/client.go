// Package charmapi provides the client for the Juju-provided charm API.
package charmapi

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/SimonRichardson/charmd/internals/config"
)

// Client is the typed dependency used by charm task handlers to access the
// Juju-provided charm API.
type Client struct {
	endpoint   *url.URL
	httpClient *http.Client
}

// New constructs a charm API client from validated startup configuration.
func New(endpoint config.Endpoint) (*Client, error) {
	if err := config.ValidateEndpoint(endpoint); err != nil {
		return nil, fmt.Errorf("charm API endpoint: %w", err)
	}
	u, err := url.Parse(endpoint.URL)
	if err != nil {
		return nil, fmt.Errorf("parse charm API endpoint: %w", err)
	}
	return &Client{
		endpoint:   u,
		httpClient: &http.Client{},
	}, nil
}

// Endpoint returns the configured base URL for the charm API.
func (c *Client) Endpoint() url.URL { return *c.endpoint }
