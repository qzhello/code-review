package rules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/qzhello/code-review/internal/model"
)

// LoadFromPaths loads rules from all rule files in the given directories.
// Supports .yaml, .yml, and .json files.
func LoadFromPaths(paths []string, disabled []string) ([]model.Rule, error) {
	disabledSet := make(map[string]bool, len(disabled))
	for _, id := range disabled {
		disabledSet[id] = true
	}

	var allRules []model.Rule

	for _, dir := range paths {
		rules, err := loadFromDir(dir, disabledSet)
		if err != nil {
			return nil, err
		}
		allRules = append(allRules, rules...)
	}

	return allRules, nil
}

// LoadFromFile loads rules from a single file (YAML or JSON, detected by extension).
func LoadFromFile(path string, disabled map[string]bool) ([]model.Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read rules file %s: %w", path, err)
	}

	var rf model.RuleFile
	if isJSON(path) {
		if err := json.Unmarshal(data, &rf); err != nil {
			return nil, fmt.Errorf("failed to parse JSON rules file %s: %w", path, err)
		}
	} else {
		if err := yaml.Unmarshal(data, &rf); err != nil {
			return nil, fmt.Errorf("failed to parse YAML rules file %s: %w", path, err)
		}
	}

	for i := range rf.Rules {
		rf.Rules[i].Source = path
		rf.Rules[i].Enabled = !disabled[rf.Rules[i].ID]
		if rf.Rules[i].Scope == "" {
			rf.Rules[i].Scope = model.ScopeAdded
		}
	}

	return rf.Rules, nil
}

// ValidateFile checks if a rules file is syntactically valid.
// Supports both YAML and JSON.
func ValidateFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}

	var rf model.RuleFile
	if isJSON(path) {
		if err := json.Unmarshal(data, &rf); err != nil {
			return fmt.Errorf("invalid JSON in %s: %w", path, err)
		}
	} else {
		if err := yaml.Unmarshal(data, &rf); err != nil {
			return fmt.Errorf("invalid YAML in %s: %w", path, err)
		}
	}

	for _, r := range rf.Rules {
		if r.ID == "" {
			return fmt.Errorf("rule missing 'id' in %s", path)
		}
		if r.Severity == "" {
			return fmt.Errorf("rule %q missing 'severity' in %s", r.ID, path)
		}
		if _, err := model.ParseSeverity(r.Severity); err != nil {
			return fmt.Errorf("rule %q in %s: %w", r.ID, path, err)
		}
	}

	return nil
}

// ExportToJSON exports rules to JSON format.
func ExportToJSON(rules []model.Rule) ([]byte, error) {
	rf := model.RuleFile{Rules: rules}
	return json.MarshalIndent(rf, "", "  ")
}

// ExportToYAML exports rules to YAML format.
func ExportToYAML(rules []model.Rule) ([]byte, error) {
	rf := model.RuleFile{Rules: rules}
	return yaml.Marshal(rf)
}

func loadFromDir(dir string, disabled map[string]bool) ([]model.Rule, error) {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return LoadFromFile(dir, disabled)
	}

	var allRules []model.Rule

	// Collect all supported rule files
	var entries []string
	for _, pattern := range []string{"*.yaml", "*.yml", "*.json"} {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, err
		}
		entries = append(entries, matches...)
	}

	for _, entry := range entries {
		rules, err := LoadFromFile(entry, disabled)
		if err != nil {
			return nil, err
		}
		allRules = append(allRules, rules...)
	}

	return allRules, nil
}

// isJSON returns true if the file path has a .json extension.
func isJSON(path string) bool {
	return strings.ToLower(filepath.Ext(path)) == ".json"
}

// IsRuleFile returns true if the file has a supported rules extension.
func IsRuleFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml" || ext == ".json"
}
