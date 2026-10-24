// Package config loads charmd's static configuration.
package config

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config contains services charmd may call while processing changes.
type Config struct {
	Services Services `yaml:"services"`
}

// Services names the APIs charmd calls.
type Services struct {
	CharmAPI Endpoint `yaml:"charm-api"`
}

// Endpoint identifies an HTTP discovery service or local charm source directory.
type Endpoint struct {
	URL  string `yaml:"url"`
	Path string `yaml:"path"`
}

// Load reads and validates configuration from path.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open configuration: %w", err)
	}
	defer f.Close()
	return Decode(f)
}

// Decode reads and validates configuration from r.
func Decode(r io.Reader) (*Config, error) {
	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("decode configuration: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &config, nil
}

// Validate ensures configured endpoints are usable by charmd.
func (c *Config) Validate() error {
	endpoint := c.Services.CharmAPI
	if endpoint.URL == "" && endpoint.Path == "" {
		return fmt.Errorf("services.charm-api: URL or path is required")
	}
	if endpoint.URL != "" && endpoint.Path != "" {
		return fmt.Errorf("services.charm-api: URL and path are mutually exclusive")
	}
	if endpoint.Path != "" {
		if !filepath.IsAbs(endpoint.Path) {
			return fmt.Errorf("services.charm-api.path: must be absolute")
		}
		return nil
	}
	if err := ValidateEndpoint(endpoint); err != nil {
		return fmt.Errorf("services.charm-api.url: %w", err)
	}
	return nil
}

// ValidateEndpoint ensures an endpoint is usable by charmd.
func ValidateEndpoint(endpoint Endpoint) error {
	return validateHTTPURL(endpoint.URL)
}

func validateHTTPURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("must use http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("must include a host")
	}
	if u.User != nil {
		return fmt.Errorf("must not include user credentials")
	}
	if u.Fragment != "" {
		return fmt.Errorf("must not include a fragment")
	}
	if strings.TrimSpace(rawURL) != rawURL {
		return fmt.Errorf("must not have surrounding whitespace")
	}
	return nil
}
