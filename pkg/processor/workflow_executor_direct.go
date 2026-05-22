// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

// directWorkflowExecutor handles WorkflowDirect.
type directWorkflowExecutor struct {
	deps WorkflowDeps
}

// NewDirectWorkflowExecutor creates a WorkflowExecutor for the direct workflow.
func NewDirectWorkflowExecutor(deps WorkflowDeps) WorkflowExecutor {
	return &directWorkflowExecutor{deps: deps}
}

// Setup syncs with remote before execution.
func (e *directWorkflowExecutor) Setup(
	ctx context.Context,
	_ prompt.BaseName,
	_ *prompt.PromptFile,
) error {
	return syncWithRemoteViaDeps(ctx, e.deps)
}

// CleanupOnError is a no-op for the direct workflow.
func (e *directWorkflowExecutor) CleanupOnError(_ context.Context) {}

// Complete sets the prompt to committing, commits all work, then moves and commits the prompt file.
// If any git operation fails after retries, the prompt stays committing for the next daemon cycle.
func (e *directWorkflowExecutor) Complete(
	gitCtx, ctx context.Context,
	pf *prompt.PromptFile,
	title, promptPath, completedPath string,
) error {
	// Transition to committing BEFORE any git operations.
	// The file stays in in-progress/ until the commit of the prompt move succeeds.
	pf.MarkCommitting()
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save committing status")
	}

	if err := e.completeCommit(gitCtx, ctx, pf, title, promptPath, completedPath); err != nil {
		slog.Error("git commit failed after all retries, will retry next cycle",
			"file", filepath.Base(promptPath), "error", err)
		return nil // do NOT crash the daemon
	}
	return nil
}

// completeCommit performs the single-phase git commit for the direct workflow.
// The prompt file is moved to completed/ before the work commit; both land in one commit.
// Uses CommitWithRetry for the work commit. Returns an error if the phase exhausts all retries.
func (e *directWorkflowExecutor) completeCommit(
	gitCtx, ctx context.Context,
	pf *prompt.PromptFile,
	title, promptPath, completedPath string,
) error {
	// Move prompt to completed/ before the work commit (sets status: completed, physically moves the file).
	if err := e.deps.PromptManager.MoveToCompleted(ctx, promptPath); err != nil {
		return errors.Wrap(ctx, err, "move to completed")
	}
	slog.Info("moved to completed", "file", filepath.Base(promptPath))

	// Commit all code changes with retry. If the commit fails, roll the prompt file back to in-progress/ first.
	if err := git.CommitWithRetry(gitCtx, git.DefaultCommitBackoff, func(retryCtx context.Context) error {
		return handleDirectWorkflow(retryCtx, ctx, e.deps, title, "")
	}); err != nil {
		if rollbackErr := e.deps.PromptManager.RollbackMoveToCompleted(ctx, completedPath, e.deps.FileMover); rollbackErr != nil {
			slog.Error("rollback after commit failure failed", "error", rollbackErr)
		}
		return errors.Wrap(ctx, err, "handle direct workflow (rolled back move)")
	}

	// Auto-complete specs (best-effort, non-blocking).
	// Must run AFTER the successful commit so allLinkedPromptsCompleted can see this
	// prompt in the completed dir.
	for _, specID := range pf.Specs() {
		if err := e.deps.AutoCompleter.CheckAndComplete(ctx, specID); err != nil {
			slog.Warn("spec auto-complete failed", "spec", specID, "error", err)
		}
	}

	// Push the branch when autoRelease is enabled.
	if e.deps.AutoRelease {
		if err := git.CommitWithRetry(gitCtx, git.DefaultCommitBackoff, func(retryCtx context.Context) error {
			return e.deps.Releaser.PushBranch(retryCtx)
		}); err != nil {
			return errors.Wrap(ctx, err, "push branch")
		}
	}

	return nil
}

// ReconstructState always returns true for the direct workflow (no isolated directory).
func (e *directWorkflowExecutor) ReconstructState(
	_ context.Context,
	_ prompt.BaseName,
	_ *prompt.PromptFile,
) (bool, error) {
	return true, nil
}
