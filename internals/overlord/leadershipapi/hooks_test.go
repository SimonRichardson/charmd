package leadershipapi

import (
	"context"
	"errors"
	"time"

	"github.com/SimonRichardson/charmd/internals/overlord/state"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
	"github.com/canonical/tc"
)

type hooksSuite struct{}

var _ = tc.InternalSuite(&hooksSuite{})

// TestChangeDuringHookPreservesSnapshotAndPriority holds a real scheduled hook
// open while leadership changes and verifies no other hook starts before release.
func (*hooksSuite) TestChangeDuringHookPreservesSnapshotAndPriority(c *tc.C) {
	client := &fakeClient{}
	entered := make(chan bool, 1)
	release := make(chan struct{})
	var calls []string
	st, runner, manager := fixture(c, client, func(ctx context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		calls = append(calls, hook.Name)
		if len(calls) == 2 {
			entered <- hook.Leader
			select {
			case <-release:
			case <-ctx.Done():
				return charmRuntime.Result{}, ctx.Err()
			}
			c.Check(hook.Leader, tc.Equals, false)
		}
		return charmRuntime.Result{}, nil
	})
	st.Lock()
	install := installation(st, "one")
	st.Unlock()
	settle(c, manager, runner)
	st.Lock()
	configHook(st, install)
	st.Unlock()
	c.Assert(runner.Ensure(), tc.IsNil)

	select {
	case leader := <-entered:
		c.Check(leader, tc.Equals, false)
	case <-time.After(5 * time.Second):
		c.Fatal("config hook did not start")
	}
	client.set(true, nil)
	manager.changed("one")
	c.Assert(manager.Ensure(), tc.IsNil)

	st.Lock()
	configHook(st, install)
	st.Unlock()
	c.Assert(runner.Ensure(), tc.IsNil)

	// The runner's state proves the exclusion while the first hook is held open.
	st.Lock()
	running := 0
	for _, task := range st.Tasks() {
		if task.Kind() == "run-hook" && task.Status() == state.DoingStatus {
			running++
		}
	}
	c.Check(running, tc.Equals, 1)
	st.Unlock()
	close(release)
	runner.Wait()
	settle(c, manager, runner)
	c.Check(calls, tc.DeepEquals, []string{"install", "config-changed", "leader-elected", "start", "config-changed"})
}

// TestGetFailurePreventsCharmExecution verifies unknown leadership cannot silently
// become false and run a charm with an invented snapshot.
func (*hooksSuite) TestGetFailurePreventsCharmExecution(c *tc.C) {
	client := &fakeClient{err: errors.New("leadership unavailable")}
	calls := 0
	st, runner, _ := fixture(c, client, func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		calls++
		return charmRuntime.Result{}, nil
	})
	st.Lock()
	task := installation(st, "one")
	st.Unlock()
	c.Assert(runner.Ensure(), tc.IsNil)

	runner.Wait()
	c.Check(calls, tc.Equals, 0)
	st.Lock()
	c.Check(task.Status(), tc.Equals, state.ErrorStatus)
	st.Unlock()
}

// TestLossAndRegainRequireNewElection verifies loss has no invented Juju hook,
// the next ordinary hook sees false, and a later gain elects before config again.
func (*hooksSuite) TestLossAndRegainRequireNewElection(c *tc.C) {
	client := &fakeClient{leader: true}
	var calls []string
	var leaders []bool
	st, runner, manager := fixture(c, client, func(_ context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		calls = append(calls, hook.Name)
		leaders = append(leaders, hook.Leader)
		return charmRuntime.Result{}, nil
	})
	st.Lock()
	install := installation(st, "one")
	configHook(st, install)
	st.Unlock()
	settle(c, manager, runner)
	client.set(false, nil)
	manager.changed("one")
	st.Lock()
	configHook(st, install)
	st.Unlock()
	settle(c, manager, runner)
	client.set(true, nil)
	manager.changed("one")
	st.Lock()
	configHook(st, install)
	st.Unlock()
	settle(c, manager, runner)
	c.Check(calls, tc.DeepEquals, []string{"install", "leader-elected", "config-changed", "start", "config-changed", "leader-elected", "config-changed"})
	c.Check(leaders, tc.DeepEquals, []bool{true, true, true, true, false, true, true})
}

// TestLossDuringElectionCannotAcknowledgeRegain verifies completion of an older
// leadership hook cannot suppress a new election observed while it was running.
func (*hooksSuite) TestLossDuringElectionCannotAcknowledgeRegain(c *tc.C) {
	client := &fakeClient{leader: true}
	entered := make(chan struct{})
	release := make(chan struct{})
	elections := 0
	st, runner, manager := fixture(c, client, func(ctx context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		if hook.Name == "leader-elected" {
			elections++
			if elections == 1 {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return charmRuntime.Result{}, ctx.Err()
				}
			}
		}
		return charmRuntime.Result{}, nil
	})
	st.Lock()
	installation(st, "one")
	st.Unlock()
	c.Assert(runner.Ensure(), tc.IsNil)

	runner.Wait()
	manager.changed("one")
	c.Assert(manager.Ensure(), tc.IsNil)

	c.Assert(runner.Ensure(), tc.IsNil)

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		c.Fatal("election did not start")
	}
	client.set(false, nil)
	manager.changed("one")
	c.Assert(manager.Ensure(), tc.IsNil)

	client.set(true, nil)
	manager.changed("one")
	c.Assert(manager.Ensure(), tc.IsNil)

	close(release)
	runner.Wait()
	settle(c, manager, runner)
	c.Check(elections, tc.Equals, 2)
}

// TestElectionInheritsActiveConfig verifies a leadership event during config
// execution waits for that hook and preserves its configuration for leader-elected.
func (*hooksSuite) TestElectionInheritsActiveConfig(c *tc.C) {
	client := &fakeClient{}
	entered := make(chan struct{})
	release := make(chan struct{})
	var electedConfig map[string]any
	st, runner, manager := fixture(c, client, func(ctx context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		if hook.Name == "config-changed" {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return charmRuntime.Result{}, ctx.Err()
			}
		}
		if hook.Name == "leader-elected" {
			electedConfig = hook.Config
		}
		return charmRuntime.Result{}, nil
	})
	st.Lock()
	install := installation(st, "one")
	st.Unlock()
	settle(c, manager, runner)
	st.Lock()
	install.Set("hook-config", map[string]any{"status": "world"})
	configHook(st, install)
	st.Unlock()
	c.Assert(runner.Ensure(), tc.IsNil)

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		c.Fatal("config did not start")
	}
	client.set(true, nil)
	manager.changed("one")
	c.Assert(manager.Ensure(), tc.IsNil)

	close(release)
	runner.Wait()
	settle(c, manager, runner)
	c.Check(electedConfig, tc.DeepEquals, map[string]any{"status": "world"})
}

// TestPreflightGainDoesNotWaitForDeferredConfig prevents a dependency cycle when
// GET observes leadership before the watch notification arrives.
func (*hooksSuite) TestPreflightGainDoesNotWaitForDeferredConfig(c *tc.C) {
	client := &fakeClient{}
	var calls []string
	st, runner, manager := fixture(c, client, func(_ context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		calls = append(calls, hook.Name)
		return charmRuntime.Result{}, nil
	})
	st.Lock()
	install := installation(st, "one")
	st.Unlock()
	settle(c, manager, runner)
	st.Lock()
	config := configHook(st, install)
	st.Unlock()
	client.set(true, nil)
	c.Assert(runner.Ensure(), tc.IsNil)

	runner.Wait()
	settle(c, manager, runner)
	c.Check(calls, tc.DeepEquals, []string{"install", "leader-elected", "config-changed", "start"})

	st.Lock()
	defer st.Unlock()
	c.Check(config.Status(), tc.Equals, state.DoneStatus)
}
