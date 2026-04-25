// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate go run -mod=mod github.com/maxbrunsfeld/counterfeiter/v6 -generate

package failurehandler

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/report"
)

//counterfeiter:generate -o ../../mocks/failure-handler.go --fake-name FailureHandler . Handler

// Handler handles prompt-failure scenarios: deciding whether to retry or permanently fail a prompt.
type Handler interface {
	// Handle is called when processPrompt returns an error.
	// If the context is cancelled it returns an error to propagate shutdown.
	// If the prompt was already moved to completed/ (post-execution failure) it returns an error to stop the daemon.
	// Otherwise it retries or marks the prompt failed and returns nil so the loop continues.
	Handle(ctx context.Context, promptPath string, err error) error
	// NotifyFromReport checks the completion report in logFile and fires a partial notification
	// if the report status is "partial".
	NotifyFromReport(ctx context.Context, logFile string, promptPath string)
}

//counterfeiter:generate -o ../../mocks/failurehandler-prompt-manager.go --fake-name FailureHandlerPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager used by the failure handler.
type PromptManager interface {
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
}

// NewHandler creates a new Handler.
func NewHandler(
	promptManager PromptManager,
	n notifier.Notifier,
	completedDir string,
	projectName string,
	autoRetryLimit int,
) Handler {
	return &handler{
		promptManager:  promptManager,
		notifier:       n,
		completedDir:   completedDir,
		projectName:    projectName,
		autoRetryLimit: autoRetryLimit,
	}
}

type handler struct {
	promptManager  PromptManager
	notifier       notifier.Notifier
	completedDir   string
	projectName    string
	autoRetryLimit int
}

// Handle is called when processPrompt returns an error.
func (h *handler) Handle(ctx context.Context, promptPath string, err error) error {
	if ctx.Err() != nil {
		slog.Info("daemon shutting down, prompt stays executing", "file", filepath.Base(promptPath))
		return errors.Wrap(ctx, err, "prompt failed")
	}
	if stopErr := h.checkPostExecutionFailure(ctx, promptPath, err); stopErr != nil {
		return stopErr
	}
	h.handlePromptFailure(ctx, promptPath, err)
	return nil
}

// checkPostExecutionFailure returns a non-nil error when the prompt file is gone from its
// in-progress path but found in completed/ — indicating the container succeeded yet a
// post-execution git step failed. Returning an error stops the daemon loop so uncommitted
// code changes are not overwritten by the next prompt's git fetch/merge.
// Returns nil when the file still exists at path (normal pre-execution failure).
func (h *handler) checkPostExecutionFailure(
	ctx context.Context,
	path string,
	origErr error,
) error {
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		return nil
	}
	completedFilePath := filepath.Join(h.completedDir, filepath.Base(path))
	if _, cStatErr := os.Stat(completedFilePath); cStatErr != nil {
		return nil
	}
	slog.Error(
		"post-execution failure, prompt already moved to completed — stopping daemon",
		"file", filepath.Base(path),
		"error", origErr,
	)
	return errors.Wrap(ctx, origErr, "post-execution git failure, manual intervention required")
}

// handlePromptFailure decides whether to retry or fail the prompt.
// Re-queuing increments retryCount and calls MarkApproved; exhausted retries call MarkFailed.
func (h *handler) handlePromptFailure(ctx context.Context, path string, err error) {
	slog.Error("prompt failed", "file", filepath.Base(path), "error", err)

	pf, loadErr := h.promptManager.Load(ctx, path)
	if loadErr != nil {
		slog.Error("failed to load prompt for failure handling", "error", loadErr)
		return
	}

	reason := err.Error()
	pf.SetLastFailReason(reason)

	if h.autoRetryLimit > 0 && pf.RetryCount() < h.autoRetryLimit {
		// Re-queue with incremented retry count
		pf.Frontmatter.RetryCount++
		pf.MarkApproved()
		if saveErr := pf.Save(ctx); saveErr != nil {
			slog.Error("failed to save prompt for retry", "error", saveErr)
			// Fall through to MarkFailed
			pf.MarkFailed()
			if saveErr2 := pf.Save(ctx); saveErr2 != nil {
				slog.Error("failed to save failed prompt", "error", saveErr2)
			}
			h.notifyFailed(ctx, path)
			return
		}
		slog.Info("prompt re-queued for retry",
			"file", filepath.Base(path),
			"retryCount", pf.RetryCount(),
			"autoRetryLimit", h.autoRetryLimit)
		return
	}

	// Retries exhausted or autoRetryLimit == 0 — mark failed
	pf.MarkFailed()
	if saveErr := pf.Save(ctx); saveErr != nil {
		slog.Error("failed to set failed status", "error", saveErr)
	}
	h.notifyFailed(ctx, path)
}

// notifyFailed fires a notification for a failed prompt.
func (h *handler) notifyFailed(ctx context.Context, path string) {
	_ = h.notifier.Notify(ctx, notifier.Event{
		ProjectName: h.projectName,
		EventType:   "prompt_failed",
		PromptName:  filepath.Base(path),
	})
}

// NotifyFromReport checks the completion report in logFile and fires a partial notification
// if the report status is "partial".
func (h *handler) NotifyFromReport(ctx context.Context, logFile string, promptPath string) {
	completionReport, err := report.ParseFromLog(ctx, logFile)
	if err != nil || completionReport == nil {
		return
	}
	if completionReport.Status == "partial" {
		_ = h.notifier.Notify(ctx, notifier.Event{
			ProjectName: h.projectName,
			EventType:   "prompt_partial",
			PromptName:  filepath.Base(promptPath),
		})
	}
}
