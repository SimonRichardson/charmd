package legacy

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/SimonRichardson/charmd/internals/runtime/legacy/hookd"
)

// session owns temporary hook tools and the hookd server. It is created only
// after discovery finds a handler and closed before RunHook returns.
type session struct {
	paths  Paths
	server *hookd.Server
}

// newSession prepares temporary tool symlinks and starts hookd for one hook context.
func newSession(executable, dir string, ctx hookd.Context) (*session, error) {
	if executable == "" {
		return nil, fmt.Errorf("hook tool executable is required")
	}
	executable, err := filepath.Abs(executable)
	if err != nil {
		return nil, err
	}
	// Short paths keep Unix sockets usable even with long daemon state paths.
	work, err := os.MkdirTemp("", "charmd-hook-")
	if err != nil {
		return nil, err
	}
	s := &session{paths: Paths{CharmDir: dir, ToolsDir: work, Socket: filepath.Join(work, "hookd.sock")}}
	for _, name := range hookd.ToolNames() {
		if err := os.Symlink(executable, filepath.Join(work, name)); err != nil {
			s.Close()
			return nil, err
		}
	}
	s.server, err = hookd.NewServer(s.paths.Socket, ctx)
	if err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// Close shuts down hookd before removing the invocation socket and tool directory.
func (s *session) Close() {
	if s.server != nil {
		s.server.Close()
	}
	os.RemoveAll(s.paths.ToolsDir)
}
