package legacy

import (
	"os"
	"strings"
)

// Paths identifies the per-invocation tool and socket locations.
type Paths struct {
	CharmDir string
	ToolsDir string
	Socket   string
}

// Environment supplies the Juju variables exposed to a charm hook.
func (c *HookContext) Environment(paths Paths, hook string) []string {
	return hookEnvironment(paths.ToolsDir, paths.CharmDir, paths.Socket, c.ID(), c.name, hook)
}

// hookEnvironment removes inherited hook variables and adds the current invocation environment.
func hookEnvironment(tools, dir, socket, id, name, hook string) []string {
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key == "PATH" || key == "CHARM_DIR" || strings.HasPrefix(key, "JUJU_") {
			continue
		}
		env = append(env, entry)
	}
	path := os.Getenv("PATH")
	if path == "" {
		path = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	return append(env, "PATH="+tools+string(os.PathListSeparator)+path,
		"CHARM_DIR="+dir, "JUJU_CHARM_DIR="+dir, "JUJU_CONTEXT_ID="+id,
		"JUJU_HOOK_NAME="+hook, "JUJU_DISPATCH_PATH=hooks/"+hook,
		"JUJU_AGENT_SOCKET_NETWORK=unix", "JUJU_AGENT_SOCKET_ADDRESS="+socket,
		"JUJU_UNIT_NAME="+name+"/0", "JUJU_MODEL_NAME=charmd")
}
