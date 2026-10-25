package leadershipapi

import (
	"context"
	"errors"

	"github.com/SimonRichardson/charmd/internals/overlord/state"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
	"github.com/canonical/tc"
)

type changesSuite struct{}

var _ = tc.InternalSuite(&changesSuite{})

// TestInstallElectionConfigOrder verifies pre-install notifications are retained,
// repeated events do not duplicate elections, and installation cannot be bypassed.
func (*changesSuite) TestInstallElectionConfigOrder(c *tc.C) {
	client := &fakeClient{leader: true}
	var calls []string
	st, runner, manager := fixture(c, client, func(_ context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		calls = append(calls, hook.Name)
		c.Check(hook.Leader, tc.Equals, true)
		return charmRuntime.Result{}, nil
	})
	manager.changed("one")
	c.Assert(manager.Ensure(), tc.IsNil)

	st.Lock()
	c.Check(st.Changes(), tc.HasLen, 0)
	install := installation(st, "one")
	config := configHook(st, install)
	st.Unlock()
	settle(c, manager, runner)
	c.Check(calls, tc.DeepEquals, []string{"install", "leader-elected", "config-changed"})
	st.Lock()
	c.Check(config.Status(), tc.Equals, state.DoneStatus)
	c.Check(st.Changes(), tc.HasLen, 3)
	st.Unlock()
	for range 3 {
		manager.changed("one")
	}
	settle(c, manager, runner)
	c.Check(calls, tc.HasLen, 3)
}

// TestFailedElectionBlocksConfig verifies acceptance requires success and ordinary
// hooks cannot run through an election failure. Loss releases that priority gate.
func (*changesSuite) TestFailedElectionBlocksConfig(c *tc.C) {
	client := &fakeClient{leader: true}
	var calls []string
	st, runner, manager := fixture(c, client, func(_ context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		calls = append(calls, hook.Name)
		if hook.Name == "leader-elected" {
			return charmRuntime.Result{}, errors.New("election failed")
		}
		return charmRuntime.Result{}, nil
	})
	st.Lock()
	install := installation(st, "one")
	config := configHook(st, install)
	st.Unlock()
	settle(c, manager, runner)
	c.Check(calls, tc.DeepEquals, []string{"install", "leader-elected"})
	st.Lock()
	c.Check(config.Status(), tc.Equals, state.DoStatus)
	st.Unlock()
	client.set(false, nil)
	manager.changed("one")
	settle(c, manager, runner)
	c.Check(calls, tc.DeepEquals, []string{"install", "leader-elected", "config-changed"})
}

// TestStaleElectionIsSkipped verifies a queued gain cannot execute after leadership loss.
func (*changesSuite) TestStaleElectionIsSkipped(c *tc.C) {
	client := &fakeClient{leader: true}
	var calls []string
	st, runner, manager := fixture(c, client, func(_ context.Context, hook charmRuntime.Hook) (charmRuntime.Result, error) {
		calls = append(calls, hook.Name)
		return charmRuntime.Result{}, nil
	})
	st.Lock()
	installation(st, "one")
	st.Unlock()
	c.Assert(runner.Ensure(), tc.IsNil)

	runner.Wait()
	manager.changed("one")
	c.Assert(manager.Ensure(), tc.IsNil)

	client.set(false, nil)
	settle(c, manager, runner)
	c.Check(calls, tc.DeepEquals, []string{"install"})
	st.Lock()
	for _, change := range st.Changes() {
		c.Check(change.Status(), tc.Equals, state.DoneStatus)
	}
	st.Unlock()
}
