package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var dismissCmd = &cobra.Command{
	Use:   "dismiss",
	Short: "Manage persistently dismissed findings",
	Long:  `List or clear findings that have been dismissed in interactive reviews.`,
}

var dismissListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all dismissed findings",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openStore()
		if err != nil {
			return err
		}
		defer db.Close()

		dismissed, err := db.ListDismissed()
		if err != nil {
			return fmt.Errorf("failed to list dismissed findings: %w", err)
		}

		if len(dismissed) == 0 {
			fmt.Println("No dismissed findings.")
			return nil
		}

		bold := color.New(color.Bold)
		dim := color.New(color.Faint)

		bold.Printf("%-8s %-30s %-20s %s\n", "ID", "File", "Rule", "Message")
		for _, d := range dismissed {
			msg := d.Message
			if len(msg) > 60 {
				msg = msg[:57] + "..."
			}
			fmt.Printf("%-8d %-30s %-20s %s\n", d.ID, truncate(d.FilePath, 30), truncate(d.RuleID, 20), msg)
			dim.Printf("         hash: %s  dismissed: %s\n", d.Hash[:16], d.DismissedAt.Format("2006-01-02 15:04"))
		}

		fmt.Printf("\nTotal: %d dismissed finding(s)\n", len(dismissed))
		return nil
	},
}

var dismissClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all dismissed findings",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openStore()
		if err != nil {
			return err
		}
		defer db.Close()

		if err := db.ClearDismissed(); err != nil {
			return fmt.Errorf("failed to clear dismissed findings: %w", err)
		}

		fmt.Println("All dismissed findings have been cleared.")
		return nil
	},
}

func init() {
	dismissCmd.AddCommand(dismissListCmd)
	dismissCmd.AddCommand(dismissClearCmd)
}
