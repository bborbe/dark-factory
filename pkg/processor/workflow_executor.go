// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
	"github.com/bborbe/dark-factory/pkg/version"
)

//counterfeiter:generate -o ../../mocks/workflow_executor.go --fake-name WorkflowExecutor . WorkflowExecutor

// WorkflowExecutor handles the git lifecycle for a single prompt execution.
// It encapsulates the pre-execution environment setup and post-execution git
// operations for one workflow variant (direct, branch, clone, or worktree).
//
// Implementations are stateful: Setup stores paths and branches internally so
// that CleanupOnError and Complete can use them without the caller tracking
// workflowState.
type WorkflowExecutor interface {
	// Setup prepares the execution environment before the YOLO container runs.
	// For clone/worktree: creates the isolated directory and chdirs into it.
	// For branch: creates or switches to the feature branch in-place.
	// For direct: no-op.
	// Returns a wrapped error on failure; partial setup is cleaned up before returning.
	Setup(ctx context.Context, baseName BaseName, pf *prompt.PromptFile) error

	// CleanupOnError undoes any environment setup performed by Setup when
	// execution or post-execution fails. Idempotent — safe to call if Setup was
	// not called or has already been cleaned up. No-op for direct and branch
	// executors where no isolated directory exists.
	CleanupOnError(ctx context.Context)

	// Complete performs all post-execution git operations after the YOLO container
	// exits successfully: commit, chdir back, cleanup isolation, push, optional PR
	// creation/merge, and moving the prompt file to completedPath.
	//
	// gitCtx is a non-cancellable context (context.WithoutCancel) for git ops.
	// ctx is the normal request context (used for prompt-manager calls and error wrapping).
	// completedPath is the destination path — the prompt has NOT been moved yet when
	// Complete is called; each implementation calls moveToCompleted internally.
	Complete(
		gitCtx context.Context,
		ctx context.Context,
		pf *prompt.PromptFile,
		title, promptPath, completedPath string,
	) error

	// ReconstructState restores internal state for a prompt being resumed after a
	// daemon restart. Returns canResume=false if the isolated directory no longer
	// exists (caller resets the prompt to approved). Returns an error only for
	// unexpected filesystem failures.
	ReconstructState(
		ctx context.Context,
		baseName BaseName,
		pf *prompt.PromptFile,
	) (canResume bool, err error)
}

// WorkflowDeps holds all dependencies that WorkflowExecutor implementations may need.
// Factory functions populate only the fields required by the selected implementation;
// unused fields are nil and must not be dereferenced by implementations that do not
// need them.
type WorkflowDeps struct {
	ProjectName   ProjectName
	PromptManager PromptManager
	AutoCompleter spec.AutoCompleter
	Releaser      git.Releaser
	VersionGetter version.Getter
	Brancher      git.Brancher
	PRCreator     git.PRCreator
	Cloner        git.Cloner
	Worktreer     git.Worktreer
	PRMerger      git.PRMerger
	PR            bool
	AutoMerge     bool
	AutoReview    bool
	AutoRelease   bool
}
