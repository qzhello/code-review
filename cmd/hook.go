package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/qzhello/code-review/internal/git"
	"github.com/qzhello/code-review/internal/hooks"
)

var defaultHookTypes = []string{"pre-commit", "pre-push"}

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage git hooks for automatic code review",
	Long:  `Install or uninstall git hooks that run cr review automatically.`,
}

var hookInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install git hooks (pre-commit + pre-push)",
	Long: `Installs git hooks that automatically run cr review:
  - pre-commit: runs rules-only review on staged changes (blocks on errors)
  - pre-push: runs review on commits being pushed (blocks on warnings+errors)

Existing hooks are backed up before overwriting.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		repoRoot, err := git.GetRepoRoot(ctx)
		if err != nil {
			return fmt.Errorf("not in a git repository")
		}

		hooksDir := filepath.Join(repoRoot, ".git", "hooks")

		bold := color.New(color.Bold)
		bold.Println("Installing cr git hooks...")

		if err := hooks.Install(hooksDir, defaultHookTypes); err != nil {
			return err
		}

		green := color.New(color.FgGreen)
		green.Println("\nHooks installed successfully!")
		fmt.Println("  pre-commit: blocks on errors (rules-only, fast)")
		fmt.Println("  pre-push:   blocks on warnings and errors")
		fmt.Println("\n  Use 'git commit --no-verify' to bypass.")

		return nil
	},
}

var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove cr git hooks",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		repoRoot, err := git.GetRepoRoot(ctx)
		if err != nil {
			return fmt.Errorf("not in a git repository")
		}

		hooksDir := filepath.Join(repoRoot, ".git", "hooks")

		bold := color.New(color.Bold)
		bold.Println("Removing cr git hooks...")

		if err := hooks.Uninstall(hooksDir, defaultHookTypes); err != nil {
			return err
		}

		fmt.Println("\nDone.")
		return nil
	},
}

var hookStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show installed hook status",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		repoRoot, err := git.GetRepoRoot(ctx)
		if err != nil {
			return fmt.Errorf("not in a git repository")
		}

		hooksDir := filepath.Join(repoRoot, ".git", "hooks")
		green := color.New(color.FgGreen)
		red := color.New(color.FgRed)

		for _, hookType := range defaultHookTypes {
			hookPath := filepath.Join(hooksDir, hookType)
			if data, err := os.ReadFile(hookPath); err == nil {
				if hooks.IsOurHook(string(data)) {
					green.Printf("  ✓ %s: installed\n", hookType)
				} else {
					fmt.Printf("  ~ %s: exists (not managed by cr)\n", hookType)
				}
			} else {
				red.Printf("  ✗ %s: not installed\n", hookType)
			}
		}

		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
	hookCmd.AddCommand(hookStatusCmd)
}
