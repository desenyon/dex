package main

import (
	"fmt"
	"os"

	"github.com/desenyon/dex/internal/cli"
)

var (
	version = "dev"
	commit  = "local"
)

func main() {
	if err := cli.NewRoot(os.Stdout, os.Stderr, cli.BuildInfo{Version: version, Commit: commit}).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
