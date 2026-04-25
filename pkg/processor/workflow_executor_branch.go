// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	"log/slog"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

// branchWorkflowExecutor handles WorkflowBranch — in-place branch switching.
type branchWorkflowExecutor struct {
	deps WorkflowDeps
	// state set during Setup
	inPlaceBranch        string
	inPlaceDefaultBranch string
}

// NewBranchWorkflowExecutor creates a WorkflowExecutor for the branch workflow.
func NewBranchWorkflowExecutor(deps WorkflowDeps) WorkflowExecutor {
	return &branchWorkflowExecutor{deps: deps}
}

// Setup syncs with remote, then optionally switches to the feature branch from the prompt frontmatter.
func (e *branchWorkflowExecutor) Setup(
	ctx context.Context,
	_ BaseName,
	pf *prompt.PromptFile,
) error {
	if err := syncWithRemoteViaDeps(ctx, e.deps); err != nil {
		return err
	}
	branch := pf.Branch()
	if branch == "" {
		// No branch specified — run directly on current branch.
		return nil
	}
	return e.setupInPlaceBranch(ctx, branch)
}

// setupInPlaceBranch switches to the given branch in-place.
func (e *branchWorkflowExecutor) setupInPlaceBranch(ctx context.Context, branch string) error {
	clean, err := e.deps.Brancher.IsClean(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "check working tree")
	}
	if !clean {
		return errors.Errorf(ctx, "working tree is not clean; cannot switch to branch %q", branch)
	}
	defaultBranch, err := e.deps.Brancher.DefaultBranch(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "get default branch")
	}
	e.inPlaceDefaultBranch = defaultBranch
	e.inPlaceBranch = branch

	if err := e.deps.Brancher.FetchAndVerifyBranch(ctx, branch); err == nil {
		if err := e.deps.Brancher.Switch(ctx, branch); err != nil {
			return errors.Wrap(ctx, err, "switch to existing branch")
		}
	} else {
		if err := e.deps.Brancher.CreateAndSwitch(ctx, branch); err != nil {
			return errors.Wrap(ctx, err, "create and switch to branch")
		}
	}
	slog.Info("switched to branch for in-place execution", "branch", branch)
	return nil
}

// CleanupOnError is a no-op for the branch workflow — no isolated directory to remove.
func (e *branchWorkflowExecutor) CleanupOnError(_ context.Context) {}

// Complete moves prompt to completed, commits, restores the default branch, and handles PR/merge.
func (e *branchWorkflowExecutor) Complete(
	gitCtx, ctx context.Context,
	pf *prompt.PromptFile,
	title, promptPath, completedPath string,
) error {
	featureBranch := e.inPlaceBranch

	if err := moveToCompletedAndCommit(ctx, gitCtx, e.deps, pf, promptPath, completedPath); err != nil {
		e.restoreDefaultBranch(ctx)
		return errors.Wrap(ctx, err, "move to completed and commit")
	}

	if err := handleDirectWorkflow(gitCtx, ctx, e.deps, title, featureBranch); err != nil {
		e.restoreDefaultBranch(ctx)
		return errors.Wrap(ctx, err, "handle direct workflow")
	}
	e.restoreDefaultBranch(ctx)

	if featureBranch == "" {
		return nil
	}

	if e.deps.PR {
		return e.handleBranchPRCompletion(gitCtx, ctx, pf, featureBranch, title, completedPath)
	}
	return e.handleBranchCompletion(gitCtx, ctx, promptPath, title, featureBranch)
}

// ReconstructState always returns true for the branch workflow (no isolated directory).
func (e *branchWorkflowExecutor) ReconstructState(
	ctx context.Context,
	_ BaseName,
	pf *prompt.PromptFile,
) (bool, error) {
	// Restore in-place branch state from frontmatter if branch was set.
	branch := pf.Branch()
	if branch != "" {
		e.inPlaceBranch = branch
		defaultBranch, err := e.deps.Brancher.DefaultBranch(ctx)
		if err != nil {
			return false, errors.Wrap(ctx, err, "get default branch for resume")
		}
		e.inPlaceDefaultBranch = defaultBranch
	}
	return true, nil
}

// restoreDefaultBranch switches back to the default branch after in-place execution.
func (e *branchWorkflowExecutor) restoreDefaultBranch(ctx context.Context) {
	if e.inPlaceDefaultBranch == "" {
		return
	}
	if err := e.deps.Brancher.Switch(ctx, e.inPlaceDefaultBranch); err != nil {
		slog.Warn(
			"failed to restore default branch",
			"branch",
			e.inPlaceDefaultBranch,
			"error",
			err,
		)
	} else {
		slog.Info("restored default branch", "branch", e.inPlaceDefaultBranch)
	}
}

// handleBranchCompletion checks if this was the last prompt on a feature branch.
// If so, merges the branch to default and triggers a release.
func (e *branchWorkflowExecutor) handleBranchCompletion(
	gitCtx context.Context,
	ctx context.Context,
	promptPath string,
	title string,
	featureBranch string,
) error {
	hasMore, err := e.deps.PromptManager.HasQueuedPromptsOnBranch(ctx, featureBranch, promptPath)
	if err != nil {
		slog.Warn(
			"failed to check remaining prompts on branch",
			"branch",
			featureBranch,
			"error",
			err,
		)
		return nil // non-fatal: skip merge, let next run re-check
	}
	if hasMore {
		slog.Info("more prompts queued on branch — skipping merge", "branch", featureBranch)
		return nil
	}
	slog.Info("last prompt on branch — merging to default and releasing", "branch", featureBranch)
	if err := e.deps.Brancher.MergeToDefault(gitCtx, featureBranch); err != nil {
		return errors.Wrap(ctx, err, "merge feature branch to default")
	}
	if err := handleDirectWorkflow(gitCtx, ctx, e.deps, title, ""); err != nil {
		return errors.Wrap(ctx, err, "release after branch merge")
	}
	return nil
}

// handleBranchPRCompletion pushes the feature branch and creates a PR after an in-place commit.
func (e *branchWorkflowExecutor) handleBranchPRCompletion(
	gitCtx context.Context,
	ctx context.Context,
	pf *prompt.PromptFile,
	featureBranch string,
	title string,
	completedPath string,
) error {
	if err := e.deps.Brancher.Push(gitCtx, featureBranch); err != nil {
		return errors.Wrap(ctx, err, "push feature branch")
	}
	prURL, err := findOrCreatePR(gitCtx, ctx, e.deps, featureBranch, title, pf.Issue())
	if err != nil {
		return errors.Wrap(ctx, err, "find or create PR")
	}
	if e.deps.AutoMerge {
		hasMore, err := e.deps.PromptManager.HasQueuedPromptsOnBranch(
			ctx,
			featureBranch,
			completedPath,
		)
		if err != nil {
			slog.Warn(
				"failed to check remaining prompts on branch",
				"branch",
				featureBranch,
				"error",
				err,
			)
		}
		if hasMore {
			slog.Info(
				"more prompts queued on branch — deferring auto-merge",
				"branch",
				featureBranch,
			)
			savePRURLToFrontmatter(gitCtx, e.deps, completedPath, prURL)
			return nil
		}
		if err := e.deps.PRMerger.WaitAndMerge(gitCtx, prURL); err != nil {
			return errors.Wrap(ctx, err, "wait and merge PR")
		}
		return postMergeActions(gitCtx, ctx, e.deps, title)
	}
	savePRURLToFrontmatter(gitCtx, e.deps, completedPath, prURL)
	return nil
}
