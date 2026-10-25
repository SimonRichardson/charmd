//go:build linux

package inotify

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// WatchFile watches the parent directory so atomic rename replacements keep
// working. The watch is established before the initial read to avoid losing a write.
func WatchFile(ctx context.Context, path string, changed func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return fmt.Errorf("initialize inotify: %w", err)
	}
	file := os.NewFile(uintptr(fd), "file-inotify")
	defer file.Close()
	mask := uint32(unix.IN_CLOSE_WRITE | unix.IN_MOVED_TO | unix.IN_DELETE_SELF | unix.IN_MOVE_SELF)
	if _, err := unix.InotifyAddWatch(fd, filepath.Dir(path), mask); err != nil {
		return fmt.Errorf("watch parent directory: %w", err)
	}
	// Closing the descriptor interrupts an idle pollable read on cancellation.
	done := make(chan struct{})
	watcherDone := make(chan struct{})
	defer func() { close(done); <-watcherDone }()
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			file.Close()
		case <-done:
		}
	}()
	if err := changed(); err != nil {
		return err
	}
	buffer := make([]byte, 64*1024)
	for {
		n, err := file.Read(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		for offset := 0; offset+unix.SizeofInotifyEvent <= n; {
			event := buffer[offset:]
			flags := binary.NativeEndian.Uint32(event[4:8])
			length := int(binary.NativeEndian.Uint32(event[12:16]))
			end := offset + unix.SizeofInotifyEvent + length
			if end > n {
				return fmt.Errorf("truncated inotify event")
			}
			name := strings.TrimRight(string(buffer[offset+unix.SizeofInotifyEvent:end]), "\x00")
			if flags&(unix.IN_DELETE_SELF|unix.IN_MOVE_SELF|unix.IN_IGNORED) != 0 {
				return fmt.Errorf("parent directory watch was removed")
			}
			// Overflow requires a full read even when no filename is available.
			if flags&unix.IN_Q_OVERFLOW != 0 || name == filepath.Base(path) {
				if err := changed(); err != nil {
					return err
				}
			}
			offset = end
		}
	}
}
