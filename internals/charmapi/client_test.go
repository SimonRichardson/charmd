package charmapi

import (
	"testing"

	"github.com/SimonRichardson/charmd/internals/config"
)

func TestNew(t *testing.T) {
	client, err := New(config.Endpoint{URL: "https://agent.example.test/charm"})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := client.Endpoint()
	if got := endpoint.String(); got != "https://agent.example.test/charm" {
		t.Errorf("Endpoint() = %q", got)
	}
}

func TestNewRejectsInvalidEndpoint(t *testing.T) {
	_, err := New(config.Endpoint{URL: "ftp://agent.example.test"})
	if err == nil {
		t.Fatal("New() succeeded with an invalid endpoint")
	}
}
