package legacy

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/runtime/legacy/hookd"
)

type sessionSuite struct{}

var _ = tc.InternalSuite(&sessionSuite{})

// TestSessionIsolation checks private, independent tool directories and that repeated cleanup leaves other invocations intact.
func (*sessionSuite) TestSessionIsolation(c *tc.C) {
	dir := charm(c, "", "")
	ctx, err := NewHookContext(dir, nil)
	c.Assert(err, tc.IsNil)

	exe, err := os.Executable()
	c.Assert(err, tc.IsNil)

	first, err := newSession(exe, dir, ctx)
	c.Assert(err, tc.IsNil)

	defer first.Close()
	second, err := newSession(exe, dir, ctx)
	c.Assert(err, tc.IsNil)

	defer second.Close()
	c.Check(first.paths.ToolsDir != second.paths.ToolsDir, tc.Equals, true)
	info, err := os.Stat(first.paths.ToolsDir)
	c.Assert(err, tc.IsNil)

	c.Check(info.Mode().Perm(), tc.Equals, os.FileMode(0700))
	for _, tool := range hookd.ToolNames() {
		target, err := os.Readlink(filepath.Join(first.paths.ToolsDir, tool))
		c.Assert(err, tc.IsNil)

		c.Check(target, tc.Equals, exe)
	}

	first.Close()
	first.Close()
	_, err = os.Stat(first.paths.ToolsDir)
	c.Check(os.IsNotExist(err), tc.Equals, true)
	_, err = os.Stat(second.paths.Socket)
	c.Check(err, tc.IsNil)
	_, err = os.Stat(filepath.Join(dir, "metadata.yaml"))
	c.Check(err, tc.IsNil)
}

// TestPreparationFailures checks invalid executable setup and temporary-directory errors return no session.
func (*sessionSuite) TestPreparationFailures(c *tc.C) {
	ctx, err := NewHookContext(charm(c, "", ""), nil)
	c.Assert(err, tc.IsNil)

	session, err := newSession("", "/charm", ctx)
	c.Check(err, tc.ErrorMatches, "hook tool executable is required")
	c.Check(session, tc.IsNil)

	c.Setenv("TMPDIR", filepath.Join(c.MkDir(), "absent"))
	session, err = newSession("/binary", "/charm", ctx)
	c.Check(err, tc.NotNil)
	c.Check(session, tc.IsNil)
}

// TestServerFailureCleansTools checks socket setup errors remove partially prepared invocation files.
func (*sessionSuite) TestServerFailureCleansTools(c *tc.C) {
	ctx, err := NewHookContext(charm(c, "", ""), nil)
	c.Assert(err, tc.IsNil)

	base := filepath.Join(c.MkDir(), strings.Repeat("long-path", 16))
	c.Assert(os.Mkdir(base, 0755), tc.IsNil)

	c.Setenv("TMPDIR", base)
	session, err := newSession("/binary", "/charm", ctx)
	c.Check(err, tc.NotNil)
	c.Check(session, tc.IsNil)

	files, err := os.ReadDir(base)
	c.Assert(err, tc.IsNil)

	c.Check(files, tc.HasLen, 0)
}
