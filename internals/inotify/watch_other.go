//go:build !linux

package inotify

import (
	"context"
	"fmt"
)

// WatchFile reports the unsupported inotify backend on non-Linux platforms.
func WatchFile(context.Context, string, func() error) error {
	return fmt.Errorf("file watching requires Linux inotify")
}
