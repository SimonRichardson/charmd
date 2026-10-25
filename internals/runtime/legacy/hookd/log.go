package hookd

import (
	"fmt"
	"strings"
)

// newLog binds legacy juju-log argument handling to the context log destination.
func newLog(ctx Context) Command {
	return commandFunc(func(args []string) ([]byte, error) {
		if len(args) > 0 && (args[0] == "-l" || args[0] == "--log-level") {
			if len(args) < 2 {
				return nil, fmt.Errorf("juju-log requires a log level")
			}
			args = args[2:]
		} else if len(args) > 0 && strings.HasPrefix(args[0], "--log-level=") {
			args = args[1:]
		}
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		if len(args) == 0 {
			return nil, fmt.Errorf("juju-log requires a message")
		}
		return nil, ctx.Log(strings.Join(args, " "))
	})
}
