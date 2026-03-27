package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/qzhello/code-review/internal/config"
	"github.com/qzhello/code-review/internal/git"
	"github.com/qzhello/code-review/internal/model"
	"github.com/qzhello/code-review/internal/output"
	"github.com/qzhello/code-review/internal/review"
	"github.com/qzhello/code-review/internal/store"
	"github.com/qzhello/code-review/internal/tui"
)

var (
	staged      bool
	branch      string
	commit      string
	mode        string
	minSeverity string
	jsonOutput  bool
	focus       string
	prdFile     string
	interactive bool
)

var reviewCmd = &cobra.Command{
	Use:   "review [paths...]",
	Short: "Review code changes",
	Long: `Review code changes using rules, AI agent, or both.

Target selection (pick one):
  cr review                        review all uncommitted changes
  cr review --staged               review staged changes only
  cr review --branch main          compare current branch to main
  cr review --commit HEAD~3        review last 3 commits
  cr review --commit v1.0..v2.0    review changes between tags/versions
  cr review --commit abc123        review a specific commit

Path scoping (combine with any target):
  cr review ./src/                 review changes in src/ only
  cr review --staged cmd/ internal/  review staged changes in cmd/ and internal/

PRD-aware review:
  cr review --prd docs/prd.md      review against product requirements
  cr review --staged --prd spec.md combine with other flags`,
	RunE: runReview,
}

func init() {
	reviewCmd.Flags().BoolVar(&staged, "staged", false, "review staged changes only")
	reviewCmd.Flags().StringVar(&branch, "branch", "", "compare current branch to this base branch")
	reviewCmd.Flags().StringVar(&commit, "commit", "", "commit range (e.g., HEAD~3, v1.0..v2.0, abc123)")
	reviewCmd.Flags().StringVar(&mode, "mode", "", "review mode: hybrid, rules-only, agent-only (overrides config)")
	reviewCmd.Flags().StringVar(&minSeverity, "min-severity", "", "minimum severity: info, warn, error (overrides config)")
	reviewCmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	reviewCmd.Flags().StringVar(&focus, "focus", "", "override agent focus area (e.g., security)")
	reviewCmd.Flags().StringVar(&prdFile, "prd", "", "path to PRD/requirements document for context-aware review")
	reviewCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "interactive TUI mode for reviewing findings")
}

func runReview(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	start := time.Now()

	// Load config
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply flag overrides
	reviewMode := cfg.Review.Mode
	if mode != "" {
		reviewMode = mode
	}
	if minSeverity != "" {
		cfg.Noise.MinSeverity = minSeverity
	}
	if focus != "" {
		cfg.Agent.Focus = []string{focus}
	}

	// Load PRD content if specified
	var prdContent string
	if prdFile != "" {
		data, err := os.ReadFile(prdFile)
		if err != nil {
			return fmt.Errorf("failed to read PRD file %s: %w", prdFile, err)
		}
		prdContent = string(data)
		if !jsonOutput {
			fmt.Printf("  PRD loaded: %s (%d bytes)\n", prdFile, len(data))
		}
	}

	// Step 1: Build diff options
	diffOpts := git.DiffOptions{
		Staged:       staged,
		Branch:       branch,
		Commit:       commit,
		ContextLines: cfg.Review.ContextLines,
		Paths:        args, // positional args are paths
	}

	raw, err := git.ExecDiff(ctx, diffOpts)
	if err != nil {
		return fmt.Errorf("failed to get diff: %w", err)
	}

	if raw == "" {
		fmt.Println("No changes to review.")
		return nil
	}

	// Step 2: Parse + filter diff
	diff := git.ParseDiff(raw)
	diff = git.FilterFiles(diff, cfg.Review.Include, cfg.Review.Exclude)

	if len(diff.Files) == 0 {
		fmt.Println("No files to review after filtering.")
		return nil
	}

	// Print diff summary
	term := output.NewTerminal(cfg.Output.ShowContext, cfg.Output.Verbose || verbose)
	if !jsonOutput {
		term.PrintDiffSummary(diff)
	}

	// Step 3: Run review engine
	engine := review.NewEngine(cfg, prdContent)
	result, err := engine.Run(ctx, diff, reviewMode)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}

	// Step 4: Output
	elapsed := time.Since(start)
	if interactive && len(result.Findings) > 0 {
		// Interactive TUI mode
		tuiModel := tui.NewModel(result, diff)
		p := tea.NewProgram(tuiModel, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("TUI error: %w", err)
		}

		// Print summary of user actions
		if fm, ok := finalModel.(tui.Model); ok {
			printTUIActions(fm.Findings())
		}
	} else if jsonOutput {
		output.PrintJSON(result, elapsed)
	} else {
		term.PrintResult(result, elapsed)
	}

	// Step 5: Save to history
	if cfg.Store.Enabled {
		saveToHistory(ctx, cfg, reviewMode, result)
	}

	// Exit code 1 if any errors
	for _, f := range result.Findings {
		if f.Severity == model.SeverityError {
			os.Exit(1)
		}
	}

	return nil
}

func saveToHistory(ctx context.Context, cfg *model.Config, reviewMode string, result *review.Result) {
	db, err := store.Open(cfg.Store.Path)
	if err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "Warning: failed to open store: %s\n", err)
		}
		return
	}
	defer db.Close()

	repo, _ := git.GetRepoRoot(ctx)
	branchName, _ := git.GetCurrentBranch(ctx)
	commitHash, _ := git.GetHeadCommit(ctx)

	_, err = db.SaveReview(repo, branchName, commitHash, reviewMode,
		result.Findings, result.Stats.Errors, result.Stats.Warnings, result.Stats.Infos)
	if err != nil && verbose {
		fmt.Fprintf(os.Stderr, "Warning: failed to save review: %s\n", err)
	}
}

func printTUIActions(items []tui.FindingItem) {
	accepted, dismissed, fixed, pending := 0, 0, 0, 0
	for _, item := range items {
		switch item.Action {
		case tui.ActionAccepted:
			accepted++
		case tui.ActionDismissed:
			dismissed++
		case tui.ActionFixed:
			fixed++
		default:
			pending++
		}
	}

	fmt.Printf("\nReview complete: %d accepted, %d dismissed, %d fixed, %d pending\n",
		accepted, dismissed, fixed, pending)
}
