package main

import (
	"os"

	"fmt"
	"github.com/SimonRichardson/charmd/internals/cli"
)

func main() {
	if err := cli.Run(nil); err != nil {
		fmt.Fprintf(cli.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
