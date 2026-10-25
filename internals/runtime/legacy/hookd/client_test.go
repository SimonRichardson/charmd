package hookd

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"

	"github.com/canonical/tc"

	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

type clientSuite struct{}

var _ = tc.InternalSuite(&clientSuite{})

// startClientServer prepares a real invocation socket and the hook-tool client environment.
func startClientServer(c *tc.C) *testContext {
	ctx := &testContext{status: charmRuntime.Status{Status: "maintenance"}}
	socket := filepath.Join(c.MkDir(), "hookd.sock")
	server, err := NewServer(socket, ctx)
	c.Assert(err, tc.IsNil)

	c.Cleanup(server.Close)
	c.Setenv("JUJU_AGENT_SOCKET_NETWORK", "unix")
	c.Setenv("JUJU_AGENT_SOCKET_ADDRESS", socket)
	c.Setenv("JUJU_CONTEXT_ID", ctx.ID())
	return ctx
}

// TestRunTool checks the real RPC path for command output and state changes.
func (*clientSuite) TestRunTool(c *tc.C) {
	ctx := startClientServer(c)
	var stdout, stderr bytes.Buffer
	code := RunTool("config-get", []string{"--format=json", "greeting"}, &stdout, &stderr)
	c.Check(code, tc.Equals, 0)
	c.Check(stdout.String(), tc.Equals, "\"hello\"\n")
	c.Check(stderr.String(), tc.Equals, "")

	code = RunTool("status-set", []string{"active", "ready"}, io.Discard, &stderr)
	c.Check(code, tc.Equals, 0)
	c.Check(ctx.status, tc.Equals, charmRuntime.Status{Status: "active", Message: "ready"})
}

// TestRunToolCommandFailure checks that remote argument errors preserve the tool's exit code and stderr.
func (*clientSuite) TestRunToolCommandFailure(c *tc.C) {
	startClientServer(c)
	var stdout, stderr bytes.Buffer
	code := RunTool("status-set", []string{"invalid"}, &stdout, &stderr)
	c.Check(code, tc.Equals, 1)
	c.Check(stdout.String(), tc.Equals, "")
	c.Check(stderr.String(), tc.Equals, "invalid workload status \"invalid\"\n")
}

// TestRunToolWrongContext checks that RPC identity failures are returned to the calling charm.
func (*clientSuite) TestRunToolWrongContext(c *tc.C) {
	startClientServer(c)
	c.Setenv("JUJU_CONTEXT_ID", "other-hook")
	var stderr bytes.Buffer
	code := RunTool("status-get", nil, io.Discard, &stderr)
	c.Check(code, tc.Equals, 1)
	c.Check(strings.Contains(stderr.String(), "wrong hook context ID"), tc.Equals, true)
}

// TestRunToolInvalidEnvironment checks errors before attempting a command RPC.
func (*clientSuite) TestRunToolInvalidEnvironment(c *tc.C) {
	c.Setenv("JUJU_AGENT_SOCKET_NETWORK", "")
	c.Setenv("JUJU_CONTEXT_ID", "")
	var stderr bytes.Buffer
	c.Check(RunTool("unknown", nil, io.Discard, &stderr), tc.Equals, 1)
	c.Check(stderr.String(), tc.Equals, "unsupported hook tool \"unknown\"\n")

	stderr.Reset()
	c.Check(RunTool("config-get", nil, io.Discard, &stderr), tc.Equals, 1)
	c.Check(stderr.String(), tc.Equals, "hook tool requires a Unix hook context\n")

	c.Setenv("JUJU_AGENT_SOCKET_NETWORK", "unix")
	stderr.Reset()
	c.Check(RunTool("config-get", nil, io.Discard, &stderr), tc.Equals, 1)
	c.Check(stderr.String(), tc.Equals, "hook tool requires JUJU_CONTEXT_ID\n")

	c.Setenv("JUJU_CONTEXT_ID", "hook")
	c.Setenv("JUJU_AGENT_SOCKET_ADDRESS", filepath.Join(c.MkDir(), "missing.sock"))
	stderr.Reset()
	c.Check(RunTool("config-get", nil, io.Discard, &stderr), tc.Equals, 1)
	c.Check(strings.Contains(stderr.String(), "missing.sock"), tc.Equals, true)
}

type failingWriter struct{}

// Write models a broken hook output destination.
func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken output") }

// TestRunToolOutputFailure checks that successful RPCs still fail when their output cannot be delivered.
func (*clientSuite) TestRunToolOutputFailure(c *tc.C) {
	startClientServer(c)
	var stderr bytes.Buffer
	code := RunTool("status-get", nil, failingWriter{}, &stderr)
	c.Check(code, tc.Equals, 1)
	c.Check(stderr.String(), tc.Equals, "broken output\n")

	code = RunTool("status-set", []string{"invalid"}, io.Discard, failingWriter{})
	c.Check(code, tc.Equals, 1)
}
