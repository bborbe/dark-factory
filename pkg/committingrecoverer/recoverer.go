// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package committingrecoverer

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/committing-recoverer.go --fake-name CommittingRecoverer . Recoverer

// Recoverer retries git commits for prompts left in `committing` status (e.g. after a daemon crash).
type Recoverer interface {
	// RecoverAll iterates all committing prompts; failures are logged and swallowed.
	RecoverAll(ctx context.Context)

	// Recover attempts to commit dirty work files and move a single prompt to completed.
	Recover(ctx context.Context, promptPath string) error
}

// PromptManager is the subset of prompt.Manager used by Recoverer.
// Defined here to avoid an import cycle with pkg/processor.
// processor.PromptManager satisfies this interface structurally.
type PromptManager interface {
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
	FindCommitting(ctx context.Context) ([]string, error)
	MoveToCompleted(ctx context.Context, path string) error
}

// NewRecoverer creates a Recoverer that retries git commits for prompts in committing state.
// The package-level git.HasDirtyFiles, git.CommitWithRetry, and git.CommitAll calls are
// used directly (not injected) because extracting a git-wrapper is out of scope.
func NewRecoverer(
	promptManager PromptManager,
	releaser git.Releaser,
	autoCompleter spec.AutoCompleter,
	completedDir string,
) Recoverer {
	return &recoverer{
		promptManager: promptManager,
		releaser:      releaser,
		autoCompleter: autoCompleter,
		completedDir:  completedDir,
	}
}

type recoverer struct {
	promptManager PromptManager
	releaser      git.Releaser
	autoCompleter spec.AutoCompleter
	completedDir  string
}

// RecoverAll iterates all committing prompts. Failures are logged and swallowed.
func (r *recoverer) RecoverAll(ctx context.Context) {
	paths, err := r.promptManager.FindCommitting(ctx)
	if err != nil {
		slog.Warn("failed to scan for committing prompts", "error", err)
		return
	}
	for _, promptPath := range paths {
		if ctx.Err() != nil {
			return
		}
		if err := r.Recover(ctx, promptPath); err != nil {
			slog.Error("git commit failed after all retries, will retry next cycle",
				"file", filepath.Base(promptPath), "error", err)
		}
	}
}

// Recover attempts to commit dirty work files and move a single prompt to completed.
// If dirty work files exist, they are committed first (the container's code changes).
// If no dirty files exist, the code was already committed — only the prompt move is needed.
func (r *recoverer) Recover(ctx context.Context, promptPath string) error {
	gitCtx := context.WithoutCancel(ctx)
	completedPath := filepath.Join(r.completedDir, filepath.Base(promptPath))

	pf, err := r.promptManager.Load(ctx, promptPath)
	if err != nil {
		return errors.Wrap(ctx, err, "load committing prompt")
	}
	title := pf.Title()
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(promptPath), ".md")
	}

	hasDirty, err := git.HasDirtyFiles(gitCtx)
	if err != nil {
		return errors.Wrap(ctx, err, "check dirty files")
	}

	if hasDirty {
		if err := git.CommitWithRetry(gitCtx, git.DefaultCommitBackoff, func(retryCtx context.Context) error {
			return git.CommitAll(retryCtx, title)
		}); err != nil {
			return errors.Wrap(ctx, err, "commit work files during recovery")
		}
		slog.Info(
			"committed work files during committing recovery",
			"file",
			filepath.Base(promptPath),
		)
	}

	for _, specID := range pf.Specs() {
		if err := r.autoCompleter.CheckAndComplete(ctx, specID); err != nil {
			slog.Warn("spec auto-complete failed", "spec", specID, "error", err)
		}
	}

	if err := r.promptManager.MoveToCompleted(ctx, promptPath); err != nil {
		return errors.Wrap(ctx, err, "move to completed during recovery")
	}

	if err := git.CommitWithRetry(gitCtx, git.DefaultCommitBackoff, func(retryCtx context.Context) error {
		return r.releaser.CommitCompletedFile(retryCtx, completedPath)
	}); err != nil {
		return errors.Wrap(ctx, err, "commit completed file during recovery")
	}

	slog.Info("git commit recovery succeeded", "file", filepath.Base(completedPath))
	return nil
}
