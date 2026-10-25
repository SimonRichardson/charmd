package legacy

import (
	"context"
	"fmt"

	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

// Runner coordinates hook discovery, tool lifetime and execution against one
// hook context. Context, transport, and executor own their respective details.
type Runner struct {
	context    Context
	executor   Executor
	executable string
	charmDir   string
}

// NewRunner binds an invocation context and paths to an executor.
// A nil executor selects local process execution.
func NewRunner(hookContext Context, charmDir, executable string, executor Executor) *Runner {
	if executor == nil {
		executor = processExecutor{}
	}
	return &Runner{context: hookContext, charmDir: charmDir, executable: executable, executor: executor}
}

// RunHook discovers the handler, owns its hookd session, and collects the final context result.
func (r *Runner) RunHook(ctx context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
	script, err := discoverHook(r.charmDir, hook.Name)
	if err != nil || script == "" {
		return r.context.Result(), err
	}
	session, err := newSession(r.executable, r.charmDir, r.context)
	if err != nil {
		return r.context.Result(), err
	}
	err = r.executor.Execute(ctx, Command{
		Path: script, Dir: r.charmDir, Env: r.context.Environment(session.paths, hook.Name),
		Stdout: hook.Stdout, Stderr: hook.Stderr,
	})
	// Finish tool requests before reading the result so no late command can
	// mutate the invocation after its result has been collected.
	session.Close()
	if err != nil {
		return r.context.Result(), fmt.Errorf("run %s hook: %w", hook.Name, err)
	}
	return r.context.Result(), nil
}
