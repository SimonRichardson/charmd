//go:build linux

package configapi

import (
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

// TestFileToHook verifies local config events follow the same manager and hookd
// pipeline as SSE, with semantic comparison owned entirely by the file client.
func (*integrationSuite) TestFileToHook(c *tc.C) {
	checksum := strings.Repeat("a", 64)
	source := filepath.Join(c.MkDir(), "source.yaml")
	original := "charms:\n  " + checksum + ": {greeting: initial}\n"
	c.Assert(os.WriteFile(source, []byte(original), 0644), tc.IsNil)

	client, err := service.New(config.Endpoint{Path: source})
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
		"metadata.yaml": "name: file-config-charm\n",
		"config.yaml":   "options:\n  greeting:\n    default: default\n",
		"dispatch":      "#!/bin/sh\nset -eu\n[ \"$JUJU_HOOK_NAME\" = config-changed ]\nconfig-get greeting > observed\n",
	} {
		c.Assert(os.WriteFile(filepath.Join(dir, name), []byte(body), 0755), tc.IsNil)
	}
	st.Lock()
	installed(st, checksum, dir)
	st.Unlock()
	c.Assert(manager.StartUp(), tc.IsNil)

	waitForValue(c, manager, runner, filepath.Join(dir, "observed"), "initial\n")
	c.Assert(os.WriteFile(source, []byte("# comment-only edit\n"+original), 0644), tc.IsNil)

	// Give the native watcher time to process the equivalent document before
	// checking that it has not requested any additional changes.
	deadline := time.After(100 * time.Millisecond)
	waiting := true
	for waiting {
		settle(c, manager, runner)
		select {
		case <-deadline:
			waiting = false
		case <-time.After(time.Millisecond):
		}
	}
	st.Lock()
	c.Check(st.Changes(), tc.HasLen, 2)
	st.Unlock()
	c.Assert(os.WriteFile(source, []byte(strings.Replace(original, "initial", "updated", 1)), 0644), tc.IsNil)

	waitForValue(c, manager, runner, filepath.Join(dir, "observed"), "updated\n")
	st.Lock()
	defer st.Unlock()
	c.Check(st.Changes(), tc.HasLen, 3)
}
