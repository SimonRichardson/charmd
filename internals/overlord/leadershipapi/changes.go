package leadershipapi

import (
	"fmt"
	"github.com/SimonRichardson/charmd/internals/overlord/runtimestate"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

// completed commits leadership acceptance only after a successful election hook.
// State holds its lock while invoking observers.
func (m *Manager) completed(task *state.Task, _, status state.Status) {
	if status != state.DoneStatus || task.Kind() != runtimestate.RunHookTask {
		return
	}
	var name, checksum string
	if task.Get("hook-name", &name) != nil {
		return
	}
	if name == "leader-elected" && task.Get("charm-sha256", &checksum) == nil {
		var leader bool
		var epoch int
		task.Get("leadership-epoch", &epoch)
		if task.Get("hook-leader", &leader) == nil && epoch == m.epoch(checksum) {
			m.state.Set("leadership-accepted-"+checksum, leader)
		}
	}
	m.state.EnsureBefore(0)
}

// blocked prioritizes outstanding leadership work, while never blocking installation.
// Failed election changes deliberately block ordinary hooks until resolved.
func (m *Manager) blocked(task *state.Task, _ []*state.Task) bool {
	if task.Kind() != runtimestate.RunHookTask || runtimestate.HookExecuted(task) {
		return false
	}
	var name, checksum string
	task.Get("hook-name", &name)
	task.Get("charm-sha256", &checksum)
	if name == "install" || name == "leader-elected" || checksum == "" {
		return false
	}
	var leader bool
	m.state.Get("leadership-current-"+checksum, &leader)
	return leader && m.pending(checksum) != nil
}

// pending finds election work that has not completed successfully. The caller holds state.
func (m *Manager) pending(checksum string) *state.Task {
	for _, task := range m.state.Tasks() {
		var name, sha string
		if task.Kind() == runtimestate.RunHookTask && task.Get("hook-name", &name) == nil && name == "leader-elected" && task.Get("charm-sha256", &sha) == nil && sha == checksum && task.Status() != state.DoneStatus {
			var epoch int
			task.Get("leadership-epoch", &epoch)
			if epoch == m.epoch(checksum) {
				return task
			}
		}
	}
	return nil
}

// Ensure schedules elections for known installed charms; pre-install events remain in state.
func (m *Manager) Ensure() error {
	if err := m.refresh(); err != nil {
		return err
	}
	m.state.Lock()
	defer m.state.Unlock()
	installs := make(map[string]*state.Task)
	for _, task := range m.state.Tasks() {
		var name, checksum string
		if task.Kind() != runtimestate.RunHookTask || task.Status() != state.DoneStatus || task.Get("hook-name", &name) != nil || name != "install" || task.Get("charm-sha256", &checksum) != nil {
			continue
		}
		if old := installs[checksum]; old == nil || task.SpawnTime().After(old.SpawnTime()) {
			installs[checksum] = task
		}
	}
	for checksum, install := range installs {
		m.reconcile(checksum, install)
	}
	return nil
}

// reconcile creates at most one outstanding election. The caller holds state.
func (m *Manager) reconcile(checksum string, install *state.Task) {
	var leader, accepted bool
	m.state.Get("leadership-current-"+checksum, &leader)
	m.state.Get("leadership-accepted-"+checksum, &accepted)
	if !leader {
		m.state.Set("leadership-accepted-"+checksum, false)
		return
	}
	if accepted || m.pending(checksum) != nil {
		return
	}
	change := m.state.NewChange("change-leadership", fmt.Sprintf("Accept leadership for charm %q", checksum))
	hook := runtimestate.HookTask(m.state, m.predecessor(checksum, install), "leader-elected")
	hook.Set("leadership-epoch", m.epoch(checksum))
	change.AddTask(hook)
	m.state.EnsureBefore(0)
}

// predecessor carries the latest successful configuration into election hooks.
// A currently running ordinary hook takes precedence and must finish first, so
// its result becomes the election's immutable input. The caller holds state.
func (m *Manager) predecessor(checksum string, install *state.Task) *state.Task {
	previous := install
	for _, task := range m.state.Tasks() {
		var sha, name string
		if task.Kind() != runtimestate.RunHookTask || task.Get("charm-sha256", &sha) != nil || sha != checksum || task.Get("hook-name", &name) != nil || name != "config-changed" {
			continue
		}
		if task.Status() == state.DoingStatus && runtimestate.HookStarted(task) {
			return task
		}
		if task.Status() == state.DoneStatus && task.ReadyTime().After(previous.ReadyTime()) {
			previous = task
		}
	}
	return previous
}
