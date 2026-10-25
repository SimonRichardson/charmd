// Package cli implements Charmd-style client commands.
package cli

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/canonical/go-flags"

	"github.com/SimonRichardson/charmd/client"
)

const changesUsage = "usage: charmd changes\n       charmd tasks <change-id>\n" +
	"       charmd install <sha256>\n" +
	"       charmd notice|notices|okay\n"

// RunWithClient parses and executes the supported changes commands through api.
func RunWithClient(args []string, api *client.Client, out, errOut io.Writer) error {
	return RunWithSocket(args, api, "", out, errOut)
}

// RunWithSocket executes commands using socketPath for notice acknowledgement state.
func RunWithSocket(args []string, api *client.Client, socketPath string, out, errOut io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := io.WriteString(out, changesUsage)
		return err
	}

	switch args[0] {
	case "changes":
		return runChanges(args[1:], api, out, errOut)
	case "tasks":
		return runTasks(args[1:], api, out)
	case "install", "notice", "notices", "okay":
		previousOut, previousErr := Stdout, Stderr
		Stdout, Stderr = out, errOut
		defer func() { Stdout, Stderr = previousOut, previousErr }()
		command := commands[args[0]].New(&CmdOptions{Client: api, SocketPath: socketPath})
		parser := flags.NewParser(command, flags.None)
		rest, err := parser.ParseArgs(args[1:])
		if err != nil {
			return err
		}
		return command.Execute(rest)
	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], changesUsage)
	}
}

// runChanges lists changes selected by the command arguments.
func runChanges(args []string, api *client.Client, out, errOut io.Writer) error {
	if len(args) > 1 {
		return fmt.Errorf("changes takes at most one service name")
	}

	service := ""
	if len(args) == 1 {
		service = args[0]
	}
	if regexp.MustCompile(`^[0-9]+$`).MatchString(service) {
		return fmt.Errorf("'charmd changes' expects a service name, try 'charmd tasks %s'", service)
	}

	changes, err := api.Changes(&client.ChangesOptions{
		ServiceName: service,
		Selector:    client.ChangesAll,
	})
	if err != nil {
		return err
	}

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].SpawnTime.Before(changes[j].SpawnTime)
	})
	if len(changes) == 0 {
		_, err := fmt.Fprintln(errOut, "no changes found")
		return err
	}

	w := tabwriter.NewWriter(out, 0, 8, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "ID\tStatus\tSpawn\tReady\tSummary"); err != nil {
		return err
	}
	for _, change := range changes {
		if _, err := fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\n",
			change.ID,
			change.Status,
			formatTime(change.SpawnTime),
			formatReady(change.ReadyTime),
			change.Summary,
		); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out)
	return err
}

// runTasks displays the tasks belonging to the requested change.
func runTasks(args []string, api *client.Client, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("tasks requires a change ID")
	}

	change, err := api.Change(args[0])
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(out, 0, 8, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "Status\tSpawn\tReady\tSummary"); err != nil {
		return err
	}
	for _, task := range change.Tasks {
		if _, err := fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\n",
			task.Status,
			formatTime(task.SpawnTime),
			formatReady(task.ReadyTime),
			task.Summary,
		); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	// Hook tracebacks are stored in task logs, so surface them alongside failed
	// tasks rather than requiring direct access to the daemon's state file.
	for _, task := range change.Tasks {
		if task.Status != "Error" || len(task.Log) == 0 {
			continue
		}
		if _, err := fmt.Fprintf(out, "%s:\n", task.Summary); err != nil {
			return err
		}
		for _, line := range task.Log {
			if _, err := fmt.Fprintln(out, line); err != nil {
				return err
			}
		}
	}
	if change.Err != "" {
		_, err = fmt.Fprintf(out, "Error: %s\n", change.Err)
	}
	return err
}

// formatTime renders a timestamp for the command table.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

// formatReady renders a completion timestamp or the placeholder for unfinished work.
func formatReady(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return formatTime(t)
}
