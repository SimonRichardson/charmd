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
	CharmAPI      Endpoint `yaml:"charm-api"`
	ConfigAPI     Endpoint `yaml:"config-api"`
	LeadershipAPI Endpoint `yaml:"leadership-api"`
}

// Endpoint identifies an HTTP service or a local source path.
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
	for _, service := range []struct {
		name     string
		endpoint Endpoint
	}{
		{"charm-api", c.Services.CharmAPI},
		{"config-api", c.Services.ConfigAPI},
		{"leadership-api", c.Services.LeadershipAPI},
	} {
		endpoint := service.endpoint
		if endpoint.URL == "" && endpoint.Path == "" {
			return fmt.Errorf("services.%s: URL or path is required", service.name)
		}
		if endpoint.URL != "" && endpoint.Path != "" {
			return fmt.Errorf("services.%s: URL and path are mutually exclusive", service.name)
		}
		if endpoint.Path != "" {
			if !filepath.IsAbs(endpoint.Path) {
				return fmt.Errorf("services.%s.path: must be absolute", service.name)
			}
		} else if err := ValidateEndpoint(endpoint); err != nil {
			return fmt.Errorf("services.%s.url: %w", service.name, err)
		}
	}
	return nil
}

// ValidateEndpoint ensures an endpoint is usable by charmd.
func ValidateEndpoint(endpoint Endpoint) error {
	return validateHTTPURL(endpoint.URL)
}

// validateHTTPURL checks the URL requirements shared by configured HTTP services.
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
