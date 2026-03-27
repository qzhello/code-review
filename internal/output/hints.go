package output

import (
	"fmt"

	"github.com/fatih/color"
)

var hintStyle = color.New(color.FgWhite, color.Faint)
var hintHeaderStyle = color.New(color.FgWhite, color.Faint, color.Bold)
var hintCmdStyle = color.New(color.FgCyan, color.Bold)

// Hint prints a "next steps" suggestion after a command.
func Hint(lines ...string) {
	if len(lines) == 0 {
		return
	}
	hintHeaderStyle.Print("Next steps:\n")
	for _, line := range lines {
		fmt.Print("  ")
		hintStyle.Println(line)
	}
	fmt.Println()
}

// HintCmd formats a command name for use in hints.
func HintCmd(cmd string) string {
	return hintCmdStyle.Sprint(cmd)
}
