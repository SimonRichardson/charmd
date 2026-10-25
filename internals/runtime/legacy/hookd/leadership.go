package hookd

import "fmt"

// newIsLeader exposes the fixed invocation snapshot using Juju output formats.
func newIsLeader(ctx Context) Command {
	return commandFunc(func(args []string) ([]byte, error) {
		format, positional, err := outputArgs(args)
		if err != nil {
			return nil, err
		}
		if len(positional) != 0 {
			return nil, fmt.Errorf("is-leader accepts no arguments")
		}
		return formatOutput(format, ctx.IsLeader())
	})
}
