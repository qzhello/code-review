package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/qzhello/code-review/internal/config"
	"github.com/qzhello/code-review/internal/model"
	"github.com/qzhello/code-review/internal/output"
	"github.com/qzhello/code-review/internal/rules"
)

var rulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "Manage review rules",
	Long: `List, validate, import, export, enable, or disable review rules.

Supports both YAML and JSON rule files.`,
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

		output.Hint(
			"Run "+output.HintCmd("cr rules import <file>")+" to add rules from a YAML/JSON file.",
			"Run "+output.HintCmd("cr rules export --format json")+" to share your rules.",
			"Run "+output.HintCmd("cr review --staged --mode rules-only")+" to test rules against staged changes.",
		)
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
	Short: "Validate rule files (YAML or JSON)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ".cr/rules/"
		if len(args) > 0 {
			path = args[0]
		}

		green := color.New(color.FgGreen)

		entries := collectRuleFiles(path)
		if len(entries) == 0 {
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

var rulesImportCmd = &cobra.Command{
	Use:   "import <file> [target-dir]",
	Short: "Import rules from a YAML or JSON file into the project",
	Long: `Import rules from an external file into the project's rules directory.

Supports YAML (.yaml, .yml) and JSON (.json) files.
Rules are validated before import. Duplicate rule IDs are reported.

Examples:
  cr rules import team-rules.yaml            # import into .cr/rules/
  cr rules import security.json .cr/rules/   # import JSON rules
  cr rules import ~/shared/rules.yaml        # import from another location`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runRulesImport,
}

var rulesExportCmd = &cobra.Command{
	Use:   "export [--format json|yaml]",
	Short: "Export all active rules to stdout",
	Long: `Export all active rules (builtin + custom) to stdout in YAML or JSON format.
Useful for sharing rules with teammates or backing up configurations.

Examples:
  cr rules export                    # export as YAML (default)
  cr rules export --format json      # export as JSON
  cr rules export --format json > rules.json  # save to file`,
	RunE: runRulesExport,
}

var exportFormat string

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
	rulesExportCmd.Flags().StringVar(&exportFormat, "format", "yaml", "output format: yaml or json")

	rulesCmd.AddCommand(rulesListCmd)
	rulesCmd.AddCommand(rulesValidateCmd)
	rulesCmd.AddCommand(rulesImportCmd)
	rulesCmd.AddCommand(rulesExportCmd)
	rulesCmd.AddCommand(rulesDisableCmd)
	rulesCmd.AddCommand(rulesEnableCmd)
}

func runRulesImport(cmd *cobra.Command, args []string) error {
	srcPath := args[0]
	targetDir := ".cr/rules/"
	if len(args) > 1 {
		targetDir = args[1]
	}

	// Validate source file first
	if err := rules.ValidateFile(srcPath); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Load source rules to check for duplicates
	srcRules, err := rules.LoadFromFile(srcPath, map[string]bool{})
	if err != nil {
		return err
	}

	// Load existing rules to detect conflicts
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	existingRules, _ := rules.LoadFromPaths(cfg.Rules.Paths, nil)
	existingIDs := make(map[string]string) // id -> source
	for _, r := range existingRules {
		existingIDs[r.ID] = r.Source
	}

	// Report conflicts
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)
	conflicts := 0
	for _, r := range srcRules {
		if src, exists := existingIDs[r.ID]; exists {
			yellow.Printf("  ! Rule %q already exists in %s (will be overridden by import)\n", r.ID, filepath.Base(src))
			conflicts++
		}
	}

	// Copy file to target directory
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	destPath := filepath.Join(targetDir, filepath.Base(srcPath))
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", destPath, err)
	}

	green.Printf("  Imported %d rules from %s → %s\n", len(srcRules), srcPath, destPath)
	if conflicts > 0 {
		yellow.Printf("  %d rule ID conflicts detected (imported rules take precedence by load order)\n", conflicts)
	}

	output.Hint(
		"Run "+output.HintCmd("cr rules list")+" to see all active rules.",
		"Run "+output.HintCmd("cr rules validate")+" to check rule syntax.",
		"Run "+output.HintCmd("cr review --staged --mode rules-only")+" to test the imported rules.",
	)
	return nil
}

func runRulesExport(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}

	disabledSet := make(map[string]bool)
	for _, id := range cfg.Rules.Disabled {
		disabledSet[id] = true
	}

	var allRules []model.Rule

	if cfg.Rules.Builtin {
		builtinRules, err := rules.LoadBuiltin(disabledSet)
		if err != nil {
			return err
		}
		allRules = append(allRules, builtinRules...)
	}

	customRules, err := rules.LoadFromPaths(cfg.Rules.Paths, cfg.Rules.Disabled)
	if err != nil {
		return err
	}
	allRules = append(allRules, customRules...)

	var data []byte
	switch exportFormat {
	case "json":
		data, err = rules.ExportToJSON(allRules)
	default:
		data, err = rules.ExportToYAML(allRules)
	}
	if err != nil {
		return fmt.Errorf("failed to export: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// collectRuleFiles finds all .yaml, .yml, and .json files in a directory.
func collectRuleFiles(path string) []string {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return nil
	}

	var entries []string
	for _, pattern := range []string{"*.yaml", "*.yml", "*.json"} {
		matches, _ := filepath.Glob(filepath.Join(path, pattern))
		entries = append(entries, matches...)
	}
	return entries
}
