package configapi

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/SimonRichardson/charmd/internals/overlord/runtimestate"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
	"github.com/canonical/tc"
)

type managerSuite struct{}

var _ = tc.InternalSuite(&managerSuite{})

// TestWatchReconnects verifies disconnects retry and shutdown cancels an idle subscription.
func (*managerSuite) TestWatchReconnects(c *tc.C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	calls := make(chan struct{}, 3)
	attempts := 0
	client := &fakeClient{watch: func(ctx context.Context, notify func(string)) error {
		attempts++
		notify(strings.Repeat("a", 64))
		calls <- struct{}{}
		if attempts == 1 {
			return io.EOF
		}
		<-ctx.Done()
		return ctx.Err()
	}}
	manager := NewManager(st, runner, client)
	manager.retryDelay = time.Millisecond
	defer manager.Stop()
	c.Assert(manager.StartUp(), tc.IsNil)

	c.Assert(manager.StartUp(), tc.IsNil)

	for range 2 {
		select {
		case <-calls:
		case <-time.After(5 * time.Second):
			c.Fatal("watch did not reconnect")
		}
	}
	manager.Stop()
	manager.Stop()
	c.Check(attempts, tc.Equals, 2)
	c.Check(manager.StartUp(), tc.NotNil)
	st.Lock()
	defer st.Unlock()
	events, err := pendingEvents(st)
	c.Assert(err, tc.IsNil)

	c.Check(events[strings.Repeat("a", 64)], tc.Equals, 2)
}

// TestStopBeforeStartup verifies lifecycle idempotency without starting network work.
func (*managerSuite) TestStopBeforeStartup(c *tc.C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	manager := NewManager(st, runner, &fakeClient{})
	manager.Stop()
	manager.Stop()
	c.Check(manager.StartUp(), tc.NotNil)
}

// TestMissingDependencies verifies manager construction rejects incomplete wiring.
func (*managerSuite) TestMissingDependencies(c *tc.C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	c.Check(func() { NewManager(nil, runner, &fakeClient{}) }, tc.PanicMatches, "state, task runner and config API client are required")
	c.Check(func() { NewManager(st, nil, &fakeClient{}) }, tc.PanicMatches, "state, task runner and config API client are required")
	c.Check(func() { NewManager(st, runner, nil) }, tc.PanicMatches, "state, task runner and config API client are required")
}

// schedulingBackend records ensure requests without introducing periodic retries.
type schedulingBackend struct{ wake chan struct{} }

// Checkpoint discards checkpoints because this fixture exercises scheduling only.
func (*schedulingBackend) Checkpoint([]byte) error { return nil }

// NeedsCheckpoint disables persistence for the scheduling fixture.
func (*schedulingBackend) NeedsCheckpoint() bool { return false }

// EnsureBefore records a wake request without blocking a task status callback.
func (b *schedulingBackend) EnsureBefore(time.Duration) {
	select {
	case b.wake <- struct{}{}:
	default:
	}
}

// TestInstallCompletionWakesConfig verifies a pending notification is processed
// promptly after the final install task finishes, without a periodic ensure pass.
func (*managerSuite) TestInstallCompletionWakesConfig(c *tc.C) {
	backend := &schedulingBackend{wake: make(chan struct{}, 16)}
	st := state.New(backend)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	checksum := strings.Repeat("a", 64)
	st.Lock()
	install := installed(st, checksum, "/charm")
	install.SetStatus(state.DoingStatus)
	st.Unlock()
	manager := NewManager(st, runner, &fakeClient{})
	defer manager.Stop()
	manager.changed(checksum)
	<-backend.wake
	c.Assert(manager.Ensure(), tc.IsNil)

	st.Lock()
	c.Check(st.Changes(), tc.HasLen, 1)
	install.SetStatus(state.DoneStatus)
	st.Unlock()
	select {
	case <-backend.wake:
	case <-time.After(time.Second):
		c.Fatal("install completion did not wake config reconciliation")
	}
	c.Assert(manager.Ensure(), tc.IsNil)

	st.Lock()
	c.Check(st.Changes(), tc.HasLen, 2)
	st.Unlock()
	// Removing the observer on stop prevents future task transitions from
	// scheduling a manager whose subscription has already been shut down.
	manager.Stop()
	for len(backend.wake) > 0 {
		<-backend.wake
	}
	st.Lock()
	other := st.NewTask(runtimestate.RunHookTask, "Another install hook")
	other.Set("hook-name", "install")
	other.SetStatus(state.DoneStatus)
	st.Unlock()
	c.Check(len(backend.wake), tc.Equals, 0)
}
