package cli

import (
	"fmt"

	"github.com/canonical/go-flags"

	"github.com/SimonRichardson/charmd/client"
)

const cmdInstallSummary = "Install a charm"
const cmdInstallDescription = `
The install command creates a change that fetches and installs a charm.

Use --revision and --channel to select a Charmhub archive. SHA-256 selection is
available only through the HTTP API.
`

type cmdInstall struct {
	client *client.Client

	Revision *int   `long:"revision" value-name:"REVISION"`
	Channel  string `long:"channel" value-name:"CHANNEL"`

	Positional struct {
		Name string `positional-arg-name:"<charm-name>" required:"1"`
	} `positional-args:"yes"`
}

func init() {
	AddCommand(&CmdInfo{
		Name:        "install",
		Summary:     cmdInstallSummary,
		Description: cmdInstallDescription,
		ArgsHelp: map[string]string{
			"--revision": "Select a Charmhub revision",
			"--channel":  "Select a Charmhub channel",
		},
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
	change, err := cmd.client.InstallCharm(client.InstallCharmOptions{
		Name:     cmd.Positional.Name,
		Revision: cmd.Revision,
		Channel:  cmd.Channel,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(Stdout, "Change %s created.\n", change)
	return err
}
