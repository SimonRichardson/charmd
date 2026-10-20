package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/config"
	"github.com/SimonRichardson/charmd/internals/overlord"
	"github.com/SimonRichardson/charmd/internals/overlord/charmstate"
)

func TestHealth(t *testing.T) {
	d := New(nil)
	recorder := httptest.NewRecorder()
	d.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	if recorder.Code != http.StatusOK {
		t.Errorf("health status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("health body = %q, want empty", recorder.Body.String())
	}
}

func TestReadinessLifecycle(t *testing.T) {
	d := New(nil)

	assertReadinessStatus(t, d, http.StatusServiceUnavailable)
	d.Start()
	assertReadinessStatus(t, d, http.StatusOK)
	d.Stop()
	assertReadinessStatus(t, d, http.StatusServiceUnavailable)
}

func TestPostCharmsCreatesInstallChange(t *testing.T) {
	o := newOverlord(t)
	d := New(o)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/charms", bytes.NewBufferString(`{"action":"install","charm":"postgresql-k8s"}`))
	d.Handler().ServeHTTP(recorder, req)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	var response struct {
		Type   string `json:"type"`
		Change string `json:"change"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Type != "async" {
		t.Fatalf("response type = %q, want async", response.Type)
	}
	if response.Change == "" {
		t.Fatal("response did not include a change ID")
	}

	state := o.State()
	state.Lock()
	defer state.Unlock()
	change := state.Change(response.Change)
	if change == nil {
		t.Fatalf("change %q not found", response.Change)
	}
	if got, want := change.Kind(), "install-charm"; got != want {
		t.Fatalf("change kind = %q, want %q", got, want)
	}
	tasks := change.Tasks()
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks))
	}
	if got, want := tasks[0].Kind(), charmstate.FetchCharmTask; got != want {
		t.Fatalf("first task kind = %q, want %q", got, want)
	}
	if got, want := tasks[1].Kind(), charmstate.InstallCharmTask; got != want {
		t.Fatalf("second task kind = %q, want %q", got, want)
	}
	if got := tasks[1].WaitTasks(); len(got) != 1 || got[0] != tasks[0] {
		t.Fatal("install task does not wait for fetch task")
	}
}

func TestPostCharmsRejectsInvalidRequest(t *testing.T) {
	o := newOverlord(t)
	d := New(o)
	for _, body := range []string{
		`{"action":`,
		`{"action":"remove","charm":"postgresql-k8s"}`,
		`{"action":"install","charm":""}`,
	} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/charms", bytes.NewBufferString(body))
		d.Handler().ServeHTTP(recorder, req)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d for %s", recorder.Code, http.StatusBadRequest, body)
		}
		var response struct {
			StatusCode int    `json:"status-code"`
			Type       string `json:"type"`
		}
		if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
			t.Errorf("decode response for %s: %v", body, err)
			continue
		}
		if response.StatusCode != http.StatusBadRequest || response.Type != "error" {
			t.Errorf("response = %#v, want 400 error for %s", response, body)
		}
	}

	state := o.State()
	state.Lock()
	defer state.Unlock()
	if got := len(state.Changes()); got != 0 {
		t.Fatalf("rejected requests created %d changes", got)
	}
	if got := state.TaskCount(); got != 0 {
		t.Fatalf("rejected requests created %d tasks", got)
	}
}

func newOverlord(t *testing.T) *overlord.Overlord {
	t.Helper()
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
	t.Cleanup(o.Stop)
	return o
}

func assertReadinessStatus(t *testing.T, d *Daemon, want int) {
	t.Helper()
	recorder := httptest.NewRecorder()
	d.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if recorder.Code != want {
		t.Errorf("readiness status = %d, want %d", recorder.Code, want)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("readiness body = %q, want empty", recorder.Body.String())
	}
}
