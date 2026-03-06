package main

import (
	"os"

	"github.com/asakaida/dandori/cmd/dandori-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
