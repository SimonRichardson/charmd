package legacy

import (
	"os/exec"
	"syscall"
)

// configureProcess configures platform-specific cancellation for the hook process.
func configureProcess(cmd *exec.Cmd) {
	// Hooks commonly spawn shell children; cancel the whole group so they
	// cannot retain output pipes or keep running after daemon shutdown.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
