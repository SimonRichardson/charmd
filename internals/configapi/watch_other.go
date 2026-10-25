//go:build !linux

package configapi

import (
	"context"
	"fmt"
)

// watchFile reports the unsupported inotify backend on non-Linux platforms.
func watchFile(context.Context, string, func() error) error {
	return fmt.Errorf("local config API watching requires Linux inotify")
}
