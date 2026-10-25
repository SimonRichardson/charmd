package hookd

import (
	"fmt"
)

// newConfigGet exposes the invocation config through Juju-compatible key and
// whole-config queries. Unset options are omitted unless --all is requested.
func newConfigGet(ctx Context) Command {
	return commandFunc(func(args []string) ([]byte, error) {
		all, filtered := configArgs(args)
		format, positional, err := outputArgs(filtered)
		if err != nil {
			return nil, err
		}
		if len(positional) > 1 {
			return nil, fmt.Errorf("config-get accepts at most one key")
		}
		if all && len(positional) != 0 {
			return nil, fmt.Errorf("config-get cannot use --all with a key")
		}
		settings := ctx.Config()
		var value any
		if len(positional) == 1 && positional[0] != "-" {
			value = settings[positional[0]]
		} else {
			// Filtering must not mutate the invocation's config: subsequent keyed or
			// --all queries must still be able to observe defined but unset options.
			values := make(map[string]any, len(settings))
			for name, v := range settings {
				if all || v != nil {
					values[name] = v
				}
			}
			value = values
		}
		return formatOutput(format, value)
	})
}

// configArgs extracts config-get's boolean flags while preserving format values
// and the -- delimiter used to query option names that begin with a dash.
func configArgs(args []string) (bool, []string) {
	all := false
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--":
			return all, append(filtered, args[i:]...)
		case "--all", "-a":
			all = true
		case "--format", "-f":
			filtered = append(filtered, args[i])
			if i+1 < len(args) {
				i++
				filtered = append(filtered, args[i])
			}
		default:
			filtered = append(filtered, args[i])
		}
	}
	return all, filtered
}
