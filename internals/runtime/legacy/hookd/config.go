package hookd

import "fmt"

// newConfigGet binds configuration lookup and output formatting to the hook context.
func newConfigGet(ctx Context) Command {
	return commandFunc(func(args []string) ([]byte, error) {
		format, positional, err := outputArgs(args)
		if err != nil {
			return nil, err
		}
		if len(positional) > 1 {
			return nil, fmt.Errorf("config-get accepts at most one key")
		}
		var value any = ctx.Config()
		if len(positional) == 1 && positional[0] != "-" {
			value = ctx.Config()[positional[0]]
		}
		return formatOutput(format, value)
	})
}
