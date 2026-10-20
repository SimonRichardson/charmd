// Package charmstate owns charm changes and their task handlers.
package charmstate

import (
	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

// Manager will create and execute charm fetch and installation tasks.
type Manager struct {
	state    *state.State
	charmAPI *charmapi.Client
}

// NewManager constructs the manager with its configured charm API dependency.
func NewManager(s *state.State, charmAPI *charmapi.Client) *Manager {
	if charmAPI == nil {
		panic("charm API client is required")
	}
	return &Manager{state: s, charmAPI: charmAPI}
}

// Ensure implements overlord.StateManager. Charm tasks will schedule their own
// work through the shared task runner.
func (m *Manager) Ensure() error { return nil }
