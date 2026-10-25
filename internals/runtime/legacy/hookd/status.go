package hookd

import (
	"fmt"
	"strings"

	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

// newStatusGet binds workload status lookup to the hook context.
func newStatusGet(ctx Context) Command {
	return commandFunc(func(args []string) ([]byte, error) {
		format, positional, err := outputArgs(args)
		if err != nil {
			return nil, err
		}
		if len(positional) != 0 {
			return nil, fmt.Errorf("status-get accepts no arguments")
		}
		return formatOutput(format, ctx.Status().Status)
	})
}

// newStatusSet validates workload statuses before updating the hook context.
func newStatusSet(ctx Context) Command {
	return commandFunc(func(args []string) ([]byte, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("status-set requires a status")
		}
		switch args[0] {
		case "maintenance", "blocked", "waiting", "active":
		default:
			return nil, fmt.Errorf("invalid workload status %q", args[0])
		}
		ctx.SetStatus(charmRuntime.Status{Status: args[0], Message: strings.Join(args[1:], " ")})
		return nil, nil
	})
}
