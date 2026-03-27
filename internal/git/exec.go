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
	Branch       string   // compare HEAD to this branch (e.g., "main")
	Commit       string   // commit range (e.g., "abc..def", "HEAD~3", "v1.0.0..v2.0.0")
	ContextLines int      // lines of context (default 3)
	Paths        []string // restrict diff to these paths
}

// ExecDiff runs git diff and returns the raw output.
func ExecDiff(ctx context.Context, opts DiffOptions) (string, error) {
	// Normalize commit ref: if it's a single ref (no ".." range), convert to ref..HEAD
	if opts.Commit != "" {
		opts.Commit = normalizeCommitRef(ctx, opts.Commit)
	}

	// Handle root commit (no parent) via git diff-tree
	if strings.HasPrefix(opts.Commit, "__ROOT__:") {
		hash := strings.TrimPrefix(opts.Commit, "__ROOT__:")
		return execDiffTree(ctx, hash, opts)
	}

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
		errMsg := strings.TrimSpace(stderr.String())
		if strings.Contains(errMsg, "unknown revision") {
			return "", fmt.Errorf("revision %q not found — check that it exists (enough commits? correct tag/branch name?)", opts.Commit)
		}
		return "", fmt.Errorf("git diff failed: %s: %w", errMsg, err)
	}

	return stdout.String(), nil
}

// normalizeCommitRef converts shorthand refs to proper git diff ranges.
//
//	"HEAD~3"       → "HEAD~3..HEAD"  (last 3 commits)
//	"abc123"       → "abc123^..abc123" (single commit)
//	"v1.0..v2.0"   → "v1.0..v2.0"   (already a range, unchanged)
//	"main...HEAD"  → "main...HEAD"   (already a range, unchanged)
func normalizeCommitRef(ctx context.Context, ref string) string {
	// Already a range — don't touch
	if strings.Contains(ref, "..") {
		return ref
	}

	// HEAD~N pattern — convert to HEAD~N..HEAD
	if strings.HasPrefix(ref, "HEAD~") || strings.HasPrefix(ref, "HEAD^") {
		return ref + "..HEAD"
	}

	// Single ref (commit hash, tag, etc.) — show just that commit's changes
	// First verify the ref resolves
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", ref+"^{commit}")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		// Can't verify — return as-is and let git diff report the error
		return ref
	}

	// Check if the commit has a parent (not root commit)
	parentCmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--quiet", ref+"^")
	if err := parentCmd.Run(); err != nil {
		// Root commit — diff the full commit's tree against empty tree
		// git diff-tree will be called separately via ExecDiffTree
		return "__ROOT__:" + strings.TrimSpace(out.String())
	}

	return ref + "^.." + ref
}

func buildDiffArgs(opts DiffOptions) []string {
	args := []string{"diff", "--no-color"}

	contextLines := opts.ContextLines
	if contextLines <= 0 {
		contextLines = 3
	}
	args = append(args, fmt.Sprintf("-U%d", contextLines))

	if opts.Commit != "" {
		// Support: "abc..def", "abc...def", "HEAD~3", "v1.0..v2.0", single commit
		args = append(args, opts.Commit)
	} else if opts.Staged {
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

// execDiffTree handles diffing the root commit (no parent) using git diff-tree.
func execDiffTree(ctx context.Context, hash string, opts DiffOptions) (string, error) {
	args := []string{"diff-tree", "--no-color", "-p", "--root", hash}

	if len(opts.Paths) > 0 {
		args = append(args, "--")
		args = append(args, opts.Paths...)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return stdout.String(), nil
		}
		return "", fmt.Errorf("git diff-tree failed: %s: %w", strings.TrimSpace(stderr.String()), err)
	}

	return stdout.String(), nil
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
