package configapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/SimonRichardson/charmd/internals/overlord/runtimestate"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
	"gopkg.in/tomb.v2"
)

// pendingEventsKey identifies notifications not yet converted into changes.
const pendingEventsKey = "config-api-pending-events"

// pendingEvents reads the durable notification counts while the caller holds state.
func pendingEvents(st *state.State) (map[string]int, error) {
	events := make(map[string]int)
	if err := st.Get(pendingEventsKey, &events); err != nil && !errors.Is(err, state.ErrNoState) {
		return nil, err
	}
	return events, nil
}

// Ensure turns every service notification into a separate config change once
// installation succeeds. It never compares values or synthesizes notifications.
func (m *Manager) Ensure() error {
	m.state.Lock()
	defer m.state.Unlock()
	events, err := pendingEvents(m.state)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	// Reinstalling the same checksum may leave older install tasks in history.
	// Deliver notifications to the most recent successful installation only.
	installs := make(map[string]*state.Task)
	for _, install := range m.state.Tasks() {
		if install.Kind() != runtimestate.RunHookTask || install.Status() != state.DoneStatus {
			continue
		}
		var name, checksum string
		if err := install.Get("hook-name", &name); err != nil || name != "install" {
			continue
		}
		if err := install.Get("charm-sha256", &checksum); err != nil {
			continue
		}
		if old := installs[checksum]; old == nil || install.SpawnTime().After(old.SpawnTime()) {
			installs[checksum] = install
		}
	}
	for checksum, count := range events {
		install := installs[checksum]
		if install == nil {
			continue
		}
		for range count {
			change := m.state.NewChange("change-config", fmt.Sprintf("Apply config for charm %q", checksum))
			fetch := m.state.NewTask(FetchConfigTask, "Fetch charm configuration")
			fetch.Set("charm-sha256", checksum)
			fetch.Set("install-task", install.ID())
			fetch.WaitFor(install)
			hook := runtimestate.HookTask(m.state, fetch, "config-changed")
			change.AddTask(fetch)
			change.AddTask(hook)
		}
		delete(events, checksum)
		m.state.EnsureBefore(0)
	}
	m.state.Set(pendingEventsKey, events)
	return nil
}

// fetchConfig performs network I/O without the state lock, then records a stable
// snapshot for hook execution. The service decides whether config has changed.
func (m *Manager) fetchConfig(task *state.Task, tomb *tomb.Tomb) error {
	m.state.Lock()
	var checksum, installID string
	err := task.Get("charm-sha256", &checksum)
	if err == nil {
		err = task.Get("install-task", &installID)
	}
	var dir string
	if err == nil {
		install := m.state.Task(installID)
		if install == nil {
			err = fmt.Errorf("config installation task %q is missing", installID)
		} else {
			err = install.Get("charm-install-path", &dir)
		}
	}
	m.state.Unlock()
	if err != nil {
		return err
	}
	values, err := m.client.Get(tomb.Context(context.Background()), checksum)
	if err != nil {
		return err
	}
	m.state.Lock()
	defer m.state.Unlock()
	task.Set("charm-install-path", dir)
	task.Set("hook-config", values)
	return nil
}
