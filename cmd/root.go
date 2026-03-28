package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	verbose bool
)

var rootCmd = &cobra.Command{
	Use:   "cr",
	Short: "Code Review CLI — hybrid rule + agent code review",
	Long: `cr is a terminal-based code review tool that combines
deterministic rule-based analysis with LLM-powered agent review.

It analyzes local git diffs, applies configurable rules, and
optionally sends code to an AI agent for deeper review.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: .cr/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	rootCmd.AddCommand(reviewCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(rulesCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(hookCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(dismissCmd)
	rootCmd.AddCommand(webCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("cr version 0.1.0")
	},
}

// exitWithError prints an error and exits.
func exitWithError(msg string) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
	os.Exit(1)
}
