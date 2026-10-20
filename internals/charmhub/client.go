// Package charmhub provides the minimal Charmhub client used by charmd.
package charmhub

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const defaultServerURL = "https://api.charmhub.io"

// Options configures a Client. Its zero value uses the public Charmhub API and
// the default HTTP client.
type Options struct {
	URL        string
	HTTPClient *http.Client
}

// Download identifies the archive returned by Charmhub's refresh endpoint.
type Download struct {
	URL    string
	SHA256 string
}

// DownloadOptions selects the charm revision and channel to download. Both
// selectors may be supplied together.
type DownloadOptions struct {
	Revision *int
	Channel  string
}

// Client gets charm download metadata from Charmhub.
type Client struct {
	refreshURL *url.URL
	httpClient *http.Client
}

// New constructs a Charmhub client.
func New(options *Options) (*Client, error) {
	serverURL := defaultServerURL
	httpClient := http.DefaultClient
	if options != nil {
		if options.URL != "" {
			serverURL = options.URL
		}
		if options.HTTPClient != nil {
			httpClient = options.HTTPClient
		}
	}

	endpoint, err := refreshURL(serverURL)
	if err != nil {
		return nil, err
	}
	return &Client{refreshURL: endpoint, httpClient: httpClient}, nil
}

// Endpoint returns the refresh endpoint used by the client.
func (c *Client) Endpoint() url.URL { return *c.refreshURL }

// Download resolves charm through Charmhub's refresh endpoint and returns its
// archive URL and SHA-256 checksum.
func (c *Client) Download(ctx context.Context, charm string, options DownloadOptions) (Download, error) {
	if err := validateDownloadRequest(charm, options); err != nil {
		return Download{}, err
	}
	instanceKey, err := newInstanceKey()
	if err != nil {
		return Download{}, err
	}
	request, err := c.downloadRequest(ctx, charm, instanceKey, options)
	if err != nil {
		return Download{}, err
	}
	response, err := c.refresh(request)
	if err != nil {
		return Download{}, err
	}
	return downloadFromRefresh(instanceKey, response)
}

func validateDownloadRequest(charm string, options DownloadOptions) error {
	if strings.TrimSpace(charm) == "" {
		return fmt.Errorf("charm name is required")
	}
	if charm != strings.TrimSpace(charm) {
		return fmt.Errorf("charm name must not have surrounding whitespace")
	}
	if options.Revision != nil && *options.Revision < 0 {
		return fmt.Errorf("charm revision must not be negative")
	}
	if options.Channel != strings.TrimSpace(options.Channel) {
		return fmt.Errorf("charm channel must not have surrounding whitespace")
	}
	return nil
}

func newInstanceKey() (string, error) {
	var key [16]byte
	if _, err := rand.Read(key[:]); err != nil {
		return "", fmt.Errorf("generate Charmhub instance key: %w", err)
	}
	return hex.EncodeToString(key[:]), nil
}

func (c *Client) downloadRequest(
	ctx context.Context, charm, instanceKey string, options DownloadOptions,
) (*http.Request, error) {
	body, err := json.Marshal(refreshRequest{
		Context: []any{},
		Actions: []refreshAction{{
			Action:      "download",
			InstanceKey: instanceKey,
			Name:        charm,
			Revision:    options.Revision,
			Channel:     options.Channel,
		}},
		Fields: []string{"download"},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal refresh request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.refreshURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create refresh request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

func (c *Client) refresh(request *http.Request) (refreshResponse, error) {
	response, err := c.httpClient.Do(request)
	if err != nil {
		return refreshResponse{}, fmt.Errorf("refresh charm: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return refreshResponse{}, fmt.Errorf(
			"refresh charm: unexpected HTTP status %s: %s",
			response.Status, strings.TrimSpace(string(data)),
		)
	}

	var result refreshResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return refreshResponse{}, fmt.Errorf("decode refresh response: %w", err)
	}
	return result, nil
}

func downloadFromRefresh(instanceKey string, response refreshResponse) (Download, error) {
	if len(response.ErrorList) != 0 {
		return Download{}, fmt.Errorf("refresh charm: %s", response.ErrorList[0].Message)
	}
	if len(response.Results) != 1 {
		return Download{}, fmt.Errorf("refresh charm: expected one result, got %d", len(response.Results))
	}

	refresh := response.Results[0]
	if refresh.InstanceKey != instanceKey {
		return Download{}, fmt.Errorf(
			"refresh charm: response instance key %q does not match %q",
			refresh.InstanceKey, instanceKey,
		)
	}
	if refresh.Result != "download" {
		return Download{}, fmt.Errorf("refresh charm: unexpected result %q", refresh.Result)
	}
	if err := validateDownload(refresh.Charm.Download); err != nil {
		return Download{}, fmt.Errorf("refresh charm: %w", err)
	}
	return Download{URL: refresh.Charm.Download.URL, SHA256: refresh.Charm.Download.SHA256}, nil
}

func refreshURL(rawURL string) (*url.URL, error) {
	if strings.TrimSpace(rawURL) != rawURL {
		return nil, fmt.Errorf("Charmhub URL must not have surrounding whitespace")
	}
	serverURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse Charmhub URL: %w", err)
	}
	if serverURL.Scheme != "http" && serverURL.Scheme != "https" {
		return nil, fmt.Errorf("Charmhub URL must use http or https")
	}
	if serverURL.Host == "" {
		return nil, fmt.Errorf("Charmhub URL must include a host")
	}
	if serverURL.User != nil {
		return nil, fmt.Errorf("Charmhub URL must not include user credentials")
	}
	if serverURL.Fragment != "" {
		return nil, fmt.Errorf("Charmhub URL must not include a fragment")
	}
	serverURL.Path = strings.TrimRight(serverURL.Path, "/") + "/v2/charms/refresh"
	serverURL.RawPath = ""
	serverURL.RawQuery = ""
	return serverURL, nil
}

func validateDownload(download refreshDownload) error {
	u, err := url.Parse(download.URL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid download URL %q", download.URL)
	}
	if len(download.SHA256) != 64 {
		return fmt.Errorf("invalid SHA-256 checksum %q", download.SHA256)
	}
	if _, err := hex.DecodeString(download.SHA256); err != nil {
		return fmt.Errorf("invalid SHA-256 checksum %q", download.SHA256)
	}
	return nil
}

type refreshRequest struct {
	Context []any           `json:"context"`
	Actions []refreshAction `json:"actions"`
	Fields  []string        `json:"fields"`
}

type refreshAction struct {
	Action      string `json:"action"`
	InstanceKey string `json:"instance-key"`
	Name        string `json:"name"`
	Revision    *int   `json:"revision,omitempty"`
	Channel     string `json:"channel,omitempty"`
}

type refreshResponse struct {
	Results   []refreshResult `json:"results"`
	ErrorList []refreshError  `json:"error-list"`
}

type refreshResult struct {
	InstanceKey string `json:"instance-key"`
	Result      string `json:"result"`
	Charm       struct {
		Download refreshDownload `json:"download"`
	} `json:"charm"`
}

type refreshDownload struct {
	URL    string `json:"url"`
	SHA256 string `json:"hash-sha-256"`
}

type refreshError struct {
	Message string `json:"message"`
}
