package runtimestate

import (
	"context"
	"errors"
	"fmt"

	"gopkg.in/tomb.v2"

	"github.com/SimonRichardson/charmd/internals/overlord/state"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

// hookStep describes durable progress independently of the task runner's status.
type hookStep string

const (
	hookQueued    hookStep = "queued"
	hookPending   hookStep = "pending"
	hookDone      hookStep = "done"
	hookCommitted hookStep = "committed"
)

// invocation retains the inputs needed to finish a hook without querying live APIs.
type invocation struct {
	Dir    string         `json:"dir"`
	Name   string         `json:"name"`
	Config map[string]any `json:"config"`
	Leader bool           `json:"leader"`
}

// hookState is checkpointed before execution, after success, and after commit.
// Done means the charm has exited successfully, not that the task is complete.
type hookState struct {
	Step       hookStep            `json:"step"`
	Invocation invocation          `json:"invocation"`
	Result     charmRuntime.Result `json:"result"`
}

// readHookState reads progress under the state lock; older unstarted tasks are queued.
func readHookState(task *state.Task) (hookState, error) {
	progress := hookState{Step: hookQueued}
	if task.Has("hook-state") {
		progress = hookState{}
		if err := task.Get("hook-state", &progress); err != nil {
			return hookState{}, fmt.Errorf("read hook execution state: %w", err)
		}
	}
	return progress, nil
}

// HookExecuted reports whether only commit/bookkeeping remains. The caller holds state.
// Scheduling must allow this work through leadership gates to avoid blocking recovery.
func HookExecuted(task *state.Task) bool {
	progress, err := readHookState(task)
	return err == nil && (progress.Step == hookDone || progress.Step == hookCommitted)
}

// HookStarted distinguishes executing hooks from tasks deferred during preparation.
// The caller holds state. Old tasks without progress retain their historical Doing meaning.
func HookStarted(task *state.Task) bool {
	if !task.Has("hook-state") {
		return task.Status() == state.DoingStatus
	}
	progress, err := readHookState(task)
	return err == nil && (progress.Step == hookPending || progress.Step == hookDone || progress.Step == hookCommitted)
}

// prepareHook reads immutable inputs without holding state across leadership API I/O.
func (m *Manager) prepareHook(ctx context.Context, task *state.Task) (invocation, error) {
	m.state.Lock()
	input, err := hookInputs(task)
	m.state.Unlock()
	if err == nil && m.Leadership != nil {
		input.Leader, err = m.Leadership(ctx, task)
	}
	return input, err
}

// hookInputs validates the predecessor and copies execution inputs under the state lock.
func hookInputs(task *state.Task) (invocation, error) {
	var input invocation
	dependencies := task.WaitTasks()
	if len(dependencies) != 1 {
		return input, fmt.Errorf("hook task requires one predecessor task")
	}
	previous := dependencies[0]
	if err := previous.Get("charm-install-path", &input.Dir); err != nil {
		return input, err
	}
	if input.Dir == "" {
		return input, fmt.Errorf("hook task has no charm installation path")
	}
	if previous.Has("hook-config") {
		if err := previous.Get("hook-config", &input.Config); err != nil {
			return input, err
		}
	}
	if err := task.Get("hook-name", &input.Name); err != nil {
		return input, err
	}
	if input.Name == "" {
		return input, fmt.Errorf("hook task has no hook name")
	}
	return input, nil
}

// executeHook persists intent before entering the runtime and success before committing.
func (m *Manager) executeHook(task *state.Task, tomb *tomb.Tomb) error {
	ctx := tomb.Context(context.Background())
	input, err := m.prepareHook(ctx, task)
	skip := errors.Is(err, ErrSkipHook)
	if err != nil && !skip {
		return err
	}
	if ctx.Err() != nil {
		return &state.Retry{Reason: "hook preparation was canceled"}
	}
	progress := hookState{Step: hookPending, Invocation: input}
	if !skip {
		m.saveHookState(task, progress)
		// A durable Pending checkpoint must exist before the charm can change the
		// system. Runtime I/O and hookd calls run without the shared state lock.
		output := taskOutput{state: m.state, task: task}
		progress.Result, err = m.runtime.RunHook(ctx, charmRuntime.Hook{
			CharmDir: input.Dir, Name: input.Name, Config: input.Config, Leader: input.Leader,
			Stdout: output, Stderr: output,
		})
		if err != nil {
			// Only recognized cancellation is safe to requeue automatically. An
			// abrupt death leaves Pending and requires hook-error recovery.
			if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
				progress.Step = hookQueued
				m.saveHookState(task, progress)
				return &state.Retry{Reason: "hook execution was canceled"}
			}
			m.saveHookState(task, progress)
			return err
		}
	}
	progress.Step = hookDone
	m.saveHookState(task, progress)
	return m.commitHook(task, progress)
}

// saveHookState checkpoints the phase and tool results together before returning.
func (m *Manager) saveHookState(task *state.Task, progress hookState) {
	m.state.Lock()
	defer m.state.Unlock()
	task.Set("hook-state", progress)
	task.Set("hook-result", progress.Result)
}

// commitHook idempotently forwards the saved invocation, then lets the runner finish
// the task and invoke lifecycle observers. It never contacts APIs or runs charm code.
func (m *Manager) commitHook(task *state.Task, progress hookState) error {
	if progress.Invocation.Dir == "" || progress.Invocation.Name == "" {
		return fmt.Errorf("completed hook has no saved invocation")
	}
	m.state.Lock()
	defer m.state.Unlock()
	if err := m.commitLifecycle(task, progress.Invocation.Name); err != nil {
		return err
	}
	task.Set("charm-install-path", progress.Invocation.Dir)
	task.Set("hook-config", progress.Invocation.Config)
	task.Set("hook-result", progress.Result)
	progress.Step = hookCommitted
	task.Set("hook-state", progress)
	return nil
}
