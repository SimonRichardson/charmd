// Package runtime defines the boundary between charm lifecycle tasks and their
// execution environments. Implementations do not depend on daemon state.
package runtime

import (
	"context"
	"io"
)

// Hook describes one lifecycle invocation in an already unpacked charm.
type Hook struct {
	CharmDir string
	Name     string
	// Config contains the service-provided overrides for this invocation.
	Config map[string]any
	Stdout io.Writer
	Stderr io.Writer
}

// Status is the workload status last set by a hook.
type Status struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Result contains changes made through hook tools during an invocation.
type Result struct {
	Status *Status `json:"status,omitempty"`
}

// Runtime executes hooks and releases invocation resources before returning.
type Runtime interface {
	// RunHook executes a lifecycle event and returns the changes made by its hook tools.
	RunHook(context.Context, Hook) (Result, error)
}
