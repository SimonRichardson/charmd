package overlord_test

import (
	"path/filepath"
	"testing"

	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/config"
	"github.com/SimonRichardson/charmd/internals/overlord"
)

func TestNewConstructsCharmManager(t *testing.T) {
	charmAPI, err := charmapi.New(config.Endpoint{URL: "https://agent.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	o, err := overlord.New(&overlord.Options{
		StatePath: filepath.Join(t.TempDir(), "state.json"),
		CharmAPI:  charmAPI,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer o.Stop()
	if o.CharmManager() == nil {
		t.Fatal("CharmManager() returned nil")
	}
}

func TestNewRequiresOptions(t *testing.T) {
	if _, err := overlord.New(nil); err == nil {
		t.Fatal("New(nil) succeeded")
	}
}
