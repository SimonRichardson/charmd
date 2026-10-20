// Package daemon exposes the overlord through a Charmd-style HTTP API.
package daemon

import (
	"net/http"

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

func (d *Daemon) addRoutes() {
	d.router = http.NewServeMux()
	for _, definition := range API {
		command := *definition
		command.d = d
		d.router.Handle(command.Path, &command)
	}
	d.router.Handle("/", NotFound("invalid API endpoint requested"))
}
