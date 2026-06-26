// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	"os"
	"path/filepath"

	"github.com/bborbe/errors"

	log "github.com/bborbe/dark-factory/pkg/log"
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
	baseName prompt.BaseName,
	pf *prompt.PromptFile,
) error {
	if err := syncWithRemoteViaDeps(ctx, e.deps); err != nil {
		return errors.Wrap(ctx, err, "sync with remote")
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
			log.From(ctx).Warn(
				"failed to remove worktree after chdir error",
				"dir", e.worktreePath,
				"error", removeErr,
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
			log.From(ctx).Warn("failed to chdir back to original directory on error", "error", err)
		}
	}
	if e.worktreePath != "" {
		if err := e.deps.Worktreer.Remove(ctx, e.worktreePath); err != nil {
			log.From(ctx).
				Warn("failed to remove worktree on error", "dir", e.worktreePath, "error", err)
		}
	}
}

// Complete moves the prompt to completed/, commits in the worktree, chdirs back,
// removes the worktree, then handles push/PR via handleAfterIsolatedCommit.
// No rollback on failure: the worktree is discarded on cleanup; original prompt path untouched.
func (e *worktreeWorkflowExecutor) Complete(
	gitCtx, ctx context.Context,
	pf *prompt.PromptFile,
	title, promptPath, completedPath string,
) error {
	// Move prompt to completed/ inside the worktree (sets status: completed, physically moves the file).
	if err := e.deps.PromptManager.MoveToCompleted(ctx, promptPath); err != nil {
		return errors.Wrap(ctx, err, "move to completed")
	}
	log.From(ctx).Info("moved to completed")

	// Single combined commit: work changes + prompt move.
	if err := e.deps.Releaser.CommitOnly(gitCtx, title); err != nil {
		return errors.Wrap(ctx, err, "commit changes")
	}

	if err := os.Chdir(e.originalDir); err != nil {
		return errors.Wrap(ctx, err, "chdir back to original directory")
	}

	// no rollback needed: worktree is discarded on cleanup; original prompt path untouched
	if err := e.deps.Worktreer.Remove(gitCtx, e.worktreePath); err != nil {
		log.From(ctx).Warn("failed to remove worktree", "dir", e.worktreePath, "error", err)
	}
	e.cleanedUp = true

	if syncErr := syncPromptFileToOriginalRepo(
		ctx,
		e.deps.PromptManager,
		promptPath,
		completedPath,
	); syncErr != nil {
		log.From(ctx).Warn(
			"clone-sync-mismatch",
			"completed_path", completedPath,
			"remote_branch", e.branchName,
			"error", syncErr.Error(),
			"hint", "remote has the rename; run `git pull` on the original repo to catch up",
		)
		// success-with-warning: do not propagate; remote is already correct.
	}

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

// ReconstructState checks if the worktree directory still exists for resume,
// and chdirs into it (mirroring Setup behavior).
func (e *worktreeWorkflowExecutor) ReconstructState(
	ctx context.Context,
	baseName prompt.BaseName,
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

	// Chdir into the worktree (mirroring Setup behavior).
	if err := os.Chdir(e.worktreePath); err != nil {
		return false, errors.Wrap(ctx, err, "chdir to worktree during resume")
	}

	return true, nil
}
