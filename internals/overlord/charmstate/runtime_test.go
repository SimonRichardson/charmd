package charmstate

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/overlord/runtimestate"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
	"github.com/SimonRichardson/charmd/internals/runtime/legacy"
	"github.com/SimonRichardson/charmd/internals/runtime/legacy/hookd"
)

// TestMain dispatches hook-tool subprocesses before running the tc suites.
func TestMain(m *testing.M) {
	if name := filepath.Base(os.Args[0]); hookd.IsTool(name) {
		os.Exit(hookd.RunTool(name, os.Args[1:], os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

// TestDownloadedCharmRunsInstall exercises fetch, extraction, hook execution, and cached reinstallation.
func (s *managerSuite) TestDownloadedCharmRunsInstall(c *tc.C) { s.testDownloadedCharm(c, false) }

// TestDownloadedCharmHookFails verifies that hook failure reaches the install change.
func (s *managerSuite) TestDownloadedCharmHookFails(c *tc.C) { s.testDownloadedCharm(c, true) }

// testDownloadedCharm runs an archived charm through the real task and hook-tool pipeline.
func (*managerSuite) testDownloadedCharm(c *tc.C, fail bool) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	script := "#!/bin/sh\nset -eu\nstatus-set maintenance installing\njuju-log installing\nprintf installed > installed\n"
	if fail {
		script += "exit 23\n"
	}
	for path, body := range map[string]string{"metadata.yaml": "name: downloaded\n", "hooks/install": script} {
		header := &zip.FileHeader{Name: path, Method: zip.Deflate}
		header.SetMode(0755)
		writer, err := archive.CreateHeader(header)
		c.Assert(err, tc.IsNil)

		_, err = writer.Write([]byte(body))
		c.Assert(err, tc.IsNil)
	}
	c.Assert(archive.Close(), tc.IsNil)

	api := newCharmAPI()
	api.archive = buffer.Bytes()
	checksum := sha256.Sum256(api.archive)
	api.info.SHA256 = hex.EncodeToString(checksum[:])
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	exe, err := os.Executable()
	c.Assert(err, tc.IsNil)

	runtimestate.NewManager(st, runner, &legacy.Runtime{Executable: exe})
	dir := c.MkDir()
	manager := NewManager(st, runner, api, dir)
	// Reinstallation exercises the cached directory's persisted install path.
	for attempt := 0; attempt < 2; attempt++ {
		change, err := manager.Install(api.info.SHA256)
		c.Assert(err, tc.IsNil)

		for step := 0; step < 6; step++ {
			c.Assert(runner.Ensure(), tc.IsNil)

			runner.Wait()
		}
		st.Lock()
		tasks := change.Tasks()
		status := tasks[2].Status()
		var result charmRuntime.Result
		err = tasks[2].Get("hook-result", &result)
		changeErr := change.Err()
		st.Unlock()
		c.Assert(err, tc.IsNil)

		c.Assert(result.Status, tc.NotNil)

		c.Check(result.Status.Status, tc.Equals, "maintenance")
		if fail {
			c.Check(status, tc.Equals, state.ErrorStatus)
			c.Check(changeErr, tc.NotNil)
		} else {
			c.Check(status, tc.Equals, state.DoneStatus)
			c.Check(changeErr, tc.IsNil)
		}
		data, err := os.ReadFile(filepath.Join(dir, api.info.SHA256, "installed"))
		c.Assert(err, tc.IsNil)

		c.Check(string(data), tc.Equals, "installed")
	}
}
