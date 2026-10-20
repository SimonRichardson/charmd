package charmstate

import (
	"reflect"
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
	NewManager(state.New(nil), state.NewTaskRunner(state.New(nil)), nil)
}

func TestNewManager(t *testing.T) {
	client, err := charmapi.New(config.Endpoint{URL: "https://agent.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	s := state.New(nil)
	r := state.NewTaskRunner(s)
	if manager := NewManager(s, r, client); manager == nil {
		t.Fatal("NewManager returned nil")
	}
	if got, want := r.KnownTaskKinds(), []string{FetchCharmTask, InstallCharmTask}; !reflect.DeepEqual(got, want) && !reflect.DeepEqual(got, []string{InstallCharmTask, FetchCharmTask}) {
		t.Fatalf("KnownTaskKinds() = %v, want %v", got, want)
	}
}

func TestInstallCreatesDependentTasks(t *testing.T) {
	s := state.New(nil)
	r := state.NewTaskRunner(s)
	manager := NewManager(s, r, newCharmAPI(t))

	change, err := manager.Install("postgresql-k8s")
	if err != nil {
		t.Fatal(err)
	}

	s.Lock()
	defer s.Unlock()
	if got, want := change.Kind(), "install-charm"; got != want {
		t.Fatalf("change kind = %q, want %q", got, want)
	}
	tasks := change.Tasks()
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks))
	}
	if got, want := tasks[0].Kind(), FetchCharmTask; got != want {
		t.Fatalf("first task kind = %q, want %q", got, want)
	}
	if got, want := tasks[1].Kind(), InstallCharmTask; got != want {
		t.Fatalf("second task kind = %q, want %q", got, want)
	}
	if got := tasks[1].WaitTasks(); !reflect.DeepEqual(got, []*state.Task{tasks[0]}) {
		t.Fatalf("install dependencies = %v, want fetch task", got)
	}
	if err := change.CheckTaskDependencies(); err != nil {
		t.Fatalf("task dependencies are invalid: %v", err)
	}
	for _, task := range tasks {
		var charm string
		if err := task.Get(charmNameKey, &charm); err != nil {
			t.Fatal(err)
		}
		if charm != "postgresql-k8s" {
			t.Fatalf("task charm = %q, want postgresql-k8s", charm)
		}
	}
}

func TestInstallRequiresCharmName(t *testing.T) {
	s := state.New(nil)
	manager := NewManager(s, state.NewTaskRunner(s), newCharmAPI(t))
	for _, charm := range []string{"", " \t", "\n"} {
		if _, err := manager.Install(charm); err == nil {
			t.Errorf("Install(%q) succeeded", charm)
		}
	}

	s.Lock()
	defer s.Unlock()
	if got := len(s.Changes()); got != 0 {
		t.Fatalf("change count = %d, want 0", got)
	}
	if got := s.TaskCount(); got != 0 {
		t.Fatalf("task count = %d, want 0", got)
	}
}

func TestInstallTasksExecuteInOrder(t *testing.T) {
	s := state.New(nil)
	r := state.NewTaskRunner(s)
	defer r.Stop()
	manager := NewManager(s, r, newCharmAPI(t))
	change, err := manager.Install("postgresql-k8s")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Ensure(); err != nil {
		t.Fatal(err)
	}
	r.Wait()

	s.Lock()
	tasks := change.Tasks()
	if got, want := tasks[0].Status(), state.DoneStatus; got != want {
		s.Unlock()
		t.Fatalf("fetch status after first ensure = %s, want %s", got, want)
	}
	if got, want := tasks[1].Status(), state.DoStatus; got != want {
		s.Unlock()
		t.Fatalf("install status after first ensure = %s, want %s", got, want)
	}
	s.Unlock()

	if err := r.Ensure(); err != nil {
		t.Fatal(err)
	}
	r.Wait()

	s.Lock()
	defer s.Unlock()
	if !change.Status().Ready() {
		t.Fatal("change did not complete")
	}
	if err := change.Err(); err != nil {
		t.Fatal(err)
	}
}

func newCharmAPI(t *testing.T) *charmapi.Client {
	t.Helper()
	client, err := charmapi.New(config.Endpoint{URL: "https://agent.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
