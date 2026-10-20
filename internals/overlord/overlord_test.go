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

func TestCharmInstallChangeSurvivesRestart(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	charmAPI, err := charmapi.New(config.Endpoint{URL: "https://agent.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	opts := &overlord.Options{StatePath: statePath, CharmAPI: charmAPI}
	o, err := overlord.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	change, err := o.CharmManager().Install("postgresql-k8s")
	if err != nil {
		t.Fatal(err)
	}
	changeID := change.ID()
	o.Stop()

	restored, err := overlord.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Stop()

	state := restored.State()
	state.Lock()
	defer state.Unlock()

	change = state.Change(changeID)
	if change == nil {
		t.Fatalf("change %q was not restored", changeID)
	}
	tasks := change.Tasks()
	if len(tasks) != 2 {
		t.Fatalf("restored task count = %d, want 2", len(tasks))
	}
	if got := tasks[1].WaitTasks(); len(got) != 1 || got[0] != tasks[0] {
		t.Fatal("restored install task does not wait for fetch task")
	}
	for _, task := range tasks {
		var charm string
		if err := task.Get("charm-name", &charm); err != nil {
			t.Fatal(err)
		}
		if charm != "postgresql-k8s" {
			t.Fatalf("restored charm = %q, want postgresql-k8s", charm)
		}
	}
}
