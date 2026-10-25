package hookd

import (
	"fmt"
	"slices"

	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

// Context supplies the agent operations available to hook commands. Transport
// and commands do not need to know how a charm or its process is managed.
type Context interface {
	// ID identifies the invocation authorized to use this context.
	ID() string
	// Config exposes read-only charm configuration for this invocation.
	Config() map[string]any
	// IsLeader returns the immutable invocation leadership snapshot.
	IsLeader() bool
	// Status returns the current workload status.
	Status() charmRuntime.Status
	// SetStatus replaces the invocation workload status.
	SetStatus(charmRuntime.Status)
	// Log records a message in the hook log.
	Log(string) error
}

// Command executes a hook tool against an invocation context.
type Command interface {
	// Run validates command arguments and returns its standard output or an error.
	Run(args []string) ([]byte, error)
}

type commandFunc func([]string) ([]byte, error)

// Run adapts a command function to the Command interface.
func (f commandFunc) Run(args []string) ([]byte, error) { return f(args) }

type commandFactory func(Context) Command

var commands = map[string]commandFactory{
	"config-get": newConfigGet,
	"is-leader":  newIsLeader,
	"juju-log":   newLog,
	"status-get": newStatusGet,
	"status-set": newStatusSet,
}

// NewCommand binds a named hook tool to its invocation context.
func NewCommand(ctx Context, name string) (Command, error) {
	factory, ok := commands[name]
	if !ok {
		return nil, fmt.Errorf("unsupported hook tool %q", name)
	}
	return factory(ctx), nil
}

// ToolNames returns the commands supported by the server and client.
func ToolNames() []string {
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// IsTool reports whether a name belongs to a registered hook-tool command.
func IsTool(name string) bool { _, ok := commands[name]; return ok }
