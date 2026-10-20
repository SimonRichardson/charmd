package charmstate

import (
	"testing"

	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/config"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

func TestNewManagerRequiresCharmAPI(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewManager did not panic without a charm API client")
		}
	}()
	NewManager(state.New(nil), nil)
}

func TestNewManager(t *testing.T) {
	client, err := charmapi.New(config.Endpoint{URL: "https://agent.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if manager := NewManager(state.New(nil), client); manager == nil {
		t.Fatal("NewManager returned nil")
	}
}
