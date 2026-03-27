package cmd

import (
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/qzhello/code-review/internal/cache"
	"github.com/qzhello/code-review/internal/config"
)

var statsPeriod string

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show LLM usage statistics and cache info",
	RunE:  runStats,
}

func init() {
	statsCmd.Flags().StringVar(&statsPeriod, "period", "30d", "stats period: 7d, 30d, 90d, all")
}

func runStats(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}

	c, err := cache.New(cfg.Store.Path, 24*time.Hour)
	if err != nil {
		return fmt.Errorf("failed to open cache: %w", err)
	}
	defer c.Close()

	// Parse period
	since := time.Now().Add(-30 * 24 * time.Hour) // default 30d
	switch statsPeriod {
	case "7d":
		since = time.Now().Add(-7 * 24 * time.Hour)
	case "90d":
		since = time.Now().Add(-90 * 24 * time.Hour)
	case "all":
		since = time.Time{}
	}

	stats, err := c.GetStats(since)
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	cacheSize, _ := c.CacheSize()

	bold := color.New(color.Bold)
	cyan := color.New(color.FgCyan)
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)

	bold.Printf("LLM Usage Statistics (last %s)\n\n", statsPeriod)

	cyan.Print("  API Calls:    ")
	fmt.Printf("%d total", stats.TotalCalls)
	if stats.CachedCalls > 0 {
		green.Printf(" (%d served from cache)", stats.CachedCalls)
	}
	fmt.Println()

	cyan.Print("  Tokens In:    ")
	fmt.Printf("%s\n", formatTokens(stats.TotalTokensIn))

	cyan.Print("  Tokens Out:   ")
	fmt.Printf("%s\n", formatTokens(stats.TotalTokensOut))

	cyan.Print("  Total Cost:   ")
	yellow.Printf("$%.4f\n", stats.TotalCostUSD)

	cyan.Print("  Cache Size:   ")
	fmt.Printf("%d entries\n", cacheSize)

	if stats.TotalCalls > 0 {
		hitRate := float64(stats.CachedCalls) / float64(stats.TotalCalls) * 100
		cyan.Print("  Cache Hit:    ")
		green.Printf("%.1f%%\n", hitRate)

		avgCost := stats.TotalCostUSD / float64(stats.TotalCalls)
		cyan.Print("  Avg Cost:     ")
		fmt.Printf("$%.4f per review\n", avgCost)
	}

	fmt.Println()
	return nil
}

func formatTokens(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}
