package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/quzhihao/code-review/internal/config"
	"github.com/quzhihao/code-review/internal/rules"
)

var rulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "Manage review rules",
	Long:  `List, validate, enable, or disable review rules.`,
}

var rulesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active rules",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}

		disabledSet := make(map[string]bool)
		for _, id := range cfg.Rules.Disabled {
			disabledSet[id] = true
		}

		bold := color.New(color.Bold)
		green := color.New(color.FgGreen)
		red := color.New(color.FgRed)
		yellow := color.New(color.FgYellow)
		cyan := color.New(color.FgCyan)

		// Built-in rules
		if cfg.Rules.Builtin {
			builtinRules, err := rules.LoadBuiltin(disabledSet)
			if err != nil {
				return err
			}
			bold.Println("Built-in rules:")
			for _, r := range builtinRules {
				printRule(r.ID, r.Severity, r.Description, r.Enabled, r.Source, green, red, yellow, cyan)
			}
			fmt.Println()
		}

		// Custom rules
		customRules, err := rules.LoadFromPaths(cfg.Rules.Paths, cfg.Rules.Disabled)
		if err != nil {
			return err
		}
		if len(customRules) > 0 {
			bold.Println("Custom rules:")
			for _, r := range customRules {
				printRule(r.ID, r.Severity, r.Description, r.Enabled, r.Source, green, red, yellow, cyan)
			}
			fmt.Println()
		}

		return nil
	},
}

func printRule(id, severity, description string, enabled bool, source string, green, red, yellow, cyan *color.Color) {
	status := green.Sprint("✓")
	if !enabled {
		status = red.Sprint("✗")
	}

	sevColor := cyan
	switch severity {
	case "error":
		sevColor = red
	case "warn":
		sevColor = yellow
	}

	fmt.Printf("  %s %-25s ", status, id)
	sevColor.Printf("%-6s", severity)
	fmt.Printf("  %s", description)
	if source != "" {
		fmt.Printf("  (%s)", filepath.Base(source))
	}
	fmt.Println()
}

var rulesValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate rule YAML syntax",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ".cr/rules/"
		if len(args) > 0 {
			path = args[0]
		}

		green := color.New(color.FgGreen)

		entries, _ := filepath.Glob(filepath.Join(path, "*.yaml"))
		ymlEntries, _ := filepath.Glob(filepath.Join(path, "*.yml"))
		entries = append(entries, ymlEntries...)

		if len(entries) == 0 {
			// Maybe it's a single file
			entries = []string{path}
		}

		hasError := false
		for _, entry := range entries {
			if err := rules.ValidateFile(entry); err != nil {
				fmt.Printf("  ✗ %s: %s\n", entry, err)
				hasError = true
			} else {
				green.Printf("  ✓ %s\n", entry)
			}
		}

		if hasError {
			return fmt.Errorf("validation failed")
		}
		fmt.Println("\nAll rules valid.")
		return nil
	},
}

var rulesDisableCmd = &cobra.Command{
	Use:   "disable [rule-id]",
	Short: "Disable a rule",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("To disable rule %q, add it to 'rules.disabled' in .cr/config.yaml:\n\n", args[0])
		fmt.Printf("  rules:\n    disabled:\n      - %s\n", args[0])
		return nil
	},
}

var rulesEnableCmd = &cobra.Command{
	Use:   "enable [rule-id]",
	Short: "Enable a rule",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("To enable rule %q, remove it from 'rules.disabled' in .cr/config.yaml\n", args[0])
		return nil
	},
}

func init() {
	rulesCmd.AddCommand(rulesListCmd)
	rulesCmd.AddCommand(rulesValidateCmd)
	rulesCmd.AddCommand(rulesDisableCmd)
	rulesCmd.AddCommand(rulesEnableCmd)
}
