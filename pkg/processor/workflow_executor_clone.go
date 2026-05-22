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

// cloneWorkflowExecutor handles WorkflowClone.
type cloneWorkflowExecutor struct {
	deps WorkflowDeps
	// state set during Setup
	branchName  string
	clonePath   string
	originalDir string
	cleanedUp   bool
}

// NewCloneWorkflowExecutor creates a WorkflowExecutor for the clone workflow.
func NewCloneWorkflowExecutor(deps WorkflowDeps) WorkflowExecutor {
	return &cloneWorkflowExecutor{deps: deps}
}

// Setup syncs with remote, creates a clone, and chdirs into it.
func (e *cloneWorkflowExecutor) Setup(
	ctx context.Context,
	baseName prompt.BaseName,
	pf *prompt.PromptFile,
) error {
	if err := syncWithRemoteViaDeps(ctx, e.deps); err != nil {
		return err
	}

	if branch := pf.Branch(); branch != "" {
		e.branchName = branch
	} else {
		e.branchName = "dark-factory/" + string(baseName)
	}
	e.clonePath = filepath.Join(
		os.TempDir(),
		"dark-factory",
		string(e.deps.ProjectName)+"-"+string(baseName),
	)

	originalDir, err := os.Getwd()
	if err != nil {
		return errors.Wrap(ctx, err, "get current directory")
	}
	e.originalDir = originalDir

	if err := e.deps.Cloner.Clone(ctx, originalDir, e.clonePath, e.branchName); err != nil {
		return errors.Wrap(ctx, err, "clone repo")
	}

	if err := os.Chdir(e.clonePath); err != nil {
		// Remove clone since we couldn't chdir into it
		if removeErr := e.deps.Cloner.Remove(ctx, e.clonePath); removeErr != nil {
			slog.Warn(
				"failed to remove clone after chdir error",
				"path",
				e.clonePath,
				"error",
				removeErr,
			)
		}
		return errors.Wrap(ctx, err, "chdir to clone")
	}

	return nil
}

// CleanupOnError restores the original directory and removes the clone on error.
func (e *cloneWorkflowExecutor) CleanupOnError(ctx context.Context) {
	if e.cleanedUp {
		return
	}
	if e.originalDir != "" {
		if err := os.Chdir(e.originalDir); err != nil {
			slog.Warn("failed to chdir back to original directory on error", "error", err)
		}
	}
	if e.clonePath != "" {
		if err := e.deps.Cloner.Remove(ctx, e.clonePath); err != nil {
			slog.Warn("failed to remove clone on error", "path", e.clonePath, "error", err)
		}
	}
}

// Complete moves the prompt to completed/, commits in the clone, pushes, chdirs back,
// removes the clone, then handles push/PR via handleAfterIsolatedCommit.
// No rollback on failure: the clone is discarded on cleanup; original prompt path untouched.
func (e *cloneWorkflowExecutor) Complete(
	gitCtx, ctx context.Context,
	pf *prompt.PromptFile,
	title, promptPath, completedPath string,
) error {
	// Move prompt to completed/ inside the clone (sets status: completed, physically moves the file).
	if err := e.deps.PromptManager.MoveToCompleted(ctx, promptPath); err != nil {
		return errors.Wrap(ctx, err, "move to completed")
	}
	slog.Info("moved to completed", "file", filepath.Base(promptPath))

	// Single combined commit: work changes + prompt move.
	if err := e.deps.Releaser.CommitOnly(gitCtx, title); err != nil {
		return errors.Wrap(ctx, err, "commit changes")
	}

	// Push from inside the clone while the feature branch is still locally visible.
	// The parent repo has never seen this branch; pushing here ensures it exists on
	// origin before the clone is removed and handleAfterIsolatedCommit runs
	// CommitsAhead against the parent repo.
	if err := e.deps.Brancher.Push(gitCtx, e.branchName); err != nil {
		return errors.Wrap(ctx, err, "push branch from clone")
	}

	if err := os.Chdir(e.originalDir); err != nil {
		return errors.Wrap(ctx, err, "chdir back to original directory")
	}

	// no rollback needed: clone is discarded on cleanup; original prompt path untouched
	if err := e.deps.Cloner.Remove(gitCtx, e.clonePath); err != nil {
		slog.Warn("failed to remove clone", "path", e.clonePath, "error", err)
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

// ReconstructState checks if the clone directory still exists for resume.
func (e *cloneWorkflowExecutor) ReconstructState(
	ctx context.Context,
	baseName prompt.BaseName,
	pf *prompt.PromptFile,
) (bool, error) {
	clonePath := filepath.Join(
		os.TempDir(),
		"dark-factory",
		string(e.deps.ProjectName)+"-"+string(baseName),
	)
	if _, err := os.Stat(clonePath); err != nil {
		return false, nil // clone missing — signal reset-to-approved
	}
	if branch := pf.Branch(); branch != "" {
		e.branchName = branch
	} else {
		e.branchName = "dark-factory/" + string(baseName)
	}
	e.clonePath = clonePath
	originalDir, err := os.Getwd()
	if err != nil {
		return false, errors.Wrap(ctx, err, "get working directory for resume")
	}
	e.originalDir = originalDir
	return true, nil
}
