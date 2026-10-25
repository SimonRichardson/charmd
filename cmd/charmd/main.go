package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/SimonRichardson/charmd/internals/cli"
	"github.com/SimonRichardson/charmd/internals/runtime/legacy/hookd"
)

// main dispatches hook-tool invocations before entering the regular CLI or daemon.
func main() {
	if name := filepath.Base(os.Args[0]); hookd.IsTool(name) {
		os.Exit(hookd.RunTool(name, os.Args[1:], os.Stdout, os.Stderr))
	}
	if err := cli.Run(nil); err != nil {
		fmt.Fprintf(cli.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
