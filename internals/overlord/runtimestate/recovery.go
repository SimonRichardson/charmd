package runtimestate

import "github.com/SimonRichardson/charmd/internals/overlord/state"

// recoverLegacyTasks conservatively fences old in-flight tasks without a phase.
// A historical hook-result alone does not prove success, so it cannot authorize replay.
func (m *Manager) recoverLegacyTasks() {
	m.state.Lock()
	defer m.state.Unlock()
	for _, task := range m.state.Tasks() {
		if task.Kind() != RunHookTask || task.Status() != state.DoingStatus || task.Has("hook-state") {
			continue
		}
		var name string
		task.Get("hook-name", &name)
		task.Set("hook-state", hookState{Step: hookPending, Invocation: invocation{Name: name}})
	}
}

// blocked excludes concurrent hooks globally and gates a charm after a hook error.
// Commit-only recovery is allowed through error gates because it cannot execute code.
func (m *Manager) blocked(task *state.Task, running []*state.Task) bool {
	if task.Kind() != RunHookTask {
		return false
	}
	for _, other := range running {
		if other.Kind() == RunHookTask {
			return true
		}
	}
	if HookExecuted(task) {
		return false
	}
	if m.startPending(task) {
		return true
	}
	var checksum string
	if task.Get("charm-sha256", &checksum) != nil || checksum == "" {
		return false
	}
	for _, other := range m.state.Tasks() {
		var sha, name string
		interrupted := other.Status() == state.DoingStatus && HookStarted(other) && !HookExecuted(other)
		if other.ID() != task.ID() && other.Kind() == RunHookTask && (other.Status() == state.ErrorStatus || interrupted) && other.Get("charm-sha256", &sha) == nil && sha == checksum {
			// Election failures are fenced by the leadership manager's epoch;
			// losing leadership must release an obsolete election's gate.
			if other.Get("hook-name", &name) == nil && name == "leader-elected" {
				continue
			}
			return true
		}
	}
	return false
}
