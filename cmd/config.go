package cmd

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/qzhello/code-review/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  `Get, set, or list configuration values.`,
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show effective configuration (merged from all layers)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}

		bold := color.New(color.Bold)
		bold.Println("Effective configuration:")
		fmt.Println()

		data, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}
		fmt.Print(string(data))
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a config value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}

		// Marshal full config to a map, then look up the key
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return err
		}
		var m map[string]interface{}
		if err := yaml.Unmarshal(data, &m); err != nil {
			return err
		}

		val := lookupKey(m, args[0])
		if val == nil {
			fmt.Fprintf(os.Stderr, "key not found: %s\n", args[0])
			return nil
		}

		out, _ := yaml.Marshal(val)
		fmt.Print(string(out))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a config value in project config",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement proper key-value setting in .cr/config.yaml
		fmt.Printf("cr config set: would set %s = %s in .cr/config.yaml\n", args[0], args[1])
		fmt.Println("(manual editing of .cr/config.yaml recommended for now)")
		return nil
	},
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configListCmd)
}

// lookupKey supports dot-separated keys like "agent.model"
func lookupKey(m map[string]interface{}, key string) interface{} {
	parts := splitKey(key)
	var current interface{} = m

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[part]
			if !ok {
				return nil
			}
			current = val
		default:
			return nil
		}
	}
	return current
}

func splitKey(key string) []string {
	var parts []string
	current := ""
	for _, c := range key {
		if c == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
