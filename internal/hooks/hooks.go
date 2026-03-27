package hooks

import (
	"fmt"
	"os"
	"path/filepath"
)

const preCommitScript = `#!/bin/sh
# cr code review — pre-commit hook
# Installed by: cr hook install

echo "Running cr review on staged changes..."
cr review --staged --mode rules-only --min-severity error
exit_code=$?

if [ $exit_code -ne 0 ]; then
    echo ""
    echo "Code review found errors. Commit blocked."
    echo "Use 'git commit --no-verify' to bypass, or fix the issues."
    exit 1
fi
`

const prePushScript = `#!/bin/sh
# cr code review — pre-push hook
# Installed by: cr hook install

remote="$1"
url="$2"

# Get the range of commits being pushed
while read local_ref local_oid remote_ref remote_oid; do
    if [ "$local_oid" = "0000000000000000000000000000000000000000" ]; then
        continue
    fi

    if [ "$remote_oid" = "0000000000000000000000000000000000000000" ]; then
        range="$local_oid"
    else
        range="$remote_oid..$local_oid"
    fi

    echo "Running cr review on commits: $range"
    cr review --commit "$range" --min-severity warn
    exit_code=$?

    if [ $exit_code -ne 0 ]; then
        echo ""
        echo "Code review found issues. Push blocked."
        echo "Use 'git push --no-verify' to bypass, or fix the issues."
        exit 1
    fi
done

exit 0
`

// Install installs git hooks for the current repository.
func Install(hooksDir string, hookTypes []string) error {
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}

	for _, hookType := range hookTypes {
		script := ""
		switch hookType {
		case "pre-commit":
			script = preCommitScript
		case "pre-push":
			script = prePushScript
		default:
			return fmt.Errorf("unknown hook type: %s", hookType)
		}

		hookPath := filepath.Join(hooksDir, hookType)

		// Check if hook already exists and is not ours
		if data, err := os.ReadFile(hookPath); err == nil {
			content := string(data)
			if len(content) > 0 && !IsOurHook(content) {
				// Backup existing hook
				backupPath := hookPath + ".cr-backup"
				if err := os.WriteFile(backupPath, data, 0o755); err != nil {
					return fmt.Errorf("failed to backup existing %s hook: %w", hookType, err)
				}
				fmt.Printf("  Backed up existing %s hook to %s\n", hookType, backupPath)
			}
		}

		if err := os.WriteFile(hookPath, []byte(script), 0o755); err != nil {
			return fmt.Errorf("failed to write %s hook: %w", hookType, err)
		}
		fmt.Printf("  Installed %s hook\n", hookType)
	}

	return nil
}

// Uninstall removes cr git hooks.
func Uninstall(hooksDir string, hookTypes []string) error {
	for _, hookType := range hookTypes {
		hookPath := filepath.Join(hooksDir, hookType)

		data, err := os.ReadFile(hookPath)
		if os.IsNotExist(err) {
			fmt.Printf("  %s hook not found, skipping\n", hookType)
			continue
		}
		if err != nil {
			return err
		}

		if !IsOurHook(string(data)) {
			fmt.Printf("  %s hook exists but was not installed by cr, skipping\n", hookType)
			continue
		}

		if err := os.Remove(hookPath); err != nil {
			return fmt.Errorf("failed to remove %s hook: %w", hookType, err)
		}

		// Restore backup if exists
		backupPath := hookPath + ".cr-backup"
		if _, err := os.Stat(backupPath); err == nil {
			if err := os.Rename(backupPath, hookPath); err != nil {
				return fmt.Errorf("failed to restore backup %s hook: %w", hookType, err)
			}
			fmt.Printf("  Restored original %s hook from backup\n", hookType)
		} else {
			fmt.Printf("  Removed %s hook\n", hookType)
		}
	}

	return nil
}

// IsOurHook checks if a hook script was installed by cr.
func IsOurHook(content string) bool {
	return len(content) > 0 && (contains(content, "cr code review") || contains(content, "Installed by: cr hook"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
