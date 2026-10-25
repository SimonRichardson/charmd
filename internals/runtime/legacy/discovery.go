package legacy

import (
	"fmt"
	"os"
	"path/filepath"
)

// discoverHook prefers dispatch to a hook-specific executable. An empty path
// means the charm does not handle this hook.
func discoverHook(dir, name string) (string, error) {
	for _, candidate := range []string{"dispatch", "hooks/" + name} {
		path := filepath.Join(dir, candidate)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("inspect hook: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return "", fmt.Errorf("hook handler %q is not executable", candidate)
		}
		return path, nil
	}
	return "", nil
}
