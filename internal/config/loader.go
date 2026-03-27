package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"

	"github.com/quzhihao/code-review/internal/model"
)

// Load loads configuration with layered precedence:
// defaults → global (~/.cr/config.yaml) → project (.cr/config.yaml) → env → overrides
func Load(projectConfigPath string) (*model.Config, error) {
	cfg := DefaultConfig()

	v := viper.New()
	v.SetConfigType("yaml")

	// Set defaults from struct
	setViperDefaults(v, cfg)

	// Layer 1: Global config (~/.cr/config.yaml)
	home, err := os.UserHomeDir()
	if err == nil {
		globalPath := filepath.Join(home, ".cr", "config.yaml")
		if _, err := os.Stat(globalPath); err == nil {
			v.SetConfigFile(globalPath)
			if err := v.MergeInConfig(); err != nil {
				return nil, fmt.Errorf("failed to read global config %s: %w", globalPath, err)
			}
		}
	}

	// Layer 2: Project config (.cr/config.yaml or custom path)
	if projectConfigPath == "" {
		projectConfigPath = ".cr/config.yaml"
	}
	if _, err := os.Stat(projectConfigPath); err == nil {
		v.SetConfigFile(projectConfigPath)
		if err := v.MergeInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read project config %s: %w", projectConfigPath, err)
		}
	}

	// Layer 3: Environment variables
	v.SetEnvPrefix("CR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Map specific env vars
	_ = v.BindEnv("agent.api_key", "OPENAI_API_KEY", "CR_AGENT_API_KEY")

	// Unmarshal into config struct
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Expand ~ in store path
	cfg.Store.Path = expandHome(cfg.Store.Path)

	return cfg, nil
}

func setViperDefaults(v *viper.Viper, cfg *model.Config) {
	v.SetDefault("review.mode", cfg.Review.Mode)
	v.SetDefault("review.context_lines", cfg.Review.ContextLines)
	v.SetDefault("review.include", cfg.Review.Include)
	v.SetDefault("review.exclude", cfg.Review.Exclude)
	v.SetDefault("review.max_file_size_kb", cfg.Review.MaxFileSizeKB)

	v.SetDefault("rules.paths", cfg.Rules.Paths)
	v.SetDefault("rules.builtin", cfg.Rules.Builtin)
	v.SetDefault("rules.disabled", cfg.Rules.Disabled)

	v.SetDefault("agent.enabled", cfg.Agent.Enabled)
	v.SetDefault("agent.provider", cfg.Agent.Provider)
	v.SetDefault("agent.model", cfg.Agent.Model)
	v.SetDefault("agent.temperature", cfg.Agent.Temperature)
	v.SetDefault("agent.max_concurrency", cfg.Agent.MaxConcurrency)
	v.SetDefault("agent.timeout", cfg.Agent.Timeout)
	v.SetDefault("agent.confidence_threshold", cfg.Agent.ConfidenceThreshold)
	v.SetDefault("agent.focus", cfg.Agent.Focus)
	v.SetDefault("agent.ignore", cfg.Agent.Ignore)
	v.SetDefault("agent.persona", cfg.Agent.Persona)

	v.SetDefault("noise.min_severity", cfg.Noise.MinSeverity)
	v.SetDefault("noise.group_threshold", cfg.Noise.GroupThreshold)
	v.SetDefault("noise.suppress_dismissed", cfg.Noise.SuppressDismissed)
	v.SetDefault("noise.dedup", cfg.Noise.Dedup)

	v.SetDefault("output.format", cfg.Output.Format)
	v.SetDefault("output.color", cfg.Output.Color)
	v.SetDefault("output.show_context", cfg.Output.ShowContext)
	v.SetDefault("output.verbose", cfg.Output.Verbose)

	v.SetDefault("store.enabled", cfg.Store.Enabled)
	v.SetDefault("store.path", cfg.Store.Path)
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
