package overlord

import (
	"fmt"
	"sync"

	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

// StateManager participates in every overlord ensure pass.
type StateManager interface {
	Ensure() error
}

// StateStarterUp is optionally implemented by managers that need startup work.
type StateStarterUp interface {
	StartUp() error
}

// StateWaiter is optionally implemented by managers with background work.
type StateWaiter interface {
	Wait()
}

// StateStopper is optionally implemented by managers with activities to stop.
type StateStopper interface {
	Stop()
}

// StateEngine dispatches lifecycle callbacks to registered managers.
type StateEngine struct {
	mu       sync.Mutex
	managers []StateManager
	state    *state.State
	started  bool
	stopped  bool
}

// NewStateEngine creates an empty manager dispatcher.
func NewStateEngine(s *state.State) *StateEngine {
	return &StateEngine{state: s}
}

// State returns the shared state managed by the engine.
func (e *StateEngine) State() *state.State { return e.state }

// AddManager registers m for subsequent lifecycle callbacks.
func (e *StateEngine) AddManager(m StateManager) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.managers = append(e.managers, m)
}

// StartUp calls the optional StartUp method on each manager.
func (e *StateEngine) StartUp() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.started {
		return nil
	}
	e.started = true
	var errs []error
	for _, manager := range e.managers {
		if starter, ok := manager.(StateStarterUp); ok {
			if err := starter.StartUp(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) != 0 {
		return fmt.Errorf("state startup errors: %v", errs)
	}
	return nil
}

// Ensure runs every manager and reports all errors from the pass.
func (e *StateEngine) Ensure() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.started {
		return fmt.Errorf("state engine skipped startup")
	}
	if e.stopped {
		return fmt.Errorf("state engine already stopped")
	}

	var errs []error
	for _, manager := range e.managers {
		if err := manager.Ensure(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) != 0 {
		return fmt.Errorf("state ensure errors: %v", errs)
	}
	return nil
}

// Wait waits for every manager that implements StateWaiter.
func (e *StateEngine) Wait() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.stopped {
		return
	}
	for _, manager := range e.managers {
		if waiter, ok := manager.(StateWaiter); ok {
			waiter.Wait()
		}
	}
}

// Stop stops managers in reverse registration order and is idempotent.
func (e *StateEngine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stopped {
		return
	}
	for i := len(e.managers) - 1; i >= 0; i-- {
		if stopper, ok := e.managers[i].(StateStopper); ok {
			stopper.Stop()
		}
	}
	e.stopped = true
}
