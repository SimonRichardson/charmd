package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
