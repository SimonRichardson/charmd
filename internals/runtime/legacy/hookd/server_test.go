package hookd

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/canonical/tc"

	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

// Test connects the tc suites to the Go test runner.
func Test(t *testing.T) { tc.InternalTestingT(t) }

type serverSuite struct{}

var _ = tc.InternalSuite(&serverSuite{})

type testContext struct{ status charmRuntime.Status }

// ID returns the fixture invocation identity.
func (*testContext) ID() string { return "test-context" }

// Config returns fixture configuration for hook command tests.
func (*testContext) Config() map[string]any { return map[string]any{"greeting": "hello"} }

// Status returns the fixture workload status.
func (c *testContext) Status() charmRuntime.Status { return c.status }

// SetStatus records the workload status in the fixture.
func (c *testContext) SetStatus(status charmRuntime.Status) { c.status = status }

// Log discards messages from the fixture hook context.
func (*testContext) Log(string) error { return nil }

// TestCommandErrorDoesNotMutateContext verifies command errors preserve the existing workload status.
func (*serverSuite) TestCommandErrorDoesNotMutateContext(c *tc.C) {
	ctx := &testContext{status: charmRuntime.Status{Status: "maintenance"}}
	handler := NewHandler(ctx)
	var response Response
	err := handler.Main(Request{ContextID: ctx.ID(), Dir: "/", CommandName: "status-set", Args: []string{"invalid"}}, &response)
	c.Assert(err, tc.IsNil)

	c.Check(response.Code, tc.Equals, 1)
	c.Check(string(response.Stderr), tc.Equals, "invalid workload status \"invalid\"\n")
	c.Check(ctx.status.Status, tc.Equals, "maintenance")
}

// TestUnknownCommand verifies that unsupported tools are rejected.
func (*serverSuite) TestUnknownCommand(c *tc.C) {
	ctx := &testContext{}
	handler := NewHandler(ctx)
	var response Response
	err := handler.Main(Request{ContextID: ctx.ID(), Dir: "/", CommandName: "unknown"}, &response)
	c.Check(err, tc.ErrorMatches, `unsupported hook tool "unknown"`)
}

// TestCloseDisconnectsIdleClients verifies prompt, repeatable server shutdown with an idle RPC client.
func (*serverSuite) TestCloseDisconnectsIdleClients(c *tc.C) {
	server, err := NewServer(filepath.Join(c.MkDir(), "hookd.sock"), &testContext{})
	c.Assert(err, tc.IsNil)

	defer server.Close()
	conn, err := net.Dial("unix", server.listener.Addr().String())
	c.Assert(err, tc.IsNil)

	defer conn.Close()
	server.Close()
	server.Close()
	c.Assert(conn.SetReadDeadline(time.Now().Add(time.Second)), tc.IsNil)

	var data [1]byte
	_, err = conn.Read(data[:])
	c.Assert(err, tc.NotNil)

	if networkErr, ok := err.(net.Error); ok {
		c.Check(networkErr.Timeout(), tc.Equals, false)
	}
}

// TestInvalidRequestsCannotMutateContext checks identity and directory validation happen before command execution.
func (*serverSuite) TestInvalidRequestsCannotMutateContext(c *tc.C) {
	ctx := &testContext{status: charmRuntime.Status{Status: "maintenance", Message: "unchanged"}}
	handler := NewHandler(ctx)
	before := ctx.status
	for _, request := range []Request{
		{ContextID: "wrong-context", Dir: "/", CommandName: "status-set", Args: []string{"active"}},
		{ContextID: ctx.ID(), Dir: "relative", CommandName: "status-set", Args: []string{"active"}},
	} {
		var response Response
		c.Check(handler.Main(request, &response), tc.NotNil)
		c.Check(ctx.status, tc.Equals, before)
		c.Check(response.Stdout, tc.HasLen, 0)
	}
}
