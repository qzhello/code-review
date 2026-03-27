package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/qzhello/code-review/internal/config"
	"github.com/qzhello/code-review/internal/output"
)

var forceInit bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize cr config in current project",
	Long:  `Creates a .cr/ directory with default config and rule files.`,
	RunE:  runInit,
}

func init() {
	initCmd.Flags().BoolVar(&forceInit, "force", false, "overwrite existing config")
}

func runInit(cmd *cobra.Command, args []string) error {
	dirs := []string{
		".cr",
		".cr/rules",
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", d, err)
		}
	}

	files := map[string]string{
		".cr/config.yaml":       config.DefaultConfigYAML,
		".cr/rules/custom.yaml": defaultCustomRules,
	}

	green := color.New(color.FgGreen)

	for path, content := range files {
		if !forceInit {
			if _, err := os.Stat(path); err == nil {
				fmt.Printf("  skip %s (already exists, use --force to overwrite)\n", path)
				continue
			}
		}
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
		green.Printf("  created %s\n", path)
	}

	fmt.Println("\ncr initialized. Edit .cr/config.yaml to customize.")

	output.Hint(
		"Run "+output.HintCmd("cr review --staged")+" to review your staged changes.",
		"Run "+output.HintCmd("cr rules list")+" to see all available rules.",
		"Run "+output.HintCmd("cr hook install")+" to auto-review on every commit.",
		"Edit "+output.HintCmd(".cr/config.yaml")+" to configure the AI agent, rules, and more.",
	)
	return nil
}

const defaultCustomRules = `# Custom review rules
# See: cr rules list (for built-in rules)

rules:
  - id: no-debug-print
    severity: warn
    description: "Debug print statements should be removed before merge"
    file: "*.go"
    pattern: 'fmt\.Print(ln|f)?\('
    scope: added

  - id: no-todo-without-issue
    severity: info
    description: "TODO/FIXME should reference an issue number"
    pattern: '(TODO|FIXME|HACK)\b'
    scope: added
`
