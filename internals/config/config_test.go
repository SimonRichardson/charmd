package config

import (
	"strings"
	"testing"
)

func TestDecode(t *testing.T) {
	config, err := Decode(strings.NewReader("services:\n  charm-api:\n    url: https://agent.example.test/charm\n"))
	if err != nil {
		t.Fatal(err)
	}
	if config.Services.CharmAPI.URL != "https://agent.example.test/charm" {
		t.Errorf("charm API URL = %q", config.Services.CharmAPI.URL)
	}
}

func TestDecodeRejectsInvalidCharmAPI(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   string
	}{
		{name: "missing", config: "services: {}\n", want: "services.charm-api.url: is required"},
		{name: "relative", config: "services:\n  charm-api:\n    url: /v1/charms\n", want: "must use http or https"},
		{name: "credentials", config: "services:\n  charm-api:\n    url: https://user:password@agent.example.test\n", want: "must not include user credentials"},
		{name: "unknown field", config: "services:\n  charm-api:\n    endpoint: https://agent.example.test\n", want: "field endpoint not found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(strings.NewReader(test.config))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Decode() error = %v, want %q", err, test.want)
			}
		})
	}
}
