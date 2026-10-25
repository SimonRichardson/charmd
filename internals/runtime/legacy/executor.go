package legacy

import (
	"context"
	"io"
	"os/exec"
	"time"
)

// Command describes the process the runner needs to execute.
type Command struct {
	Path   string
	Dir    string
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
}

// Executor separates local process management from hook preparation.
type Executor interface {
	// Execute runs a prepared command until completion or cancellation.
	Execute(context.Context, Command) error
}

type processExecutor struct{}

// Execute starts a local hook process and waits for completion or context cancellation.
func (processExecutor) Execute(ctx context.Context, command Command) error {
	cmd := exec.CommandContext(ctx, command.Path)
	cmd.Dir = command.Dir
	cmd.Env = command.Env
	cmd.Stdout, cmd.Stderr = command.Stdout, command.Stderr
	configureProcess(cmd)
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
