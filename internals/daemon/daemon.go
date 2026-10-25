// Package daemon exposes the overlord through a Charmd-style HTTP API.
package daemon

import (
	"net/http"
	"sync/atomic"

	"github.com/SimonRichardson/charmd/internals/overlord"
)

// ResponseFunc handles one HTTP method for an API Command.
type ResponseFunc func(*Command, *http.Request) Response

// Command declares one API endpoint and its supported methods.
type Command struct {
	Path string
	GET  ResponseFunc
	POST ResponseFunc
	d    *Daemon
}

// ServeHTTP dispatches a request to the registered method handler.
func (c *Command) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var handler ResponseFunc
	switch r.Method {
	case http.MethodGet:
		handler = c.GET
	case http.MethodPost:
		handler = c.POST
	}
	if handler == nil {
		MethodNotAllowed("method %s is not allowed", r.Method).ServeHTTP(w, r)
		return
	}
	handler(c, r).ServeHTTP(w, r)
}

// Daemon owns the API router and the overlord it exposes.
type Daemon struct {
	overlord *overlord.Overlord
	router   *http.ServeMux
	ready    atomic.Bool
}

// New creates a daemon and installs the registered API commands.
func New(o *overlord.Overlord) *Daemon {
	d := &Daemon{overlord: o}
	d.addRoutes()
	return d
}

// Handler returns the daemon's API router.
func (d *Daemon) Handler() http.Handler {
	return d.router
}

// Start marks the daemon ready to accept and process requests.
func (d *Daemon) Start() {
	d.ready.Store(true)
}

// Stop marks the daemon unavailable before its HTTP server is shut down.
func (d *Daemon) Stop() {
	d.ready.Store(false)
}

// addRoutes installs health checks and the registered API endpoints.
func (d *Daemon) addRoutes() {
	d.router = http.NewServeMux()
	d.router.HandleFunc("GET /health", d.health)
	d.router.HandleFunc("GET /ready", d.readiness)
	for _, definition := range API {
		command := *definition
		command.d = d
		d.router.Handle(command.Path, &command)
	}
	d.router.Handle("/", NotFound("invalid API endpoint requested"))
}

// health reports that the daemon HTTP server is responding.
func (d *Daemon) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// readiness reports whether the daemon is accepting work.
func (d *Daemon) readiness(w http.ResponseWriter, _ *http.Request) {
	if !d.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}
