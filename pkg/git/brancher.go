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
}

// brancher implements Brancher.
type brancher struct{}

// NewBrancher creates a new Brancher.
func NewBrancher() Brancher {
	return &brancher{}
}

// CreateAndSwitch creates a new branch and switches to it.
func (b *brancher) CreateAndSwitch(ctx context.Context, name string) error {
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

// DefaultBranch returns the repository's default branch name via gh CLI.
func (b *brancher) DefaultBranch(ctx context.Context) (string, error) {
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
		return "", errors.Wrap(ctx, err, "get default branch")
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "", errors.Errorf(ctx, "default branch is empty")
	}
	slog.Debug("default branch", "branch", branch)
	return branch, nil
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

// MergeOriginDefault merges the remote default branch into the current branch.
func (b *brancher) MergeOriginDefault(ctx context.Context) error {
	defaultBranch, err := b.DefaultBranch(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "get default branch")
	}

	slog.Debug("merging origin default branch", "branch", defaultBranch)

	// #nosec G204 -- branch name is fetched via gh CLI from GitHub API, not user input
	cmd := exec.CommandContext(ctx, "git", "merge", "origin/"+defaultBranch)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "merge origin/"+defaultBranch)
	}
	return nil
}
