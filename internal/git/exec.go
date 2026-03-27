package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DiffOptions controls how git diff is invoked.
type DiffOptions struct {
	Staged       bool
	Branch       string // compare HEAD to this branch (e.g., "main")
	ContextLines int    // lines of context (default 3)
	Paths        []string
}

// ExecDiff runs git diff and returns the raw output.
func ExecDiff(ctx context.Context, opts DiffOptions) (string, error) {
	args := buildDiffArgs(opts)
	cmd := exec.CommandContext(ctx, "git", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// git diff returns exit code 1 when there are differences, which is normal
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return stdout.String(), nil
		}
		return "", fmt.Errorf("git diff failed: %s: %w", strings.TrimSpace(stderr.String()), err)
	}

	return stdout.String(), nil
}

func buildDiffArgs(opts DiffOptions) []string {
	args := []string{"diff", "--no-color"}

	contextLines := opts.ContextLines
	if contextLines <= 0 {
		contextLines = 3
	}
	args = append(args, fmt.Sprintf("-U%d", contextLines))

	if opts.Staged {
		args = append(args, "--cached")
	} else if opts.Branch != "" {
		args = append(args, fmt.Sprintf("%s...HEAD", opts.Branch))
	}

	if len(opts.Paths) > 0 {
		args = append(args, "--")
		args = append(args, opts.Paths...)
	}

	return args
}

// GetCurrentBranch returns the current git branch name.
func GetCurrentBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// GetRepoRoot returns the root directory of the git repository.
func GetRepoRoot(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// GetHeadCommit returns the short hash of HEAD.
func GetHeadCommit(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get HEAD commit: %w", err)
	}
	return strings.TrimSpace(stdout.String()), nil
}
