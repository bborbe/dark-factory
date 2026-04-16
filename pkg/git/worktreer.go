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

//counterfeiter:generate -o ../../mocks/worktreer.go --fake-name Worktreer . Worktreer

// Worktreer handles git worktree operations.
type Worktreer interface {
	// Add creates a linked worktree at worktreePath on branch.
	// If the branch does not yet exist, it is created from the current HEAD.
	// Returns a wrapped error on failure (e.g. branch already checked out elsewhere).
	Add(ctx context.Context, worktreePath string, branch string) error

	// Remove removes the linked worktree at worktreePath.
	// Uses --force to handle cases where the worktree is in an unclean state.
	// Failure is logged as a warning but does NOT return an error (callers treat
	// cleanup failure as non-fatal, per spec constraint).
	Remove(ctx context.Context, worktreePath string) error
}

// NewWorktreer creates a new Worktreer.
func NewWorktreer() Worktreer {
	return &worktreer{}
}

// worktreer implements Worktreer.
type worktreer struct{}

// Add creates a linked worktree at worktreePath on branch.
// If branch already exists locally, checks it out into the new worktree.
// If branch does not exist, creates it from the current HEAD.
func (w *worktreer) Add(ctx context.Context, worktreePath string, branch string) error {
	slog.Info("adding worktree", "path", worktreePath, "branch", branch)

	// Check if branch exists locally — decides whether we pass -b (create) or not (checkout existing).
	// #nosec G204 -- branch is derived from config and prompt filename
	check := exec.CommandContext(
		ctx,
		"git",
		"rev-parse",
		"--verify",
		"--quiet",
		"refs/heads/"+branch,
	)
	branchExists := check.Run() == nil

	var cmd *exec.Cmd
	if branchExists {
		cmd = exec.CommandContext(
			ctx,
			"git",
			"worktree",
			"add",
			worktreePath,
			branch,
		) // #nosec G204 -- worktreePath and branch are derived from config and prompt filename
	} else {
		cmd = exec.CommandContext(ctx, "git", "worktree", "add", "-b", branch, worktreePath) // #nosec G204 -- branch and worktreePath are derived from config and prompt filename
	}

	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(
			ctx,
			err,
			"git worktree add (path=%s branch=%s exists=%t): %s",
			worktreePath,
			branch,
			branchExists,
			stderr.String(),
		)
	}
	return nil
}

// Remove removes the linked worktree at worktreePath.
func (w *worktreer) Remove(ctx context.Context, worktreePath string) error {
	slog.Debug("removing worktree", "path", worktreePath)
	// #nosec G204 -- worktreePath is derived from config and prompt filename
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", worktreePath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		slog.Warn(
			"git worktree remove failed",
			"path",
			worktreePath,
			"error",
			err,
			"stderr",
			stderr.String(),
		)
	}
	return nil
}
