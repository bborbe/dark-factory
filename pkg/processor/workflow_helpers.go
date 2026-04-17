// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

// syncWithRemoteViaDeps fetches and merges from remote using deps.Brancher.
func syncWithRemoteViaDeps(ctx context.Context, deps WorkflowDeps) error {
	slog.Info("syncing with remote default branch")
	fetchCtx, fetchCancel := context.WithTimeout(ctx, 30*time.Second)
	defer fetchCancel()
	if err := deps.Brancher.Fetch(fetchCtx); err != nil {
		return errors.Wrap(ctx, err, "git fetch origin")
	}
	if err := deps.Brancher.MergeOriginDefault(ctx); err != nil {
		return errors.Wrap(ctx, err, "git merge origin default branch")
	}
	return nil
}

// buildPRBody constructs the PR body, appending an issue reference when one is set.
func buildPRBody(issue string) string {
	if issue != "" {
		return "Automated by dark-factory\n\nIssue: " + issue
	}
	return "Automated by dark-factory"
}

// moveToCompletedAndCommit moves the prompt to completed/, triggers spec auto-complete, and commits.
func moveToCompletedAndCommit(
	ctx context.Context,
	gitCtx context.Context,
	deps WorkflowDeps,
	pf *prompt.PromptFile,
	promptPath string,
	completedPath string,
) error {
	if err := deps.PromptManager.MoveToCompleted(ctx, promptPath); err != nil {
		return errors.Wrap(ctx, err, "move to completed")
	}
	slog.Info("moved to completed", "file", filepath.Base(promptPath))
	for _, specID := range pf.Specs() {
		if err := deps.AutoCompleter.CheckAndComplete(ctx, specID); err != nil {
			slog.Warn("spec auto-complete failed", "spec", specID, "error", err)
		}
	}
	if err := deps.Releaser.CommitCompletedFile(gitCtx, completedPath); err != nil {
		return errors.Wrap(ctx, err, "commit completed file")
	}
	return nil
}

// findOrCreatePR checks for an existing open PR on the branch, creates one if absent.
func findOrCreatePR(
	gitCtx context.Context,
	ctx context.Context,
	deps WorkflowDeps,
	branchName string,
	title string,
	issue string,
) (string, error) {
	prURL, err := deps.PRCreator.FindOpenPR(gitCtx, branchName)
	if err != nil {
		slog.Warn("failed to check for existing PR", "branch", branchName, "error", err)
	}
	if prURL != "" {
		slog.Info(
			"open PR already exists for branch — skipping creation",
			"branch",
			branchName,
			"url",
			prURL,
		)
		return prURL, nil
	}
	prURL, err = deps.PRCreator.Create(gitCtx, title, buildPRBody(issue))
	if err != nil {
		return "", errors.Wrap(ctx, err, "create pull request")
	}
	slog.Info("created PR", "url", prURL)
	return prURL, nil
}

// savePRURLToFrontmatter saves the PR URL to the prompt frontmatter (best-effort).
func savePRURLToFrontmatter(
	gitCtx context.Context,
	deps WorkflowDeps,
	completedPath string,
	prURL string,
) {
	if existingPF, err := deps.PromptManager.Load(gitCtx, completedPath); err == nil &&
		existingPF != nil && existingPF.PRURL() != "" {
		slog.Debug("pr-url already set, preserving existing value")
		return
	}
	if err := deps.PromptManager.SetPRURL(gitCtx, completedPath, prURL); err != nil {
		slog.Warn("failed to save PR URL to frontmatter", "error", err)
	}
}

// handleDirectWorkflow handles the direct commit workflow: commit, tag, push.
func handleDirectWorkflow(
	gitCtx context.Context,
	ctx context.Context,
	deps WorkflowDeps,
	title string,
	featureBranch string,
) error {
	if featureBranch != "" {
		if err := deps.Releaser.CommitOnly(gitCtx, title); err != nil {
			return errors.Wrap(ctx, err, "commit on feature branch")
		}
		slog.Info("committed changes on feature branch (no release)", "branch", featureBranch)
		return nil
	}
	if !deps.Releaser.HasChangelog(gitCtx) {
		if err := deps.Releaser.CommitOnly(gitCtx, title); err != nil {
			return errors.Wrap(ctx, err, "commit")
		}
		slog.Info("committed changes")
		return nil
	}
	if !deps.AutoRelease {
		if err := deps.Releaser.CommitOnly(gitCtx, title); err != nil {
			return errors.Wrap(ctx, err, "commit without release")
		}
		slog.Info("committed changes (autoRelease disabled, skipping tag)")
		return nil
	}
	bump := git.DetermineBumpFromChangelog(ctx, ".")
	nextVersion, err := deps.Releaser.GetNextVersion(gitCtx, bump)
	if err != nil {
		return errors.Wrap(ctx, err, "get next version")
	}
	if err := deps.Releaser.CommitAndRelease(gitCtx, bump); err != nil {
		return errors.Wrap(ctx, err, "commit and release")
	}
	slog.Info("committed and tagged", "version", nextVersion)
	return nil
}

// postMergeActions switches to default branch, pulls, and optionally releases.
func postMergeActions(
	gitCtx context.Context,
	ctx context.Context,
	deps WorkflowDeps,
	title string,
) error {
	defaultBranch, err := deps.Brancher.DefaultBranch(gitCtx)
	if err != nil {
		return errors.Wrap(ctx, err, "get default branch")
	}
	if err := deps.Brancher.Switch(gitCtx, defaultBranch); err != nil {
		return errors.Wrap(ctx, err, "switch to default branch")
	}
	if err := deps.Brancher.Pull(gitCtx); err != nil {
		return errors.Wrap(ctx, err, "pull default branch")
	}
	slog.Info("merged PR and updated default branch", "branch", defaultBranch)
	if deps.AutoRelease && deps.Releaser.HasChangelog(gitCtx) {
		if err := handleDirectWorkflow(gitCtx, ctx, deps, title, ""); err != nil {
			return errors.Wrap(ctx, err, "auto-release after merge")
		}
	}
	return nil
}

// handleAutoMergeForClone defers or immediately merges the PR based on remaining queued prompts.
func handleAutoMergeForClone(
	gitCtx context.Context,
	ctx context.Context,
	deps WorkflowDeps,
	pf *prompt.PromptFile,
	branchName string,
	promptPath string,
	completedPath string,
	prURL string,
	title string,
) error {
	hasMore, err := deps.PromptManager.HasQueuedPromptsOnBranch(ctx, branchName, promptPath)
	if err != nil {
		slog.Warn("failed to check remaining prompts on branch", "branch", branchName, "error", err)
	}
	if hasMore {
		slog.Info("more prompts queued on branch — deferring auto-merge", "branch", branchName)
		if err := moveToCompletedAndCommit(ctx, gitCtx, deps, pf, promptPath, completedPath); err != nil {
			return errors.Wrap(ctx, err, "move to completed and commit")
		}
		savePRURLToFrontmatter(gitCtx, deps, completedPath, prURL)
		return nil
	}
	if err := deps.PRMerger.WaitAndMerge(gitCtx, prURL); err != nil {
		return errors.Wrap(ctx, err, "wait and merge PR")
	}
	if err := moveToCompletedAndCommit(ctx, gitCtx, deps, pf, promptPath, completedPath); err != nil {
		return errors.Wrap(ctx, err, "move to completed and commit")
	}
	return postMergeActions(gitCtx, ctx, deps, title)
}

// handleAfterIsolatedCommit handles push + optional PR + prompt lifecycle for clone/worktree.
func handleAfterIsolatedCommit(
	gitCtx context.Context,
	ctx context.Context,
	deps WorkflowDeps,
	pf *prompt.PromptFile,
	branchName string,
	title string,
	promptPath string,
	completedPath string,
) error {
	ahead, err := deps.Brancher.CommitsAhead(gitCtx, branchName)
	if err != nil {
		return errors.Wrap(ctx, err, "count commits ahead")
	}
	if ahead == 0 {
		slog.Info("no new commits on branch — skipping push/PR", "branch", branchName)
		return moveToCompletedAndCommit(ctx, gitCtx, deps, pf, promptPath, completedPath)
	}
	if err := deps.Brancher.Push(gitCtx, branchName); err != nil {
		return errors.Wrap(ctx, err, "push branch")
	}
	if !deps.PR {
		return moveToCompletedAndCommit(ctx, gitCtx, deps, pf, promptPath, completedPath)
	}
	prURL, err := findOrCreatePR(gitCtx, ctx, deps, branchName, title, pf.Issue())
	if err != nil {
		return errors.Wrap(ctx, err, "find or create PR")
	}
	if deps.AutoMerge {
		return handleAutoMergeForClone(
			gitCtx,
			ctx,
			deps,
			pf,
			branchName,
			promptPath,
			completedPath,
			prURL,
			title,
		)
	}
	if deps.AutoReview {
		savePRURLToFrontmatter(gitCtx, deps, promptPath, prURL)
		if err := deps.PromptManager.SetStatus(ctx, promptPath, string(prompt.InReviewPromptStatus)); err != nil {
			return errors.Wrap(ctx, err, "set in_review status")
		}
		slog.Info("PR created, waiting for review", "url", prURL)
		return nil
	}
	if err := moveToCompletedAndCommit(ctx, gitCtx, deps, pf, promptPath, completedPath); err != nil {
		return errors.Wrap(ctx, err, "move to completed and commit")
	}
	savePRURLToFrontmatter(gitCtx, deps, completedPath, prURL)
	return nil
}
