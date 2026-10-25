package hookd

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"strings"
)

// formatOutput renders hook-tool values as smart text, JSON, or YAML.
func formatOutput(format string, value any) ([]byte, error) {
	switch format {
	case "json":
		data, err := json.Marshal(value)
		return append(data, '\n'), err
	case "yaml":
		return yaml.Marshal(value)
	case "smart":
		if value == nil {
			return nil, nil
		}
		switch value.(type) {
		case map[string]any:
			return yaml.Marshal(value)
		}
		return []byte(fmt.Sprintln(value)), nil
	default:
		return nil, fmt.Errorf("unsupported output format %q", format)
	}
}

// outputArgs separates output-format flags from positional arguments.
func outputArgs(args []string) (string, []string, error) {
	format := "smart"
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return format, append(positional, args[i+1:]...), nil
		case arg == "--format" || arg == "-f":
			i++
			if i == len(args) {
				return "", nil, fmt.Errorf("missing output format")
			}
			format = args[i]
		case strings.HasPrefix(arg, "--format="):
			format = strings.TrimPrefix(arg, "--format=")
		case strings.HasPrefix(arg, "-") && arg != "-":
			return "", nil, fmt.Errorf("unsupported option %q", arg)
		default:
			positional = append(positional, arg)
		}
	}
	return format, positional, nil
}
