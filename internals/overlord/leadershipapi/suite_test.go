package leadershipapi

import (
	"context"
	"github.com/SimonRichardson/charmd/internals/runtime/legacy/hookd"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/SimonRichardson/charmd/internals/overlord/runtimestate"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
	"github.com/canonical/tc"
)

// Test connects focused tc suites to the Go test runner.
func Test(t *testing.T) { tc.InternalTestingT(t) }

// fakeClient supplies synchronized snapshots and a controllable subscription.
type fakeClient struct {
	mu     sync.Mutex
	leader bool
	err    error
	watch  func(context.Context, func(string)) error
	get    func(context.Context, string) (bool, error)
}

// Get returns the snapshot or failure configured by the test.
func (f *fakeClient) Get(ctx context.Context, checksum string) (bool, error) {
	if f.get != nil {
		return f.get(ctx, checksum)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.leader, f.err
}

// Watch runs the controlled stream or blocks until cancellation.
func (f *fakeClient) Watch(ctx context.Context, notify func(string)) error {
	if f.watch != nil {
		return f.watch(ctx, notify)
	}
	<-ctx.Done()
	return ctx.Err()
}

// set publishes an atomic service snapshot between hook invocations.
func (f *fakeClient) set(leader bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.leader, f.err = leader, err
}

type runtimeFunc func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error)

// RunHook adapts a test function to the runtime boundary.
func (f runtimeFunc) RunHook(ctx context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
	return f(ctx, hook)
}

// fixture constructs the real shared scheduler and both cooperating managers.
func fixture(c *tc.C, client *fakeClient, run runtimeFunc) (*state.State, *state.TaskRunner, *Manager) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	runtime := runtimestate.NewManager(st, runner, run)
	manager := NewManager(st, runner, client)
	runtime.Leadership = manager.HookLeadership
	c.Cleanup(func() { runner.Stop(); manager.Stop() })
	return st, runner, manager
}

// installation creates an unpacked charm and a pending install hook under state lock.
func installation(st *state.State, checksum string) *state.Task {
	unpack := st.NewTask("unpack", "Unpack charm")
	unpack.Set("charm-install-path", "/charm/"+checksum)
	unpack.Set("charm-sha256", checksum)
	unpack.SetStatus(state.DoneStatus)
	install := runtimestate.InstallTask(st, unpack)
	change := st.NewChange("install-charm", "Install charm")
	change.AddTask(unpack)
	change.AddTask(install)
	return install
}

// configHook queues an independent ordinary hook after installation.
func configHook(st *state.State, install *state.Task) *state.Task {
	task := runtimestate.HookTask(st, install, "config-changed")
	change := st.NewChange("change-config", "Apply config")
	change.AddTask(task)
	return task
}

// settle advances reconciliation and task execution without arbitrary sleeps.
func settle(c *tc.C, manager *Manager, runner *state.TaskRunner) {
	for range 6 {
		c.Assert(manager.Ensure(), tc.IsNil)

		c.Assert(runner.Ensure(), tc.IsNil)

		runner.Wait()
	}
}

// TestMain dispatches real hook-tool subprocesses before the tc suites.
func TestMain(m *testing.M) {
	if name := filepath.Base(os.Args[0]); hookd.IsTool(name) {
		os.Exit(hookd.RunTool(name, os.Args[1:], os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}
