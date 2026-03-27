package rules

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/quzhihao/code-review/internal/model"
)

// LoadFromPaths loads rules from all YAML files in the given directories.
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

// LoadFromFile loads rules from a single YAML file.
func LoadFromFile(path string, disabled map[string]bool) ([]model.Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read rules file %s: %w", path, err)
	}

	var rf model.RuleFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("failed to parse rules file %s: %w", path, err)
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

// ValidateFile checks if a YAML rules file is syntactically valid.
func ValidateFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}

	var rf model.RuleFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return fmt.Errorf("invalid YAML in %s: %w", path, err)
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
	entries, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	ymlEntries, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return nil, err
	}
	entries = append(entries, ymlEntries...)

	for _, entry := range entries {
		rules, err := LoadFromFile(entry, disabled)
		if err != nil {
			return nil, err
		}
		allRules = append(allRules, rules...)
	}

	return allRules, nil
}
