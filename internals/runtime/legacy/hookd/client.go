// Package hookd supplies hook-tool clients, commands, and an invocation-scoped
// RPC server. It depends on a hook context rather than the legacy runner.
package hookd

import (
	"fmt"
	"io"
	"net/rpc"
	"os"
)

// RunTool forwards a tool invocation to the active hook context.
func RunTool(name string, args []string, stdout, stderr io.Writer) int {
	fail := func(err error) int { fmt.Fprintln(stderr, err); return 1 }
	if !IsTool(name) {
		return fail(fmt.Errorf("unsupported hook tool %q", name))
	}
	if os.Getenv("JUJU_AGENT_SOCKET_NETWORK") != "unix" {
		return fail(fmt.Errorf("hook tool requires a Unix hook context"))
	}
	id := os.Getenv("JUJU_CONTEXT_ID")
	if id == "" {
		return fail(fmt.Errorf("hook tool requires JUJU_CONTEXT_ID"))
	}
	dir, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	client, err := rpc.Dial("unix", os.Getenv("JUJU_AGENT_SOCKET_ADDRESS"))
	if err != nil {
		return fail(err)
	}
	defer client.Close()
	var response Response
	err = client.Call("Hookd.Main", Request{ContextID: id, Dir: dir, CommandName: name, Args: args}, &response)
	if err != nil {
		return fail(err)
	}
	if _, err := stdout.Write(response.Stdout); err != nil {
		return fail(err)
	}
	if _, err := stderr.Write(response.Stderr); err != nil {
		return 1
	}
	return response.Code
}
