package leadershipapi

import (
	"bytes"
	"context"
	"errors"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
	"time"

	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
	"github.com/canonical/tc"
)

type managerSuite struct{}

var _ = tc.InternalSuite(&managerSuite{})

// TestLifecycle verifies idempotent startup and shutdown cancel an idle stream.
func (*managerSuite) TestLifecycle(c *tc.C) {
	entered := make(chan struct{})
	client := &fakeClient{watch: func(ctx context.Context, _ func(string)) error { close(entered); <-ctx.Done(); return ctx.Err() }}
	_, _, manager := fixture(c, client, func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		return charmRuntime.Result{}, nil
	})
	c.Assert(manager.StartUp(), tc.IsNil)

	c.Assert(manager.StartUp(), tc.IsNil)

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		c.Fatal("watch did not start")
	}
	manager.Stop()
	manager.Stop()
	c.Check(manager.StartUp(), tc.ErrorMatches, "leadership manager is stopped")
}

// TestFailedRefreshRetainsNotification verifies an outage cannot drop the only
// notification; a later successful GET reconciles it without another API event.
func (*managerSuite) TestFailedRefreshRetainsNotification(c *tc.C) {
	client := &fakeClient{err: errors.New("offline")}
	st, _, manager := fixture(c, client, func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		return charmRuntime.Result{}, nil
	})
	manager.changed("one")
	c.Check(manager.Ensure(), tc.ErrorMatches, "fetch leadership: offline")
	st.Lock()
	pending, err := manager.notifications()
	st.Unlock()
	c.Assert(err, tc.IsNil)

	c.Check(pending, tc.DeepEquals, map[string]int{"one": 1})
	client.set(true, nil)
	c.Assert(manager.Ensure(), tc.IsNil)

	st.Lock()
	pending, err = manager.notifications()
	var leader bool
	snapshotErr := st.Get("leadership-current-one", &leader)
	st.Unlock()
	c.Assert(err, tc.IsNil)

	c.Check(pending, tc.HasLen, 0)
	c.Assert(snapshotErr, tc.IsNil)

	c.Check(leader, tc.Equals, true)
}

// TestNotificationDuringGetRemainsPending verifies reconciliation performs I/O
// without holding state and cannot acknowledge a notification received mid-fetch.
func (*managerSuite) TestNotificationDuringGetRemainsPending(c *tc.C) {
	entered := make(chan struct{})
	release := make(chan struct{})
	client := &fakeClient{get: func(ctx context.Context, _ string) (bool, error) {
		close(entered)
		select {
		case <-release:
			return true, nil
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}}
	st, _, manager := fixture(c, client, func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		return charmRuntime.Result{}, nil
	})
	manager.changed("one")
	done := make(chan error, 1)
	go func() { done <- manager.Ensure() }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		c.Fatal("fetch did not start")
	}
	// This would deadlock if reconciliation held state across the GET.
	notified := make(chan struct{})
	go func() { manager.changed("one"); close(notified) }()
	select {
	case <-notified:
	case <-time.After(5 * time.Second):
		c.Fatal("GET blocked notifications")
	}
	close(release)
	c.Assert(<-done, tc.IsNil)

	st.Lock()
	pending, err := manager.notifications()
	st.Unlock()
	c.Assert(err, tc.IsNil)

	c.Check(pending, tc.DeepEquals, map[string]int{"one": 2})
}

// TestPendingNotificationSurvivesRestart verifies the state checkpoint retains
// unprocessed elections, which are fetched and scheduled after restoration.
func (*managerSuite) TestPendingNotificationSurvivesRestart(c *tc.C) {
	client := &fakeClient{leader: true}
	st, _, manager := fixture(c, client, func(context.Context, charmRuntime.Hook) (charmRuntime.Result, error) {
		return charmRuntime.Result{}, nil
	})
	manager.changed("one")
	st.Lock()
	install := installation(st, "one")
	install.Set("charm-install-path", "/charm/one")
	install.SetStatus(state.DoneStatus)
	data, err := st.MarshalJSON()
	st.Unlock()
	c.Assert(err, tc.IsNil)

	restored, err := state.ReadState(nil, bytes.NewReader(data))
	c.Assert(err, tc.IsNil)

	runner := state.NewTaskRunner(restored)
	next := NewManager(restored, runner, client)
	defer next.Stop()
	defer runner.Stop()
	c.Assert(next.Ensure(), tc.IsNil)

	restored.Lock()
	changes := restored.Changes()
	pending, err := next.notifications()
	restored.Unlock()
	c.Assert(err, tc.IsNil)

	c.Check(pending, tc.HasLen, 0)
	c.Check(changes, tc.HasLen, 2)
}
