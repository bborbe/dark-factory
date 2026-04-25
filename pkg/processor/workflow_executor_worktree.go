// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

// worktreeWorkflowExecutor handles WorkflowWorktree.
type worktreeWorkflowExecutor struct {
	deps WorkflowDeps
	// state set during Setup
	branchName   string
	worktreePath string
	originalDir  string
	cleanedUp    bool
}

// NewWorktreeWorkflowExecutor creates a WorkflowExecutor for the worktree workflow.
func NewWorktreeWorkflowExecutor(deps WorkflowDeps) WorkflowExecutor {
	return &worktreeWorkflowExecutor{deps: deps}
}

// Setup syncs with remote, creates a worktree, and chdirs into it.
func (e *worktreeWorkflowExecutor) Setup(
	ctx context.Context,
	baseName BaseName,
	pf *prompt.PromptFile,
) error {
	if err := syncWithRemoteViaDeps(ctx, e.deps); err != nil {
		return err
	}

	branch := pf.Branch()
	if branch == "" {
		branch = "dark-factory/" + string(baseName)
	}
	e.branchName = branch
	e.worktreePath = filepath.Join(
		os.TempDir(),
		"dark-factory",
		string(e.deps.ProjectName)+"-"+string(baseName),
	)

	originalDir, err := os.Getwd()
	if err != nil {
		return errors.Wrap(ctx, err, "get current directory")
	}
	e.originalDir = originalDir

	if err := e.deps.Worktreer.Add(ctx, e.worktreePath, branch); err != nil {
		return errors.Wrap(ctx, err, "add worktree")
	}

	if err := os.Chdir(e.worktreePath); err != nil {
		// Remove worktree since we couldn't chdir into it
		if removeErr := e.deps.Worktreer.Remove(ctx, e.worktreePath); removeErr != nil {
			slog.Warn(
				"failed to remove worktree after chdir error",
				"path",
				e.worktreePath,
				"error",
				removeErr,
			)
		}
		return errors.Wrap(ctx, err, "chdir to worktree")
	}

	return nil
}

// CleanupOnError restores the original directory and removes the worktree on error.
func (e *worktreeWorkflowExecutor) CleanupOnError(ctx context.Context) {
	if e.cleanedUp {
		return
	}
	if e.originalDir != "" {
		if err := os.Chdir(e.originalDir); err != nil {
			slog.Warn("failed to chdir back to original directory on error", "error", err)
		}
	}
	if e.worktreePath != "" {
		if err := e.deps.Worktreer.Remove(ctx, e.worktreePath); err != nil {
			slog.Warn("failed to remove worktree on error", "path", e.worktreePath, "error", err)
		}
	}
}

// Complete commits in the worktree, chdirs back, removes the worktree, then handles push/PR.
func (e *worktreeWorkflowExecutor) Complete(
	gitCtx, ctx context.Context,
	pf *prompt.PromptFile,
	title, promptPath, completedPath string,
) error {
	if err := e.deps.Releaser.CommitOnly(gitCtx, title); err != nil {
		return errors.Wrap(ctx, err, "commit changes")
	}

	if err := os.Chdir(e.originalDir); err != nil {
		return errors.Wrap(ctx, err, "chdir back to original directory")
	}

	if err := e.deps.Worktreer.Remove(gitCtx, e.worktreePath); err != nil {
		slog.Warn("failed to remove worktree", "path", e.worktreePath, "error", err)
	}
	e.cleanedUp = true

	return handleAfterIsolatedCommit(
		gitCtx,
		ctx,
		e.deps,
		pf,
		e.branchName,
		title,
		promptPath,
		completedPath,
	)
}

// ReconstructState checks if the worktree directory still exists for resume.
func (e *worktreeWorkflowExecutor) ReconstructState(
	ctx context.Context,
	baseName BaseName,
	pf *prompt.PromptFile,
) (bool, error) {
	worktreePath := filepath.Join(
		os.TempDir(),
		"dark-factory",
		string(e.deps.ProjectName)+"-"+string(baseName),
	)
	if _, err := os.Stat(worktreePath); err != nil {
		return false, nil // worktree missing — signal reset-to-approved
	}
	branch := pf.Branch()
	if branch == "" {
		branch = "dark-factory/" + string(baseName)
	}
	e.branchName = branch
	e.worktreePath = worktreePath
	originalDir, err := os.Getwd()
	if err != nil {
		return false, errors.Wrap(ctx, err, "get working directory for resume")
	}
	e.originalDir = originalDir
	return true, nil
}
