// Package runtimestate connects charm lifecycle tasks to a runtime.
package runtimestate

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/tomb.v2"

	"github.com/SimonRichardson/charmd/internals/overlord/state"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

// ErrSkipHook indicates an obsolete lifecycle event that should complete without charm execution.
var ErrSkipHook = errors.New("hook no longer required")

// RunHookTask identifies lifecycle hook work on the shared runner.
const RunHookTask = "run-hook"

// Manager owns hook execution on the shared task runner.
type Manager struct {
	state   *state.State
	runtime charmRuntime.Runtime
	// Leadership resolves and reconciles leadership immediately before hook execution.
	Leadership func(context.Context, *state.Task) (bool, error)
}

// NewManager registers hook execution and serializes hooks through the shared task runner.
func NewManager(st *state.State, runner *state.TaskRunner, runtime charmRuntime.Runtime) *Manager {
	if st == nil || runner == nil || runtime == nil {
		panic("state, task runner and runtime are required")
	}
	m := &Manager{state: st, runtime: runtime}
	m.recoverLegacyTasks()
	runner.AddHandler(RunHookTask, m.runHook, nil)
	// The initial runtime models one local unit per charm installation. Serialize
	// hooks to avoid concurrent mutations of the same charm's working directory.
	runner.AddBlocked(m.blocked)
	return m
}

// Ensure leaves task scheduling to the shared task runner.
func (m *Manager) Ensure() error { return nil }

// InstallTask creates the install hook after unpacking. The caller holds the state lock.
func InstallTask(st *state.State, install *state.Task) *state.Task {
	return HookTask(st, install, "install")
}

// HookTask creates a hook that waits for its predecessor to complete successfully.
func HookTask(st *state.State, previous *state.Task, name string) *state.Task {
	task := st.NewTask(RunHookTask, fmt.Sprintf("Run charm %s hook", name))
	task.Set("hook-name", name)
	task.Set("hook-state", hookState{Step: hookQueued})
	// Retain installation identity through config fetches and subsequent hooks.
	var installID string
	if name == "install" {
		installID = task.ID()
	} else if previous.Get("hook-install-task", &installID) != nil {
		previous.Get("install-task", &installID)
	}
	if installID != "" {
		task.Set("hook-install-task", installID)
	}
	var checksum string
	if previous.Get("charm-sha256", &checksum) == nil {
		task.Set("charm-sha256", checksum)
	}
	task.WaitFor(previous)
	return task
}

// runHook resumes the durable operation phase before considering charm execution.
func (m *Manager) runHook(task *state.Task, tomb *tomb.Tomb) error {
	m.state.Lock()
	progress, err := readHookState(task)
	m.state.Unlock()
	if err != nil {
		return err
	}
	switch progress.Step {
	case hookPending:
		return fmt.Errorf("%s hook execution was interrupted; retry or resolution is required", progress.Invocation.Name)
	case hookDone, hookCommitted:
		return m.commitHook(task, progress)
	case hookQueued:
		return m.executeHook(task, tomb)
	default:
		return fmt.Errorf("unknown hook execution step %q", progress.Step)
	}
}

type taskOutput struct {
	state *state.State
	task  *state.Task
}

// Write records bounded output chunks under the state lock, allowing concurrent hook output.
func (w taskOutput) Write(data []byte) (int, error) {
	w.state.Lock()
	defer w.state.Unlock()
	// Bound individual log entries, including output without newlines.
	for start := 0; start < len(data); start += 4096 {
		end := min(start+4096, len(data))
		if line := strings.TrimSpace(string(data[start:end])); line != "" {
			w.task.Logf("%s", line)
		}
	}
	return len(data), nil
}
