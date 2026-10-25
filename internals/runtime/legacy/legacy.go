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
// Executable must support hook-tool invocation through tool-name symlinks.
type Runtime struct {
	Executable string
	Executor   Executor
}

// RunHook prepares an invocation context and delegates hook execution to its runner.
func (r *Runtime) RunHook(ctx context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
	if hook.Name != "install" {
		return charmRuntime.Result{}, fmt.Errorf("unsupported legacy hook %q", hook.Name)
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
	runner := NewRunner(hookContext, dir, r.Executable, r.Executor)
	return runner.RunHook(ctx, hook)
}
