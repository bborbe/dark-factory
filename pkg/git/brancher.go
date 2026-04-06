// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/bborbe/errors"
)

//counterfeiter:generate -o ../../mocks/brancher.go --fake-name Brancher . Brancher

// Brancher handles git branch operations.
type Brancher interface {
	CreateAndSwitch(ctx context.Context, name string) error
	Push(ctx context.Context, name string) error
	Switch(ctx context.Context, name string) error
	CurrentBranch(ctx context.Context) (string, error)
	Fetch(ctx context.Context) error
	FetchAndVerifyBranch(ctx context.Context, branch string) error
	DefaultBranch(ctx context.Context) (string, error)
	Pull(ctx context.Context) error
	MergeOriginDefault(ctx context.Context) error
	IsClean(ctx context.Context) (bool, error)
	MergeToDefault(ctx context.Context, branch string) error
}

// BrancherOption is a functional option for configuring a brancher.
type BrancherOption func(*brancher)

// WithDefaultBranch sets a configured default branch on the brancher.
// When set, DefaultBranch() returns this value directly without calling gh.
// Passing an empty string is a no-op (gh CLI fallback is used).
func WithDefaultBranch(branch string) BrancherOption {
	return func(b *brancher) {
		if branch != "" {
			b.configuredDefaultBranch = branch
		}
	}
}

// brancher implements Brancher.
type brancher struct {
	configuredDefaultBranch string
}

// NewBrancher creates a new Brancher.
func NewBrancher(opts ...BrancherOption) Brancher {
	b := &brancher{}
	for _, opt := range opts {
		opt(b)
	}

	return b
}

// CreateAndSwitch creates a new branch and switches to it.
func (b *brancher) CreateAndSwitch(ctx context.Context, name string) error {
	if err := ValidateBranchName(ctx, name); err != nil {
		return errors.Wrap(ctx, err, "validate branch name")
	}
	slog.Debug("creating and switching to branch", "branch", name)

	// #nosec G204 -- branch name is derived from prompt filename and sanitized
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", name)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "create and switch to branch")
	}
	return nil
}

// Push pushes a branch to the remote repository.
func (b *brancher) Push(ctx context.Context, name string) error {
	if err := ValidateBranchName(ctx, name); err != nil {
		return errors.Wrap(ctx, err, "validate branch name")
	}
	slog.Debug("pushing branch to remote", "branch", name)

	// #nosec G204 -- branch name is derived from prompt filename and sanitized
	cmd := exec.CommandContext(ctx, "git", "push", "-u", "origin", name)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "push branch to remote")
	}
	return nil
}

// Switch switches to an existing branch.
func (b *brancher) Switch(ctx context.Context, name string) error {
	if err := ValidateBranchName(ctx, name); err != nil {
		return errors.Wrap(ctx, err, "validate branch name")
	}
	slog.Debug("switching to branch", "branch", name)

	// #nosec G204 -- branch name is controlled by the application
	cmd := exec.CommandContext(ctx, "git", "checkout", name)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "switch to branch")
	}
	return nil
}

// CurrentBranch returns the name of the current branch.
func (b *brancher) CurrentBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", errors.Wrap(ctx, err, "get current branch")
	}
	branch := strings.TrimSpace(string(output))
	slog.Debug("current branch", "branch", branch)
	return branch, nil
}

// Fetch fetches updates from the remote repository.
func (b *brancher) Fetch(ctx context.Context) error {
	slog.Debug("fetching from origin")

	cmd := exec.CommandContext(ctx, "git", "fetch", "origin")
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "fetch from origin")
	}
	return nil
}

// FetchAndVerifyBranch fetches from origin and verifies the branch exists remotely.
func (b *brancher) FetchAndVerifyBranch(ctx context.Context, branch string) error {
	if err := ValidateBranchName(ctx, branch); err != nil {
		return errors.Wrap(ctx, err, "validate branch name")
	}
	slog.Debug("fetching from origin and verifying branch", "branch", branch)

	// #nosec G204 -- branch name comes from validated frontmatter
	cmd := exec.CommandContext(ctx, "git", "fetch", "origin")
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "fetch from origin")
	}

	// #nosec G204 -- branch name comes from validated frontmatter
	cmd = exec.CommandContext(ctx, "git", "rev-parse", "--verify", "origin/"+branch)
	if err := cmd.Run(); err != nil {
		return errors.Errorf(ctx, "branch not found at origin: %s", branch)
	}
	return nil
}

// DefaultBranch returns the repository's default branch name.
// If a default branch was configured via WithDefaultBranch, it is returned directly.
// Otherwise, gh CLI is used to query the remote repository.
func (b *brancher) DefaultBranch(ctx context.Context) (string, error) {
	if b.configuredDefaultBranch != "" {
		slog.Debug("default branch from config", "branch", b.configuredDefaultBranch)
		return b.configuredDefaultBranch, nil
	}
	// #nosec G204 -- static command with no user input
	cmd := exec.CommandContext(
		ctx,
		"gh",
		"repo",
		"view",
		"--json",
		"defaultBranchRef",
		"--jq",
		".defaultBranchRef.name",
	)
	output, err := cmd.Output()
	if err != nil {
		if branch := defaultBranchFromSymbolicRef(ctx); branch != "" {
			return branch, nil
		}
		return "", errors.Wrap(ctx, err, "get default branch")
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "", errors.Errorf(ctx, "default branch is empty")
	}
	slog.Debug("default branch", "branch", branch)
	return branch, nil
}

// defaultBranchFromSymbolicRef tries to determine the default branch using
// git symbolic-ref refs/remotes/origin/HEAD, which works for any git remote.
func defaultBranchFromSymbolicRef(ctx context.Context) string {
	// #nosec G204 -- static command with no user input
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	const prefix = "refs/remotes/origin/"
	ref := strings.TrimSpace(string(output))
	if !strings.HasPrefix(ref, prefix) {
		return ""
	}
	branch := strings.TrimPrefix(ref, prefix)
	if branch == "" {
		return ""
	}
	slog.Debug("default branch from git symbolic-ref", "branch", branch)
	return branch
}

// Pull runs git pull on the current branch.
func (b *brancher) Pull(ctx context.Context) error {
	slog.Debug("pulling current branch")

	cmd := exec.CommandContext(ctx, "git", "pull")
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "pull current branch")
	}
	return nil
}

// IsClean returns true if the working tree has no uncommitted changes.
func (b *brancher) IsClean(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false, errors.Wrap(ctx, err, "check working tree status")
	}
	return strings.TrimSpace(string(output)) == "", nil
}

// MergeOriginDefault merges the remote default branch into the current branch.
// If the default branch cannot be determined, it logs a warning and skips the merge.
func (b *brancher) MergeOriginDefault(ctx context.Context) error {
	defaultBranch, err := b.DefaultBranch(ctx)
	if err != nil {
		slog.Warn("skipping merge origin default: could not determine default branch", "error", err)
		return nil
	}

	slog.Debug("merging origin default branch", "branch", defaultBranch)

	// #nosec G204 -- branch name is fetched via gh CLI from GitHub API, not user input
	cmd := exec.CommandContext(ctx, "git", "merge", "origin/"+defaultBranch)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "merge origin/"+defaultBranch)
	}
	return nil
}

// MergeToDefault merges the given feature branch into the default branch.
// It ensures the repo is on the default branch before merging.
func (b *brancher) MergeToDefault(ctx context.Context, branch string) error {
	if err := ValidateBranchName(ctx, branch); err != nil {
		return errors.Wrap(ctx, err, "validate branch name")
	}
	defaultBranch, err := b.DefaultBranch(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "get default branch")
	}
	// Ensure we're on the default branch
	// #nosec G204 -- defaultBranch comes from gh CLI, branch comes from validated frontmatter
	if err := exec.CommandContext(ctx, "git", "checkout", defaultBranch).Run(); err != nil {
		return errors.Wrap(ctx, err, "switch to default branch before merge")
	}
	// Merge feature branch into default (no fast-forward to preserve branch history)
	// #nosec G204 -- branch name comes from validated frontmatter
	cmd := exec.CommandContext(ctx, "git", "merge", "--no-ff", branch)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(ctx, err, "merge branch %q to default: %s", branch, stderr.String())
	}
	slog.Info("merged feature branch to default", "branch", branch, "default", defaultBranch)
	return nil
}
