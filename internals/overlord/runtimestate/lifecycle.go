package runtimestate

import (
	"errors"
	"fmt"

	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

// lifecycle records startup independently of transient config notifications and
// task-runner status. Each successful installation owns its own lifecycle.
type lifecycle struct {
	Installed bool   `json:"installed"`
	Started   bool   `json:"started"`
	StartTask string `json:"start-task,omitempty"`
}

// installationID follows older dependency chains when an explicit identity is absent.
// The caller holds state; a hook may depend on a config fetch or another hook.
func installationID(task *state.Task) (string, error) {
	seen := make(map[string]bool)
	for task != nil && !seen[task.ID()] {
		seen[task.ID()] = true
		var id, name string
		if task.Get("hook-install-task", &id) == nil && id != "" {
			return id, nil
		}
		if task.Kind() == RunHookTask && task.Get("hook-name", &name) == nil && name == "install" {
			return task.ID(), nil
		}
		previous := task.WaitTasks()
		if len(previous) != 1 {
			break
		}
		task = previous[0]
	}
	return "", fmt.Errorf("hook has no installation identity")
}

// readLifecycle loads the installation's durable startup state under the state lock.
func (m *Manager) readLifecycle(id string) (lifecycle, error) {
	var current lifecycle
	err := m.state.Get("charm-lifecycle-"+id, &current)
	if errors.Is(err, state.ErrNoState) {
		err = nil
	}
	return current, err
}

// commitLifecycle schedules start atomically with the first successful config
// commit, and records startup only when start itself commits successfully.
// The caller holds state, so replay cannot create duplicate start tasks.
func (m *Manager) commitLifecycle(task *state.Task, name string) error {
	if name != "install" && name != "config-changed" && name != "start" {
		return nil
	}
	id, err := installationID(task)
	if err != nil {
		return err
	}
	current, err := m.readLifecycle(id)
	if err != nil {
		return err
	}
	switch name {
	case "install":
		current.Installed = true
	case "config-changed":
		install := m.state.Task(id)
		if install == nil || install.Status() != state.DoneStatus {
			return fmt.Errorf("configuration cannot start an incomplete installation")
		}
		current.Installed = true
		if !current.Started && current.StartTask == "" {
			start := HookTask(m.state, task, "start")
			start.Set("hook-install-task", id)
			task.Change().AddTask(start)
			current.StartTask = start.ID()
			m.state.EnsureBefore(0)
		}
	case "start":
		if !current.Installed || current.StartTask != task.ID() {
			return fmt.Errorf("start hook does not belong to the pending startup")
		}
		current.Started = true
	}
	task.Set("hook-install-task", id)
	m.state.Set("charm-lifecycle-"+id, current)
	return nil
}

// startPending gives the initial start priority over subsequent config hooks.
// Failed/interrupted starts retain this gate until resolved. The caller holds state.
func (m *Manager) startPending(task *state.Task) bool {
	var name string
	if task.Get("hook-name", &name) != nil || name != "config-changed" {
		return false
	}
	id, err := installationID(task)
	if err != nil {
		return false
	}
	current, err := m.readLifecycle(id)
	if err != nil || current.Started || current.StartTask == "" {
		return false
	}
	return true
}
