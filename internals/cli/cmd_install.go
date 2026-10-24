package cli

import (
	"encoding/hex"
	"fmt"

	"github.com/canonical/go-flags"

	"github.com/SimonRichardson/charmd/client"
)

const cmdInstallSummary = "Install a charm"
const cmdInstallDescription = `
The install command creates a change that fetches and installs a SHA-addressed
charm archive.
`

type cmdInstall struct {
	client *client.Client

	Positional struct {
		SHA256 string `positional-arg-name:"<sha256>" required:"1"`
	} `positional-args:"yes"`
}

func init() {
	AddCommand(&CmdInfo{
		Name:        "install",
		Summary:     cmdInstallSummary,
		Description: cmdInstallDescription,
		New: func(opts *CmdOptions) flags.Commander {
			return &cmdInstall{client: opts.Client}
		},
	})
}

// Execute creates the asynchronous charm installation change.
func (cmd *cmdInstall) Execute(args []string) error {
	if len(args) > 0 {
		return ErrExtraArgs
	}
	if len(cmd.Positional.SHA256) != 64 {
		return fmt.Errorf("invalid SHA-256 checksum %q", cmd.Positional.SHA256)
	}
	if _, err := hex.DecodeString(cmd.Positional.SHA256); err != nil {
		return fmt.Errorf("invalid SHA-256 checksum %q", cmd.Positional.SHA256)
	}
	change, err := cmd.client.InstallCharm(client.InstallCharmOptions{
		SHA256: cmd.Positional.SHA256,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(Stdout, "Change %s created.\n", change)
	return err
}
