package hookd

import (
	"fmt"
	"strconv"
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
		var err error
		args, err = unitStatusArgs(args)
		if err != nil {
			return nil, err
		}
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

// unitStatusArgs accepts the explicit false application flag used by Ops for
// unit status. Application status requires leadership support and is rejected.
func unitStatusArgs(args []string) ([]string, error) {
	for len(args) > 0 {
		if args[0] == "--" {
			return args[1:], nil
		}
		if args[0] == "--application" {
			return nil, fmt.Errorf("application status is not supported")
		}
		value, ok := strings.CutPrefix(args[0], "--application=")
		if !ok {
			break
		}
		application, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("invalid application flag %q", value)
		}
		if application {
			return nil, fmt.Errorf("application status is not supported")
		}
		args = args[1:]
	}
	return args, nil
}
