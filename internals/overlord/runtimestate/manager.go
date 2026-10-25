// Package runtimestate connects charm lifecycle tasks to a runtime.
package runtimestate

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/tomb.v2"

	"github.com/SimonRichardson/charmd/internals/overlord/state"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

const RunHookTask = "run-hook"

// Manager owns hook execution on the shared task runner.
type Manager struct {
	state   *state.State
	runtime charmRuntime.Runtime
}

// NewManager registers hook execution and serializes hooks through the shared task runner.
func NewManager(st *state.State, runner *state.TaskRunner, runtime charmRuntime.Runtime) *Manager {
	if st == nil || runner == nil || runtime == nil {
		panic("state, task runner and runtime are required")
	}
	m := &Manager{state: st, runtime: runtime}
	runner.AddHandler(RunHookTask, m.runHook, nil)
	// The initial runtime models one local unit per charm installation. Serialize
	// hooks to avoid concurrent mutations of the same charm's working directory.
	runner.AddBlocked(func(task *state.Task, running []*state.Task) bool {
		if task.Kind() != RunHookTask {
			return false
		}
		for _, other := range running {
			if other.Kind() == RunHookTask {
				return true
			}
		}
		return false
	})
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
	task.WaitFor(previous)
	return task
}

// runHook reads the completed installation, executes its hook, and persists the result without holding state during execution.
func (m *Manager) runHook(task *state.Task, tomb *tomb.Tomb) error {
	m.state.Lock()
	dependencies := task.WaitTasks()
	var dir, name string
	var config map[string]any
	var err error
	if len(dependencies) != 1 {
		err = fmt.Errorf("hook task requires one predecessor task")
	} else {
		err = dependencies[0].Get("charm-install-path", &dir)
		if err == nil && dependencies[0].Has("hook-config") {
			err = dependencies[0].Get("hook-config", &config)
		}
	}
	if err == nil {
		err = task.Get("hook-name", &name)
	}
	m.state.Unlock()

	if err != nil {
		return err
	}
	if dir == "" {
		return fmt.Errorf("hook task has no charm installation path")
	}
	// Hooks and tool requests may perform I/O for an arbitrary duration.
	// Keep the shared state unlocked until there is a result to commit.
	output := taskOutput{state: m.state, task: task}
	result, err := m.runtime.RunHook(tomb.Context(context.Background()), charmRuntime.Hook{
		CharmDir: dir, Name: name, Config: config, Stdout: output, Stderr: output,
	})
	m.state.Lock()
	task.Set("hook-result", result)
	// Forward the installation path along the lifecycle dependency chain.
	if err == nil {
		task.Set("charm-install-path", dir)
		task.Set("hook-config", config)
	}
	m.state.Unlock()
	return err
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
