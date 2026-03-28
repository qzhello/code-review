package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
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
	lang        string
	exportFile  string
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
	reviewCmd.Flags().StringVar(&lang, "lang", "", "output language for agent findings: en, zh, ja, ko, etc. (overrides config)")
	reviewCmd.Flags().StringVar(&exportFile, "export", "", "export findings to a file (supports .md)")
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
	if lang != "" {
		cfg.Agent.Language = lang
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

	// Step 3: Run review engine with progress
	engine := review.NewEngine(cfg, prdContent)

	// Set up progress bar for agent review (only in terminal, non-JSON mode)
	if !jsonOutput && (reviewMode == "hybrid" || reviewMode == "agent-only") && cfg.Agent.Enabled {
		progress := output.NewProgress(len(diff.Files), true)
		engine.SetTotalCallback(func(total int) {
			progress.SetTotal(total)
		})
		engine.SetProgress(func(fileName string, done bool, cached bool) {
			if done {
				progress.Complete(fileName, cached)
			} else {
				progress.Start(fileName)
			}
		})
		defer progress.Finish()
	}

	result, err := engine.Run(ctx, diff, reviewMode)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}

	// Filter out previously dismissed findings
	var db *store.DB
	if cfg.Store.Enabled {
		db, err = store.Open(cfg.Store.Path)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Warning: failed to open store for dismissals: %s\n", err)
			}
		} else {
			defer db.Close()
			result.Findings = review.FilterDismissed(result.Findings, db)
		}
	}

	// Export report if --export is set
	if exportFile != "" {
		if strings.HasSuffix(exportFile, ".md") {
			f, err := os.Create(exportFile)
			if err != nil {
				return fmt.Errorf("failed to create export file: %w", err)
			}
			if err := output.ExportMarkdown(result, diff, f); err != nil {
				f.Close()
				return fmt.Errorf("failed to export markdown: %w", err)
			}
			f.Close()
			fmt.Printf("Report exported to %s\n", exportFile)
		} else {
			return fmt.Errorf("unsupported export format: only .md is supported")
		}
	}

	// Step 4: Output
	elapsed := time.Since(start)
	if interactive && len(result.Findings) > 0 {
		// Interactive TUI mode — pass agent config so TUI can invoke AI fixes
		var agentCfg *model.AgentConfig
		if cfg.Agent.Enabled && cfg.Agent.APIKey != "" {
			agentCfg = &cfg.Agent
		}
		tuiModel := tui.NewModel(result, diff, agentCfg)
		p := tea.NewProgram(tuiModel, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("TUI error: %w", err)
		}

		// Print summary of user actions and save dismissals
		if fm, ok := finalModel.(tui.Model); ok {
			items := fm.Findings()
			printTUIActions(items)

			// Persist newly dismissed findings
			if db != nil {
				for _, item := range items {
					if item.Action == tui.ActionDismissed {
						f := item.Finding
						_ = db.DismissFindingByHash(f.Hash(), f.FilePath, f.RuleID, f.Message)
					}
				}
			}
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

	// Step 6: Next steps hints
	if !jsonOutput {
		printReviewHints(result, reviewMode, interactive)
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

func printReviewHints(result *review.Result, reviewMode string, isInteractive bool) {
	hasFindings := len(result.Findings) > 0
	hasErrors := result.Stats.Errors > 0

	if !hasFindings {
		output.Hint(
			"No issues found. You're good to commit!",
			"Run "+output.HintCmd("cr hook install")+" to auto-review on every commit.",
		)
		return
	}

	var hints []string

	if hasErrors {
		hints = append(hints, "Fix the errors above, then run "+output.HintCmd("cr review --staged")+" again.")
	}

	if !isInteractive {
		hints = append(hints, "Run "+output.HintCmd("cr review -i")+" for interactive mode (accept/dismiss findings).")
	}

	if reviewMode == "rules-only" {
		hints = append(hints, "Run "+output.HintCmd("cr review")+" (hybrid mode) for AI-powered deeper review.")
	}

	hints = append(hints, "Run "+output.HintCmd("cr review --json")+" to get machine-readable output for CI.")
	hints = append(hints, "Run "+output.HintCmd("cr history list")+" to view past reviews.")

	output.Hint(hints...)
}
