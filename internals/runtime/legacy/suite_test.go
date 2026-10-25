package legacy

import (
	"github.com/SimonRichardson/charmd/internals/runtime/legacy/hookd"
	"github.com/canonical/tc"
	"os"
	"path/filepath"
	"testing"
)

// Test connects the tc suites to the Go test runner.
func Test(t *testing.T) { tc.InternalTestingT(t) }

// The test binary supplies the same multicall entry point as cmd/charmd.
func TestMain(m *testing.M) {
	if name := filepath.Base(os.Args[0]); hookd.IsTool(name) {
		os.Exit(hookd.RunTool(name, os.Args[1:], os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

// charm writes a minimal charm with an optional executable hook handler.
func charm(c *tc.C, script, name string) string {
	dir := c.MkDir()
	c.Assert(os.WriteFile(filepath.Join(dir, "metadata.yaml"), []byte("name: test\n"), 0644), tc.IsNil)

	if name != "" {
		path := filepath.Join(dir, name)
		c.Assert(os.MkdirAll(filepath.Dir(path), 0755), tc.IsNil)

		c.Assert(os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+script), 0755), tc.IsNil)
	}
	return dir
}
