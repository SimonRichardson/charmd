// Package configapi turns configuration-service notifications into durable changes.
package configapi

import (
	"context"
	"fmt"
	"sync"
	"time"

	service "github.com/SimonRichardson/charmd/internals/configapi"
	"github.com/SimonRichardson/charmd/internals/logger"
	"github.com/SimonRichardson/charmd/internals/overlord/runtimestate"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

// FetchConfigTask retrieves the snapshot consumed by a config-changed hook.
const FetchConfigTask = "fetch-config"

// Manager watches config notifications and schedules independent config changes.
// Every notification becomes a change; the service owns config comparison.
// State owns hook scheduling and guarantees no two hooks execute together.
type Manager struct {
	state            *state.State
	client           service.Client
	ctx              context.Context
	cancel           context.CancelFunc
	done             chan struct{}
	mu               sync.Mutex
	started, stopped bool
	retryDelay       time.Duration
	observerID       int
}

// NewManager registers the config fetch handler without starting network activity.
func NewManager(st *state.State, runner *state.TaskRunner, client service.Client) *Manager {
	if st == nil || runner == nil || client == nil {
		panic("state, task runner and config API client are required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{state: st, client: client, ctx: ctx, cancel: cancel, done: make(chan struct{}), retryDelay: time.Second}
	runner.AddHandler(FetchConfigTask, m.fetchConfig, nil)
	st.Lock()
	m.observerID = st.AddTaskStatusChangedHandler(m.hookCompleted)
	st.Unlock()
	return m
}

// StartUp starts the subscription once. Unavailable services reconnect in the background.
func (m *Manager) StartUp() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return fmt.Errorf("config API manager is stopped")
	}
	if !m.started {
		m.started = true
		go m.watch()
	}
	return nil
}

// Stop cancels streaming requests and waits for the watcher, including before startup.
func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.stopped {
		m.stopped = true
		m.cancel()
		m.state.Lock()
		m.state.RemoveTaskStatusChangedHandler(m.observerID)
		m.state.Unlock()
		if !m.started {
			close(m.done)
		}
	}
	m.mu.Unlock()
	<-m.done
}

// watch reconnects after disconnects without synthesizing config notifications.
// The service is responsible for reporting initial and subsequent config changes.
func (m *Manager) watch() {
	defer close(m.done)
	for m.ctx.Err() == nil {
		if err := m.client.Watch(m.ctx, m.changed); err != nil && m.ctx.Err() == nil {
			logger.Noticef("config API watch: %v", err)
		}
		timer := time.NewTimer(m.retryDelay)
		select {
		case <-m.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// changed persists each notification before waking the ensure loop. Notifications
// received before install finishes are retained until its hook succeeds.
func (m *Manager) changed(checksum string) {
	m.state.Lock()
	defer m.state.Unlock()
	events, err := pendingEvents(m.state)
	if err != nil {
		logger.Noticef("read pending config events: %v", err)
		return
	}
	events[checksum]++
	m.state.Set(pendingEventsKey, events)
	m.state.EnsureBefore(0)
}

// hookCompleted wakes reconciliation after installation, including when its last
// task has no dependents. State invokes this callback with its lock held.
func (m *Manager) hookCompleted(task *state.Task, _, status state.Status) {
	if status != state.DoneStatus || task.Kind() != runtimestate.RunHookTask {
		return
	}
	var name string
	if err := task.Get("hook-name", &name); err == nil && name == "install" {
		m.state.EnsureBefore(0)
	}
}
