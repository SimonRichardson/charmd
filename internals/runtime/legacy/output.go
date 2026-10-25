package legacy

import (
	"io"
	"sync"
)

// Hook output and tool logging can arrive concurrently.
type lockedWriter struct {
	mu     *sync.Mutex
	writer io.Writer
}

// Write serializes process output and hook-tool logging to a shared destination.
func (w lockedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(data)
}
