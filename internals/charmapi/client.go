// Package charmapi provides charmd's charm client.
package charmapi

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/SimonRichardson/charmd/internals/config"
)

// Client is the typed dependency used by charm task handlers. It uses either
// the internal charm client or a configured external API.
type Client struct {
	endpoint   *url.URL
	httpClient *http.Client
}

// New constructs a charm client from startup configuration. An empty endpoint
// selects the internal charm client.
func New(endpoint config.Endpoint) (*Client, error) {
	if endpoint.URL == "" {
		return NewInternal(), nil
	}
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

// NewInternal constructs the default internal charm client.
func NewInternal() *Client {
	return &Client{httpClient: &http.Client{}}
}

// Endpoint returns the configured external API base URL. It is empty when the
// client uses charmd's internal charm client.
func (c *Client) Endpoint() url.URL {
	if c.endpoint == nil {
		return url.URL{}
	}
	return *c.endpoint
}
