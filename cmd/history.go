package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
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
		// TODO: implement in Phase 6
		fmt.Println("cr history list — not yet implemented (Phase 6)")
		return nil
	},
}

var historyShowCmd = &cobra.Command{
	Use:   "show [id]",
	Short: "Show findings from a past review",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement in Phase 6
		fmt.Printf("cr history show %s — not yet implemented (Phase 6)\n", args[0])
		return nil
	},
}

var historyClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear review history",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement in Phase 6
		fmt.Println("cr history clear — not yet implemented (Phase 6)")
		return nil
	},
}

var historyDismissCmd = &cobra.Command{
	Use:   "dismiss [finding-id]",
	Short: "Dismiss a finding so it won't re-report",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement in Phase 6
		fmt.Printf("cr history dismiss %s — not yet implemented (Phase 6)\n", args[0])
		return nil
	},
}

func init() {
	historyCmd.AddCommand(historyListCmd)
	historyCmd.AddCommand(historyShowCmd)
	historyCmd.AddCommand(historyClearCmd)
	historyCmd.AddCommand(historyDismissCmd)
}
