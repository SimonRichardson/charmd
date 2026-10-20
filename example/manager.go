package example

import (
	"gopkg.in/tomb.v2"

	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

const (
	// FirstTask is the first task kind in the example change.
	FirstTask = "example-first"
	// SecondTask is the dependent task kind in the example change.
	SecondTask = "example-second"
)

// Manager demonstrates a Charmd-style domain manager which registers task kinds
// and creates a change with an explicit dependency.
type Manager struct {
	state  *state.State
	runner *state.TaskRunner
	// Events records completed example handlers for demonstration and tests.
	Events []string
}

// NewManager registers the example task handlers with r.
func NewManager(s *state.State, r *state.TaskRunner) *Manager {
	m := &Manager{state: s, runner: r}
	r.AddHandler(FirstTask, m.first, nil)
	r.AddHandler(SecondTask, m.second, nil)
	return m
}

// Ensure implements overlord.StateManager; this example has no periodic work.
func (m *Manager) Ensure() error {
	return nil
}

// CreateChange creates the two dependent tasks used to demonstrate the runner.
func (m *Manager) CreateChange() (*state.Change, error) {
	m.state.Lock()
	defer m.state.Unlock()
	c := m.state.NewChange("example", "run two dependent example tasks")
	a := m.state.NewTask(FirstTask, "first task")
	b := m.state.NewTask(SecondTask, "second task")
	b.WaitFor(a)
	c.AddTask(a)
	c.AddTask(b)
	m.state.EnsureBefore(0)
	return c, nil
}

func (m *Manager) first(*state.Task, *tomb.Tomb) error {
	m.Events = append(m.Events, "first")
	return nil
}

func (m *Manager) second(*state.Task, *tomb.Tomb) error {
	m.Events = append(m.Events, "second")
	return nil
}
