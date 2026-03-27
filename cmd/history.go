package cmd

import (
	"fmt"
	"strconv"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/quzhihao/code-review/internal/config"
	"github.com/quzhihao/code-review/internal/model"
	"github.com/quzhihao/code-review/internal/review"
	"github.com/quzhihao/code-review/internal/store"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View review history",
	Long:  `List, show, or clear past review results.`,
}

var historyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List past reviews",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openStore()
		if err != nil {
			return err
		}
		defer db.Close()

		reviews, err := db.ListReviews(20)
		if err != nil {
			return fmt.Errorf("failed to list reviews: %w", err)
		}

		if len(reviews) == 0 {
			fmt.Println("No review history.")
			return nil
		}

		bold := color.New(color.Bold)
		red := color.New(color.FgRed)
		yellow := color.New(color.FgYellow)
		cyan := color.New(color.FgCyan)

		bold.Printf("%-5s %-20s %-15s %-8s %-8s %s\n", "ID", "Date", "Branch", "Commit", "Mode", "Findings")
		for _, r := range reviews {
			findingsStr := ""
			if r.Errors > 0 {
				findingsStr += red.Sprintf("%dE ", r.Errors)
			}
			if r.Warnings > 0 {
				findingsStr += yellow.Sprintf("%dW ", r.Warnings)
			}
			if r.Infos > 0 {
				findingsStr += cyan.Sprintf("%dI", r.Infos)
			}
			if findingsStr == "" {
				findingsStr = color.GreenString("clean")
			}

			fmt.Printf("%-5d %-20s %-15s %-8s %-8s %s\n",
				r.ID,
				r.CreatedAt.Format("2006-01-02 15:04:05"),
				truncate(r.Branch, 15),
				truncate(r.CommitHash, 8),
				r.Mode,
				findingsStr,
			)
		}

		return nil
	},
}

var historyShowCmd = &cobra.Command{
	Use:   "show [id]",
	Short: "Show findings from a past review",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid review ID: %s", args[0])
		}

		db, err := openStore()
		if err != nil {
			return err
		}
		defer db.Close()

		r, err := db.GetReview(id)
		if err != nil {
			return fmt.Errorf("review not found: %w", err)
		}

		bold := color.New(color.Bold)
		bold.Printf("Review #%d\n", r.ID)
		fmt.Printf("  Branch: %s | Commit: %s | Mode: %s\n", r.Branch, r.CommitHash, r.Mode)
		fmt.Printf("  Date: %s\n\n", r.CreatedAt.Format("2006-01-02 15:04:05"))

		// Reuse terminal output
		result := &review.Result{
			Findings: r.Findings,
			Stats: review.Stats{
				Errors:   r.Errors,
				Warnings: r.Warnings,
				Infos:    r.Infos,
			},
		}

		errColor := color.New(color.FgRed, color.Bold)
		warnColor := color.New(color.FgYellow, color.Bold)
		infoColor := color.New(color.FgCyan, color.Bold)

		for _, f := range result.Findings {
			var sevColor *color.Color
			var sevLabel string
			switch f.Severity {
			case model.SeverityError:
				sevColor = errColor
				sevLabel = "ERROR"
			case model.SeverityWarn:
				sevColor = warnColor
				sevLabel = "WARN "
			default:
				sevColor = infoColor
				sevLabel = "INFO "
			}

			sevColor.Printf("  %s", sevLabel)
			fmt.Printf("  %s  [%s]\n", f.Location(), f.RuleID)
			fmt.Printf("         %s\n", f.Message)
		}

		if len(result.Findings) == 0 {
			color.Green("  No issues found.")
		}
		fmt.Println()

		return nil
	},
}

var historyClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear review history",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openStore()
		if err != nil {
			return err
		}
		defer db.Close()

		if err := db.ClearReviews(); err != nil {
			return fmt.Errorf("failed to clear history: %w", err)
		}

		fmt.Println("Review history cleared.")
		return nil
	},
}

var historyDismissCmd = &cobra.Command{
	Use:   "dismiss [finding-id]",
	Short: "Dismiss a finding so it won't re-report",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Dismissal tracking: use review ID + finding index (e.g., cr history dismiss 1:0)\n")
		fmt.Println("This feature will be enhanced in a future release.")
		return nil
	},
}

func init() {
	historyCmd.AddCommand(historyListCmd)
	historyCmd.AddCommand(historyShowCmd)
	historyCmd.AddCommand(historyClearCmd)
	historyCmd.AddCommand(historyDismissCmd)
}

func openStore() (*store.DB, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, err
	}
	if !cfg.Store.Enabled {
		return nil, fmt.Errorf("store is disabled in config")
	}
	return store.Open(cfg.Store.Path)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
