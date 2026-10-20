// Package charmstate owns charm changes and their task handlers.
package charmstate

import (
	"fmt"
	"strings"

	"gopkg.in/tomb.v2"

	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

const (
	// FetchCharmTask resolves and downloads the requested charm.
	FetchCharmTask = "fetch-charm"
	// InstallCharmTask installs a charm fetched by FetchCharmTask.
	InstallCharmTask = "install-charm"
)

const charmNameKey = "charm-name"

// Manager will create and execute charm fetch and installation tasks.
type Manager struct {
	state    *state.State
	runner   *state.TaskRunner
	charmAPI *charmapi.Client
}

// NewManager constructs the manager and registers its task handlers with r.
func NewManager(s *state.State, r *state.TaskRunner, charmAPI *charmapi.Client) *Manager {
	if s == nil {
		panic("state is required")
	}
	if r == nil {
		panic("task runner is required")
	}
	if charmAPI == nil {
		panic("charm API client is required")
	}
	m := &Manager{state: s, runner: r, charmAPI: charmAPI}
	r.AddHandler(FetchCharmTask, m.fetchCharm, nil)
	r.AddHandler(InstallCharmTask, m.installCharm, nil)
	return m
}

// Ensure implements overlord.StateManager. Charm tasks will schedule their own
// work through the shared task runner.
func (m *Manager) Ensure() error { return nil }

// Install creates the change that fetches and then installs charm.
func (m *Manager) Install(charm string) (*state.Change, error) {
	if strings.TrimSpace(charm) == "" {
		return nil, fmt.Errorf("charm name is required")
	}

	m.state.Lock()
	defer m.state.Unlock()

	change := m.state.NewChange("install-charm", fmt.Sprintf("Install charm %q", charm))
	fetch := m.state.NewTask(FetchCharmTask, fmt.Sprintf("Fetch charm %q", charm))
	install := m.state.NewTask(InstallCharmTask, fmt.Sprintf("Install charm %q", charm))

	fetch.Set(charmNameKey, charm)
	install.Set(charmNameKey, charm)

	install.WaitFor(fetch)

	change.AddTask(fetch)
	change.AddTask(install)

	m.state.EnsureBefore(0)

	return change, nil
}

// fetchCharm is a placeholder until the charm API resolve/download contract is defined.
func (m *Manager) fetchCharm(*state.Task, *tomb.Tomb) error { return nil }

// installCharm is a placeholder until fetched charm installation is defined.
func (m *Manager) installCharm(*state.Task, *tomb.Tomb) error { return nil }
