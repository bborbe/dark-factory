// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"log/slog"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/subproc"
)

//counterfeiter:generate -o ../../mocks/worktreer.go --fake-name Worktreer . Worktreer

// Worktreer handles git worktree operations.
type Worktreer interface {
	// Add creates a linked worktree at worktreePath on branch.
	Add(ctx context.Context, worktreePath string, branch string) error
	// Remove removes the linked worktree at worktreePath.
	Remove(ctx context.Context, worktreePath string) error
}

// NewWorktreer creates a new Worktreer.
func NewWorktreer() Worktreer {
	return &worktreer{runner: subproc.NewRunner()}
}

// newWorktreerWithRunner creates a Worktreer with an injected runner (for tests).
func newWorktreerWithRunner(r subproc.Runner) *worktreer {
	return &worktreer{runner: r}
}

// worktreer implements Worktreer.
type worktreer struct {
	runner subproc.Runner
}

// Add creates a linked worktree at worktreePath on branch.
func (w *worktreer) Add(ctx context.Context, worktreePath string, branch string) error {
	slog.Info("adding worktree", "path", worktreePath, "branch", branch)

	// Check if branch exists locally.
	_, checkErr := w.runner.RunWithWarnAndTimeout(
		ctx,
		"git rev-parse --verify",
		"git",
		"rev-parse",
		"--verify",
		"--quiet",
		"refs/heads/"+branch,
	)
	branchExists := checkErr == nil

	var err error
	if branchExists {
		_, err = w.runner.RunWithWarnAndTimeout(
			ctx,
			"git worktree add",
			"git",
			"worktree",
			"add",
			worktreePath,
			branch,
		)
	} else {
		_, err = w.runner.RunWithWarnAndTimeout(
			ctx,
			"git worktree add -b",
			"git",
			"worktree",
			"add",
			"-b",
			branch,
			worktreePath,
		)
	}

	if err != nil {
		return errors.Wrapf(
			ctx,
			err,
			"git worktree add (path=%s branch=%s exists=%t): %s",
			worktreePath,
			branch,
			branchExists,
			stderrFromErr(err),
		)
	}
	return nil
}

// Remove removes the linked worktree at worktreePath.
func (w *worktreer) Remove(ctx context.Context, worktreePath string) error {
	slog.Debug("removing worktree", "path", worktreePath)
	_, err := w.runner.RunWithWarnAndTimeout(
		ctx,
		"git worktree remove --force",
		"git",
		"worktree",
		"remove",
		"--force",
		worktreePath,
	)
	if err != nil {
		slog.Warn(
			"git worktree remove failed",
			"path",
			worktreePath,
			"error",
			err,
			"stderr",
			stderrFromErr(err),
		)
	}
	return nil
}
