//go:build !linux

package legacy

import "os/exec"

// configureProcess configures platform-specific cancellation for the hook process.
func configureProcess(cmd *exec.Cmd) {}
