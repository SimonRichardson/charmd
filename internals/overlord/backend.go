package overlord

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// fileBackend implements state.Backend using Charmd-style write/sync/rename
// replacement, so an interrupted checkpoint leaves the prior file intact.
type fileBackend struct {
	path   string
	ensure func(time.Duration)
}

func (b *fileBackend) Checkpoint(data []byte) error {
	dir := filepath.Dir(b.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, b.path); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}
	d, err := os.Open(dir)
	if err == nil {
		defer d.Close()
		if err = d.Sync(); err != nil {
			return err
		}
	}
	return nil
}

func (b *fileBackend) EnsureBefore(d time.Duration) { b.ensure(d) }

func (b *fileBackend) NeedsCheckpoint() bool { return true }
