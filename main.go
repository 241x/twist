package main

import (
	"os"

	"github.com/241x/twist/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(cmd.GetExitCode(err))
	}
}
