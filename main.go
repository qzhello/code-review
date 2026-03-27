package main

import (
	"os"

	"github.com/qzhello/code-review/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
