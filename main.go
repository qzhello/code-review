package main

import (
	"os"

	"github.com/quzhihao/code-review/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
