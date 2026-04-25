// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package promptresumer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/completionreport"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/project"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/prompt-resumer.go --fake-name Resumer . Resumer

// Resumer reattaches to and drives executing prompts to completion.
// Used once at daemon startup before the event loop.
type Resumer interface {
	ResumeAll(ctx context.Context) error
}

// PromptManager is the subset of prompt.Manager used by Resumer.
type PromptManager interface {
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
}

// WorkflowExecutor is the subset of processor.WorkflowExecutor used by Resumer.
type WorkflowExecutor interface {
	ReconstructState(
		ctx context.Context,
		baseName prompt.BaseName,
		pf *prompt.PromptFile,
	) (bool, error)
	Complete(
		gitCtx context.Context,
		ctx context.Context,
		pf *prompt.PromptFile,
		title, promptPath, completedPath string,
	) error
}

// FailureNotifier is the subset of failurehandler.Handler used by Resumer.
type FailureNotifier interface {
	NotifyFromReport(ctx context.Context, logFile string, promptPath string)
}

// NewResumer creates a Resumer that reattaches to executing prompts on daemon startup.
func NewResumer(
	promptManager PromptManager,
	exec executor.Executor,
	workflowExecutor WorkflowExecutor,
	completionReportValidator completionreport.Validator,
	failureNotifier FailureNotifier,
	queueDir string,
	completedDir string,
	logDir string,
	projectName project.Name,
	maxPromptDuration time.Duration,
) Resumer {
	return &resumer{
		promptManager:             promptManager,
		executor:                  exec,
		workflowExecutor:          workflowExecutor,
		completionReportValidator: completionReportValidator,
		failureNotifier:           failureNotifier,
		queueDir:                  queueDir,
		completedDir:              completedDir,
		logDir:                    logDir,
		projectName:               projectName,
		maxPromptDuration:         maxPromptDuration,
	}
}

type resumer struct {
	promptManager             PromptManager
	executor                  executor.Executor
	workflowExecutor          WorkflowExecutor
	completionReportValidator completionreport.Validator
	failureNotifier           FailureNotifier
	queueDir                  string
	completedDir              string
	logDir                    string
	projectName               project.Name
	maxPromptDuration         time.Duration
}

// ResumeAll scans the queue directory and reattaches to any prompts in "executing" state.
func (r *resumer) ResumeAll(ctx context.Context) error {
	entries, err := os.ReadDir(r.queueDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(ctx, err, "read queue dir for resume")
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		promptPath := filepath.Join(r.queueDir, entry.Name())
		if err := r.resumePrompt(ctx, promptPath); err != nil {
			return errors.Wrap(ctx, err, "resume prompt")
		}
	}
	return nil
}

// resumePrompt resumes a single prompt that is in "executing" state.
func (r *resumer) resumePrompt(ctx context.Context, promptPath string) error {
	pf, containerName, baseName, logFile, title, err := r.prepareResume(ctx, promptPath)
	if err != nil || pf == nil {
		return err
	}

	canResume, err := r.workflowExecutor.ReconstructState(ctx, baseName, pf)
	if err != nil {
		return errors.Wrap(ctx, err, "reconstruct workflow state for resume")
	}
	if !canResume {
		slog.Warn(
			"cannot resume prompt: isolation directory missing; resetting to approved",
			"file", filepath.Base(promptPath),
		)
		pf.MarkApproved()
		if err := pf.Save(ctx); err != nil {
			return errors.Wrap(ctx, err, "save prompt after failed resume")
		}
		return nil
	}

	slog.Info(
		"resuming executing prompt",
		"file", filepath.Base(promptPath),
		"container", containerName,
	)

	remainingDuration, elapsed, exceeded := r.computeReattachDuration(pf.Frontmatter.Started)
	if exceeded {
		return r.killTimedOutContainer(ctx, pf, containerName, elapsed)
	}

	if err := r.executor.Reattach(ctx, logFile, containerName, remainingDuration); err != nil {
		return errors.Wrap(ctx, err, "reattach to container")
	}

	slog.Info("reattached container exited", "file", filepath.Base(promptPath))

	// Reload prompt file (state may have changed)
	pf, err = r.promptManager.Load(ctx, promptPath)
	if err != nil {
		return errors.Wrap(ctx, err, "reload prompt after reattach")
	}

	gitCtx := context.WithoutCancel(ctx)
	completedPath := filepath.Join(r.completedDir, filepath.Base(promptPath))

	completionReport, err := r.completionReportValidator.Validate(ctx, logFile)
	if err != nil {
		r.failureNotifier.NotifyFromReport(ctx, logFile, promptPath)
		return errors.Wrap(ctx, err, "validate completion report")
	}
	if completionReport != nil && completionReport.Summary != "" {
		pf.SetSummary(completionReport.Summary)
		if err := pf.Save(ctx); err != nil {
			return errors.Wrap(ctx, err, "save summary")
		}
	}

	return r.workflowExecutor.Complete(gitCtx, ctx, pf, title, promptPath, completedPath)
}

// prepareResume loads and validates the prompt for resume, returning nil pf when the prompt
// should not be resumed (not executing, or missing container — caller should return err).
func (r *resumer) prepareResume(
	ctx context.Context,
	promptPath string,
) (*prompt.PromptFile, string, prompt.BaseName, string, string, error) {
	pf, err := r.promptManager.Load(ctx, promptPath)
	if err != nil {
		return nil, "", "", "", "", errors.Wrap(ctx, err, "load prompt for resume")
	}
	if prompt.PromptStatus(pf.Frontmatter.Status) != prompt.ExecutingPromptStatus {
		return nil, "", "", "", "", nil // not executing — skip
	}

	containerName := pf.Frontmatter.Container
	if containerName == "" {
		slog.Warn("cannot resume prompt: no container name in frontmatter; resetting to approved",
			"file", filepath.Base(promptPath))
		pf.MarkApproved()
		if err := pf.Save(ctx); err != nil {
			return nil, "", "", "", "", errors.Wrap(ctx, err, "save prompt after failed resume")
		}
		return nil, "", "", "", "", nil
	}

	baseName := prompt.BaseName(strings.TrimSuffix(filepath.Base(promptPath), ".md"))
	logFile, err := filepath.Abs(filepath.Join(r.logDir, string(baseName)+".log"))
	if err != nil {
		return nil, "", "", "", "", errors.Wrap(ctx, err, "resolve log file path for resume")
	}
	title := pf.Title()
	if title == "" {
		title = baseName.String()
	}
	return pf, containerName, baseName, logFile, title, nil
}

// killTimedOutContainer stops a container that has already exceeded its timeout on reattach,
// marks the prompt as failed, and saves it.
func (r *resumer) killTimedOutContainer(
	ctx context.Context,
	pf *prompt.PromptFile,
	containerName string,
	elapsed time.Duration,
) error {
	slog.Warn("container exceeded maxPromptDuration, killing without reattach",
		"container", containerName,
		"started", pf.Frontmatter.Started,
		"elapsed", elapsed)
	r.executor.StopAndRemoveContainer(ctx, containerName)
	pf.SetLastFailReason(fmt.Sprintf("prompt timed out after %s (detected on reattach)", elapsed))
	pf.MarkFailed()
	if saveErr := pf.Save(ctx); saveErr != nil {
		return errors.Wrap(ctx, saveErr, "save prompt after timeout on reattach")
	}
	return nil
}

// computeReattachDuration computes the remaining allowed run time for a reattached container.
// Returns (remaining, elapsed, exceeded) where exceeded=true means the container has already
// run past maxPromptDuration and should be killed without reattaching.
// When maxPromptDuration is 0 or started is empty, remaining equals maxPromptDuration and exceeded is false.
func (r *resumer) computeReattachDuration(started string) (time.Duration, time.Duration, bool) {
	if r.maxPromptDuration == 0 || started == "" {
		return r.maxPromptDuration, 0, false
	}
	t, err := time.Parse(time.RFC3339, started)
	if err != nil {
		slog.Warn(
			"cannot parse started timestamp, using full timeout",
			"started", started,
			"error", err,
		)
		return r.maxPromptDuration, 0, false
	}
	elapsed := time.Since(t)
	remaining := r.maxPromptDuration - elapsed
	if remaining <= 0 {
		return 0, elapsed, true
	}
	slog.Info("computed remaining timeout for reattach",
		"remaining", remaining,
		"elapsed", elapsed,
		"maxPromptDuration", r.maxPromptDuration)
	return remaining, elapsed, false
}
