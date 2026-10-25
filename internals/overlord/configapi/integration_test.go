package configapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SimonRichardson/charmd/internals/config"
	service "github.com/SimonRichardson/charmd/internals/configapi"
	"github.com/SimonRichardson/charmd/internals/overlord/runtimestate"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
	"github.com/SimonRichardson/charmd/internals/runtime/legacy"
	"github.com/canonical/tc"
)

type integrationSuite struct{}

var _ = tc.InternalSuite(&integrationSuite{})

// TestSSEToHook verifies a real SSE notification causes a GET and a separate
// config-changed invocation whose config-get receives the updated service value.
func (*integrationSuite) TestSSEToHook(c *tc.C) {
	checksum := strings.Repeat("a", 64)
	snapshots := &fakeClient{values: map[string]any{"greeting": "initial"}}
	events := make(chan struct{}, 1)
	connected := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/events":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "event: config-changed\ndata: {\"sha256\":%q}\n\n", checksum)
			w.(http.Flusher).Flush()
			select {
			case connected <- struct{}{}:
			default:
			}
			for {
				select {
				case <-events:
					fmt.Fprintf(w, "event: config-changed\ndata: {\"sha256\":%q}\n\n", checksum)
					w.(http.Flusher).Flush()
				case <-r.Context().Done():
					return
				}
			}
		case "/v1/charms/" + checksum + "/config":
			values, _ := snapshots.Get(r.Context(), checksum)
			json.NewEncoder(w).Encode(values)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := service.New(config.Endpoint{URL: server.URL})
	c.Assert(err, tc.IsNil)

	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	manager := NewManager(st, runner, client)
	defer manager.Stop()
	exe, err := os.Executable()
	c.Assert(err, tc.IsNil)

	runtimestate.NewManager(st, runner, &legacy.Runtime{Executable: exe})
	dir := c.MkDir()
	for name, body := range map[string]string{
		"metadata.yaml": "name: watched-charm\n",
		"config.yaml":   "options:\n  greeting:\n    default: default\n",
		"dispatch":      "#!/bin/sh\nset -eu\n[ \"$JUJU_HOOK_NAME\" = config-changed ]\nconfig-get greeting >> history\nconfig-get greeting > observed\n",
	} {
		c.Assert(os.WriteFile(filepath.Join(dir, name), []byte(body), 0755), tc.IsNil)
	}
	st.Lock()
	installed(st, checksum, dir)
	st.Unlock()
	c.Assert(manager.StartUp(), tc.IsNil)

	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		c.Fatal("SSE stream did not connect")
	}
	waitForValue(c, manager, runner, filepath.Join(dir, "observed"), "initial\n")
	snapshots.set(map[string]any{"greeting": "updated"})
	events <- struct{}{}
	waitForValue(c, manager, runner, filepath.Join(dir, "observed"), "updated\n")
	history, err := os.ReadFile(filepath.Join(dir, "history"))
	c.Assert(err, tc.IsNil)

	c.Check(string(history), tc.Equals, "initial\nupdated\n")
	st.Lock()
	defer st.Unlock()
	c.Check(st.Changes(), tc.HasLen, 3)
}

// waitForValue advances real tasks until their observable hook output reaches the expected snapshot.
func waitForValue(c *tc.C, manager *Manager, runner *state.TaskRunner, path, want string) {
	deadline := time.After(10 * time.Second)
	for {
		settle(c, manager, runner)
		data, err := os.ReadFile(path)
		if err == nil && string(data) == want {
			return
		}
		select {
		case <-deadline:
			c.Fatalf("hook output did not reach %q; last output %q, error %v", want, data, err)
		case <-time.After(time.Millisecond):
		}
	}
}
