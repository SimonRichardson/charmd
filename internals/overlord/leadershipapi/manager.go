// Package leadershipapi reconciles leadership notifications between serialized hooks.
package leadershipapi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	service "github.com/SimonRichardson/charmd/internals/leadershipapi"
	"github.com/SimonRichardson/charmd/internals/logger"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

// Manager watches leadership and gives elections priority over ordinary hooks.
// The service reports the elected unit; charmd does not acquire or renew leases.
type Manager struct {
	state  *state.State
	client service.Client
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	// snapshotMu orders service reads and their state commits across hooks and refreshes.
	snapshotMu       sync.Mutex
	done             chan struct{}
	started, stopped bool
	observer         int
}

// NewManager registers election wakeups and priority on the shared task runner.
func NewManager(st *state.State, runner *state.TaskRunner, client service.Client) *Manager {
	if st == nil || runner == nil || client == nil {
		panic("state, runner and leadership client are required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{state: st, client: client, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	st.Lock()
	m.observer = st.AddTaskStatusChangedHandler(m.completed)
	st.Unlock()
	runner.AddBlocked(m.blocked)
	return m
}

// StartUp starts a single reconnecting subscription.
func (m *Manager) StartUp() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return fmt.Errorf("leadership manager is stopped")
	}
	if !m.started {
		m.started = true
		go m.watch()
	}
	return nil
}

// Stop cancels the subscription and removes the state observer, including before startup.
func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.stopped {
		m.stopped = true
		m.cancel()
		m.state.Lock()
		m.state.RemoveTaskStatusChangedHandler(m.observer)
		m.state.Unlock()
		if !m.started {
			close(m.done)
		}
	}
	m.mu.Unlock()
	<-m.done
}

// watch reconnects after interruption; notifications always fetch the current election.
func (m *Manager) watch() {
	defer close(m.done)
	for m.ctx.Err() == nil {
		err := m.client.Watch(m.ctx, m.changed)
		if err != nil && m.ctx.Err() == nil {
			logger.Noticef("leadership API watch: %v", err)
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-m.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// changed durably records notifications before waking reconciliation. Failed GETs
// retain this work so a service outage cannot silently lose a leadership change.
func (m *Manager) changed(checksum string) {
	m.state.Lock()
	defer m.state.Unlock()
	pending, err := m.notifications()
	if err != nil {
		logger.Noticef("leadership pending notifications: %v", err)
		return
	}
	pending[checksum]++
	m.state.Set("leadership-pending", pending)
	m.state.EnsureBefore(0)
}

// notifications reads pending generations while the caller holds state.
func (m *Manager) notifications() (map[string]int, error) {
	pending := make(map[string]int)
	err := m.state.Get("leadership-pending", &pending)
	if err != nil && !errors.Is(err, state.ErrNoState) {
		return nil, err
	}
	return pending, nil
}

// refresh fetches pending snapshots without holding state. Generation checks keep
// notifications received during I/O pending for another reconciliation pass.
func (m *Manager) refresh() error {
	m.state.Lock()
	pending, err := m.notifications()
	m.state.Unlock()
	if err != nil {
		return err
	}
	for checksum, generation := range pending {
		m.snapshotMu.Lock()
		leader, err := m.client.Get(m.ctx, checksum)
		if err != nil {
			m.snapshotMu.Unlock()
			m.state.EnsureBefore(time.Second)
			return fmt.Errorf("fetch leadership: %w", err)
		}
		m.state.Lock()
		latest, err := m.notifications()
		if err == nil {
			m.setCurrent(checksum, leader)
			if latest[checksum] == generation {
				delete(latest, checksum)
			}
			m.state.Set("leadership-pending", latest)
		}
		m.state.Unlock()
		m.snapshotMu.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

// setCurrent fences old election work when leadership is lost. An old hook may
// finish, but its completion cannot acknowledge a later election. State is locked.
func (m *Manager) setCurrent(checksum string, leader bool) {
	var previous bool
	m.state.Get("leadership-current-"+checksum, &previous)
	if previous && !leader {
		var epoch int
		m.state.Get("leadership-epoch-"+checksum, &epoch)
		m.state.Set("leadership-epoch-"+checksum, epoch+1)
	}
	m.state.Set("leadership-current-"+checksum, leader)
	if !leader {
		m.state.Set("leadership-accepted-"+checksum, false)
	}
}

// epoch returns the persisted election generation; missing state starts at zero.
// The caller holds state.
func (m *Manager) epoch(checksum string) int {
	var epoch int
	m.state.Get("leadership-epoch-"+checksum, &epoch)
	return epoch
}
