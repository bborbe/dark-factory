// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/bborbe/errors"

	log "github.com/bborbe/dark-factory/pkg/log"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

// syncWithRemoteViaDeps fetches and merges from remote using deps.Brancher.
func syncWithRemoteViaDeps(ctx context.Context, deps WorkflowDeps) error {
	log.From(ctx).Info("syncing with remote default branch")
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

// buildPRBody constructs the PR body from the prompt file's summary, spec links, and issue reference.
func buildPRBody(pf *prompt.PromptFile) string {
	var parts []string
	if summary := pf.Summary(); summary != "" {
		parts = append(parts, summary)
	}
	var metaLines []string
	for _, spec := range pf.Specs() {
		metaLines = append(metaLines, "Spec: "+spec)
	}
	if issue := pf.Issue(); issue != "" {
		metaLines = append(metaLines, "Issue: "+issue)
	}
	if len(metaLines) > 0 {
		parts = append(parts, strings.Join(metaLines, "\n"))
	}
	parts = append(parts, "Automated by dark-factory")
	return strings.Join(parts, "\n\n")
}

// findOrCreatePR checks for an existing open PR on the branch, creates one if absent.
func findOrCreatePR(
	gitCtx context.Context,
	ctx context.Context,
	deps WorkflowDeps,
	branchName string,
	title string,
	pf *prompt.PromptFile,
) (string, error) {
	prURL, err := deps.PRCreator.FindOpenPR(gitCtx, branchName)
	if err != nil {
		log.From(ctx).Warn("failed to check for existing PR", "branch", branchName, "error", err)
	}
	if prURL != "" {
		log.From(ctx).Info(
			"open PR already exists for branch — skipping creation",
			"branch", branchName,
			"url", prURL,
		)
		return prURL, nil
	}
	prURL, err = deps.PRCreator.Create(gitCtx, title, buildPRBody(pf), branchName)
	if err != nil {
		return "", errors.Wrap(ctx, err, "create pull request")
	}
	log.From(ctx).Info("created PR", "url", prURL)
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
		log.From(gitCtx).Debug("pr-url already set, preserving existing value")
		return
	}
	if err := deps.PromptManager.SetPRURL(gitCtx, completedPath, prURL); err != nil {
		log.From(gitCtx).Warn("failed to save PR URL to frontmatter", "error", err)
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
		log.From(ctx).Info("committed changes on feature branch (no release)",
			"branch", featureBranch,
			"workflow_step", "commit",
		)
		return nil
	}
	if !deps.Releaser.HasChangelog(gitCtx) {
		if err := deps.Releaser.CommitOnly(gitCtx, title); err != nil {
			return errors.Wrap(ctx, err, "commit")
		}
		log.From(ctx).Info("committed changes", "workflow_step", "commit")
		return nil
	}
	if !deps.AutoRelease {
		if err := deps.Releaser.CommitOnly(gitCtx, title); err != nil {
			return errors.Wrap(ctx, err, "commit without release")
		}
		log.From(ctx).
			Info("committed changes (autoRelease disabled, skipping tag)", "workflow_step", "commit")
		return nil
	}
	bump := deps.Releaser.DetermineBump(ctx)
	nextVersion, err := deps.Releaser.GetNextVersion(gitCtx, bump)
	if err != nil {
		return errors.Wrap(ctx, err, "get next version")
	}
	if err := deps.Releaser.CommitAndRelease(gitCtx, bump); err != nil {
		return errors.Wrap(ctx, err, "commit and release")
	}
	log.From(ctx).Info("committed and tagged", "version", nextVersion, "workflow_step", "commit")
	return nil
}

// PostMergeActions switches to default branch, pulls, and optionally releases.
func PostMergeActions(
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
	log.From(ctx).
		Info("merged PR and updated default branch", "branch", defaultBranch, "workflow_step", "push")
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
	branchName string,
	promptPath string,
	completedPath string,
	prURL string,
	title string,
) error {
	hasMore, err := deps.PromptManager.HasQueuedPromptsOnBranch(ctx, branchName, promptPath)
	if err != nil {
		log.From(ctx).
			Warn("failed to check remaining prompts on branch", "branch", branchName, "error", err)
	}
	if hasMore {
		log.From(ctx).
			Info("more prompts queued on branch — deferring auto-merge", "branch", branchName)
		savePRURLToFrontmatter(gitCtx, deps, completedPath, prURL)
		return nil
	}
	if err := deps.PRMerger.WaitAndMerge(gitCtx, prURL); err != nil {
		return errors.Wrap(ctx, err, "wait and merge PR")
	}
	// The feature branch (including the completed prompt file) is merged into default
	// via PostMergeActions; no separate move/commit needed.
	return PostMergeActions(gitCtx, ctx, deps, title)
}

// syncPromptFileToOriginalRepo mirrors the in-progress → completed rename into the
// ORIGINAL repo AFTER the combined commit has already been pushed from an isolated
// clone/worktree. It is filesystem-only: no git calls, no remote operations.
//
// Idempotent:
//   - If the file is already at completedPath, this is a no-op (debug log).
//   - If the file is missing at promptPath AND missing at completedPath, the original
//     repo's view has truly diverged from the pushed remote; the function returns a
//     wrapped "clone-sync-mismatch" error so the caller can WARN.
//
// On rename failure, returns the wrapped error so the caller can log
// "clone-sync-mismatch" and continue with success-with-warning semantics.
func syncPromptFileToOriginalRepo(
	ctx context.Context,
	promptMgr PromptManager,
	promptPath, completedPath string,
) error {
	// Already mirrored — idempotent no-op.
	if _, err := os.Stat(completedPath); err == nil {
		log.From(ctx).Debug("sync-already-at-completed", "dir", completedPath)
		return nil
	}

	// File not yet mirrored — perform the rename via MoveToCompleted.
	if _, err := os.Stat(promptPath); err == nil {
		if err := promptMgr.MoveToCompleted(ctx, promptPath); err != nil {
			return errors.Wrap(ctx, err, "sync prompt file to original repo")
		}
		return nil
	}

	// Both source and destination absent — true divergence.
	return errors.Errorf(
		ctx,
		"clone-sync-mismatch: prompt absent at both %s and %s",
		promptPath,
		completedPath,
	)
}

// handleAfterIsolatedCommit handles push + optional PR + prompt lifecycle for clone/worktree.
// The prompt file move was already committed as part of the combined commit in the
// clone/worktree Complete method.
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
	// Fetch the branch as a local ref so that CommitsAhead can resolve the bare
	// branch name via git rev-list. For the clone workflow the branch was just
	// pushed from inside the clone; for worktree the local ref already exists and
	// this is a fast no-op (or a silent skip if origin does not have it yet).
	if err := deps.Brancher.FetchBranch(gitCtx, branchName); err != nil {
		return errors.Wrap(ctx, err, "fetch branch before commit count")
	}
	ahead, err := deps.Brancher.CommitsAhead(gitCtx, branchName)
	if err != nil {
		return errors.Wrap(ctx, err, "count commits ahead")
	}
	if ahead == 0 {
		// Unexpected: clone/worktree completed a commit but parent sees zero ahead.
		// The prompt file move was already committed in the isolated environment.
		log.From(ctx).Warn("after-isolated-commit-no-ahead-commits", "branch", branchName)
		return nil
	}
	if err := deps.Brancher.Push(gitCtx, branchName); err != nil {
		return errors.Wrap(ctx, err, "push branch")
	}
	if !deps.PR {
		// No PR: prompt move was already committed in the isolated environment.
		return nil
	}
	prURL, err := findOrCreatePR(gitCtx, ctx, deps, branchName, title, pf)
	if err != nil {
		return errors.Wrap(ctx, err, "find or create PR")
	}
	if deps.AutoMerge {
		return handleAutoMergeForClone(
			gitCtx,
			ctx,
			deps,
			branchName,
			promptPath,
			completedPath,
			prURL,
			title,
		)
	}
	savePRURLToFrontmatter(gitCtx, deps, completedPath, prURL)
	return nil
}
