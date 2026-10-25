package configapi

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/SimonRichardson/charmd/internals/overlord/runtimestate"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
	"github.com/SimonRichardson/charmd/internals/runtime/legacy/hookd"
	"github.com/canonical/tc"
)

// Test connects focused tc suites to the Go runner.
func Test(t *testing.T) { tc.InternalTestingT(t) }

// TestMain dispatches real hook-tool subprocesses before starting test suites.
func TestMain(m *testing.M) {
	if name := filepath.Base(os.Args[0]); hookd.IsTool(name) {
		os.Exit(hookd.RunTool(name, os.Args[1:], os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

// fakeClient supplies synchronized config snapshots and a controlled watch stream.
type fakeClient struct {
	mu     sync.Mutex
	values map[string]any
	err    error
	gets   int
	watch  func(context.Context, func(string)) error
}

// Get returns the current snapshot or configured failure under the client lock.
func (f *fakeClient) Get(context.Context, string) (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	values := make(map[string]any, len(f.values))
	for k, v := range f.values {
		values[k] = v
	}
	return values, f.err
}

// Watch delegates to the test stream or blocks until cancellation.
func (f *fakeClient) Watch(ctx context.Context, notify func(string)) error {
	if f.watch != nil {
		return f.watch(ctx, notify)
	}
	<-ctx.Done()
	return ctx.Err()
}

// set changes the service snapshot between reconciliations.
func (f *fakeClient) set(values map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values = values
}

type runtimeFunc func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error)

// RunHook adapts hook assertions to the runtime interface.
func (f runtimeFunc) RunHook(ctx context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
	return f(ctx, hook)
}

// installed creates a completed installation with a durable checksum and path.
// The caller holds the state lock.
func installed(st *state.State, checksum, dir string) *state.Task {
	unpack := st.NewTask("install-charm", "Unpack charm")
	unpack.Set("charm-install-path", dir)
	unpack.SetStatus(state.DoneStatus)
	hook := runtimestate.InstallTask(st, unpack)
	hook.Set("charm-sha256", checksum)
	hook.Set("charm-install-path", dir)
	hook.SetStatus(state.DoneStatus)
	change := st.NewChange("install-charm", "Install charm")
	change.AddTask(unpack)
	change.AddTask(hook)
	return hook
}

// settle advances scheduling and handler completion without relying on wall-clock sleeps.
func settle(c *tc.C, manager *Manager, runner *state.TaskRunner) {
	for range 5 {
		c.Assert(manager.Ensure(), tc.IsNil)

		c.Assert(runner.Ensure(), tc.IsNil)

		runner.Wait()
	}
}

// discardRuntime executes nothing while preserving successful hook semantics.
func discardRuntime(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
	return charmRuntime.Result{}, nil
}
