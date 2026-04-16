// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"

	"github.com/bborbe/errors"

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
func (e *directWorkflowExecutor) Setup(ctx context.Context, _ string, _ *prompt.PromptFile) error {
	return syncWithRemoteViaDeps(ctx, e.deps)
}

// CleanupOnError is a no-op for the direct workflow.
func (e *directWorkflowExecutor) CleanupOnError(_ context.Context) {}

// Complete moves the prompt to completed and runs the direct commit workflow.
func (e *directWorkflowExecutor) Complete(
	gitCtx, ctx context.Context,
	pf *prompt.PromptFile,
	title, promptPath, completedPath string,
) error {
	if err := moveToCompletedAndCommit(ctx, gitCtx, e.deps, pf, promptPath, completedPath); err != nil {
		return errors.Wrap(ctx, err, "move to completed and commit")
	}
	return handleDirectWorkflow(gitCtx, ctx, e.deps, title, "")
}

// ReconstructState always returns true for the direct workflow (no isolated directory).
func (e *directWorkflowExecutor) ReconstructState(
	_ context.Context,
	_ string,
	_ *prompt.PromptFile,
) (bool, error) {
	return true, nil
}
