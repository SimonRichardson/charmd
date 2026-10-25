package leadershipapi

import (
	"context"
	"github.com/SimonRichardson/charmd/internals/overlord/runtimestate"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

// HookLeadership fetches a snapshot immediately before execution. A newly observed
// election defers ordinary hooks until leader-elected succeeds. Once returned, the
// snapshot is owned by the invocation and cannot change during that hook.
func (m *Manager) HookLeadership(ctx context.Context, task *state.Task) (bool, error) {
	m.state.Lock()
	var name, checksum string
	err := task.Get("hook-name", &name)
	if err == nil {
		err = task.Get("charm-sha256", &checksum)
	}
	m.state.Unlock()
	if err != nil {
		return false, err
	}
	m.snapshotMu.Lock()
	defer m.snapshotMu.Unlock()
	leader, err := m.client.Get(ctx, checksum)
	if err != nil {
		return false, err
	}
	m.state.Lock()
	defer m.state.Unlock()
	m.setCurrent(checksum, leader)
	task.Set("hook-leader", leader)
	var epoch int
	task.Get("leadership-epoch", &epoch)
	if name == "leader-elected" && (!leader || epoch != m.epoch(checksum)) {
		return false, runtimestate.ErrSkipHook
	}
	if name == "install" || name == "leader-elected" {
		return leader, nil
	}
	var install *state.Task
	for _, candidate := range m.state.Tasks() {
		var sha, hook string
		if candidate.Status() == state.DoneStatus && candidate.Get("charm-sha256", &sha) == nil && sha == checksum && candidate.Get("hook-name", &hook) == nil && hook == "install" && (install == nil || candidate.SpawnTime().After(install.SpawnTime())) {
			install = candidate
		}
	}
	if install != nil {
		m.reconcile(checksum, install)
	}
	if leader && m.pending(checksum) != nil {
		return false, &state.Retry{Reason: "leadership hook must complete first"}
	}
	return leader, nil
}
