// Package legacy runs Juju-style charms on the local machine. A runner prepares
// each hook with its own context, hookd session, and process executor.
package legacy

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

// Runtime creates a fresh execution context and hookd session for every hook.
// Calls on a Runtime are serialized through completion of hookd cleanup.
// A Runtime must not be copied after first use.
// Executable must support hook-tool invocation through tool-name symlinks.
type Runtime struct {
	mu sync.Mutex

	Executable string
	Executor   Executor
}

// RunHook prepares an invocation context and delegates hook execution to its runner.
func (r *Runtime) RunHook(ctx context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
	switch hook.Name {
	case "install", "config-changed":
	default:
		return charmRuntime.Result{}, fmt.Errorf("unsupported legacy hook %q", hook.Name)
	}
	// Hold the execution lock across preparation, execution, and cleanup so the
	// next hook cannot observe a partially completed invocation.
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return charmRuntime.Result{}, err
	}

	dir, err := filepath.Abs(hook.CharmDir)
	if err != nil {
		return charmRuntime.Result{}, err
	}
	// Process output and hook-tool logging can share a destination.
	var outputMu sync.Mutex
	if hook.Stdout != nil {
		hook.Stdout = lockedWriter{&outputMu, hook.Stdout}
	}
	if hook.Stderr != nil {
		hook.Stderr = lockedWriter{&outputMu, hook.Stderr}
	}
	hookContext, err := NewHookContext(dir, hook.Stderr)
	if err != nil {
		return charmRuntime.Result{}, err
	}
	if err := hookContext.applyConfig(hook.Config); err != nil {
		return charmRuntime.Result{}, err
	}
	runner := NewRunner(hookContext, dir, r.Executable, r.Executor)
	return runner.RunHook(ctx, hook)
}
