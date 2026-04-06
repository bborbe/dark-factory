// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bborbe/errors"
	"github.com/fsnotify/fsnotify"

	"github.com/bborbe/dark-factory/pkg/containerlock"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/report"
	"github.com/bborbe/dark-factory/pkg/spec"
	"github.com/bborbe/dark-factory/pkg/version"
)

var sanitizeContainerNameRegexp = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

//counterfeiter:generate -o ../../mocks/processor.go --fake-name Processor . Processor

// Processor processes queued prompts.
type Processor interface {
	Process(ctx context.Context) error
	ProcessQueue(ctx context.Context) error
	// ResumeExecuting resumes any prompts still in "executing" state on startup.
	// Called once by the runner before the normal event loop begins.
	// For each executing prompt, it reattaches to the running container and drives
	// the prompt to completion through the normal post-execution flow.
	ResumeExecuting(ctx context.Context) error
}

// NewProcessor creates a new Processor.
func NewProcessor(
	queueDir string,
	completedDir string,
	logDir string,
	projectName string,
	exec executor.Executor,
	promptManager prompt.Manager,
	releaser git.Releaser,
	versionGetter version.Getter,
	ready <-chan struct{},
	pr bool,
	worktree bool,
	brancher git.Brancher,
	prCreator git.PRCreator,
	cloner git.Cloner,
	prMerger git.PRMerger,
	autoMerge bool,
	autoRelease bool,
	autoReview bool,
	autoCompleter spec.AutoCompleter,
	specLister spec.Lister,
	validationCommand string,
	validationPrompt string,
	testCommand string,
	verificationGate bool,
	n notifier.Notifier,
	containerCounter executor.ContainerCounter,
	maxContainers int,
	additionalInstructions string,
	containerLock containerlock.ContainerLock,
	containerChecker executor.ContainerChecker,
	dirtyFileThreshold int,
	dirtyFileChecker DirtyFileChecker,
	gitLockChecker GitLockChecker,
	autoRetryLimit int,
	maxPromptDuration time.Duration,
) Processor {
	return &processor{
		queueDir:               queueDir,
		completedDir:           completedDir,
		logDir:                 logDir,
		projectName:            projectName,
		executor:               exec,
		promptManager:          promptManager,
		releaser:               releaser,
		versionGetter:          versionGetter,
		ready:                  ready,
		pr:                     pr,
		worktree:               worktree,
		brancher:               brancher,
		prCreator:              prCreator,
		cloner:                 cloner,
		autoMerge:              autoMerge,
		autoRelease:            autoRelease,
		autoReview:             autoReview,
		prMerger:               prMerger,
		autoCompleter:          autoCompleter,
		specLister:             specLister,
		validationCommand:      validationCommand,
		validationPrompt:       validationPrompt,
		testCommand:            testCommand,
		verificationGate:       verificationGate,
		skippedPrompts:         make(map[string]time.Time),
		notifier:               n,
		containerCounter:       containerCounter,
		maxContainers:          maxContainers,
		containerPollInterval:  10 * time.Second,
		additionalInstructions: additionalInstructions,
		containerLock:          containerLock,
		containerChecker:       containerChecker,
		dirtyFileThreshold:     dirtyFileThreshold,
		dirtyFileChecker:       dirtyFileChecker,
		gitLockChecker:         gitLockChecker,
		autoRetryLimit:         autoRetryLimit,
		maxPromptDuration:      maxPromptDuration,
	}
}

// processor implements Processor.
type processor struct {
	queueDir               string
	completedDir           string
	logDir                 string
	projectName            string
	executor               executor.Executor
	promptManager          prompt.Manager
	releaser               git.Releaser
	versionGetter          version.Getter
	ready                  <-chan struct{}
	pr                     bool
	worktree               bool
	brancher               git.Brancher
	prCreator              git.PRCreator
	cloner                 git.Cloner
	autoMerge              bool
	autoRelease            bool
	autoReview             bool
	prMerger               git.PRMerger
	autoCompleter          spec.AutoCompleter
	specLister             spec.Lister
	validationCommand      string
	validationPrompt       string
	testCommand            string
	verificationGate       bool
	skippedPrompts         map[string]time.Time // filename → mod time when skipped
	notifier               notifier.Notifier
	containerCounter       executor.ContainerCounter
	maxContainers          int
	containerPollInterval  time.Duration
	additionalInstructions string
	containerLock          containerlock.ContainerLock
	containerChecker       executor.ContainerChecker
	dirtyFileThreshold     int
	dirtyFileChecker       DirtyFileChecker
	gitLockChecker         GitLockChecker
	lastBlockedMsg         string
	autoRetryLimit         int
	maxPromptDuration      time.Duration
}

// Process starts processing queued prompts.
// It processes existing queued prompts on startup, then listens for signals from the watcher.
func (p *processor) Process(ctx context.Context) error {
	slog.Info("processor started")

	// Transition prompted specs with all prompts completed to verifying
	if err := p.checkPromptedSpecs(ctx); err != nil {
		return errors.Wrap(ctx, err, "check prompted specs on startup")
	}

	// Process any existing queued prompts first
	if err := p.processExistingQueued(ctx); err != nil {
		slog.Warn("prompt failed on startup scan; queue blocked until manual retry", "error", err)
		// do NOT return — daemon continues running
	}

	slog.Info("waiting for changes")

	// Listen for ready signals from watcher
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("processor shutting down")
			return nil

		case <-p.ready:
			// Watcher normalized files, check for new queued prompts
			// Clear skipped prompts so all files get re-evaluated after fsnotify event
			p.skippedPrompts = make(map[string]time.Time)
			if err := p.processExistingQueued(ctx); err != nil {
				slog.Warn("prompt failed; queue blocked until manual retry", "error", err)
				// do NOT return — daemon continues running
			}

		case <-ticker.C:
			// Periodic scan for queued prompts (in case we missed a signal)
			if err := p.processExistingQueued(ctx); err != nil {
				slog.Warn("prompt failed; queue blocked until manual retry", "error", err)
				// do NOT return — daemon continues running
			}
		}
	}
}

// ProcessQueue runs the startup sequence and drains all queued prompts, then returns.
// Unlike Process, it does not enter the event loop — suitable for one-shot / CI usage.
func (p *processor) ProcessQueue(ctx context.Context) error {
	slog.Info("processor started (one-shot)")

	// Transition prompted specs with all prompts completed to verifying
	if err := p.checkPromptedSpecs(ctx); err != nil {
		return errors.Wrap(ctx, err, "check prompted specs on startup")
	}

	// Process all existing queued prompts and return
	if err := p.processExistingQueued(ctx); err != nil {
		return errors.Wrap(ctx, err, "process existing queued prompts")
	}

	// Log once when the queue is empty (one-shot mode only)
	queued, err := p.promptManager.ListQueued(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "list queued prompts")
	}
	if len(queued) == 0 {
		slog.Info("no queued prompts")
	}

	return nil
}

// ResumeExecuting resumes any prompts still in "executing" state on startup.
// Called once by the runner before the normal event loop begins.
func (p *processor) ResumeExecuting(ctx context.Context) error {
	entries, err := os.ReadDir(p.queueDir)
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
		promptPath := filepath.Join(p.queueDir, entry.Name())
		if err := p.resumePrompt(ctx, promptPath); err != nil {
			return errors.Wrap(ctx, err, "resume prompt")
		}
	}
	return nil
}

// resumePrompt resumes a single prompt that is in "executing" state.
func (p *processor) resumePrompt(ctx context.Context, promptPath string) error {
	pf, err := p.promptManager.Load(ctx, promptPath)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt for resume")
	}
	if prompt.PromptStatus(pf.Frontmatter.Status) != prompt.ExecutingPromptStatus {
		return nil // not executing — skip
	}

	containerName := pf.Frontmatter.Container
	if containerName == "" {
		// No container name in frontmatter — cannot resume; reset to approved
		slog.Warn("cannot resume prompt: no container name in frontmatter; resetting to approved",
			"file", filepath.Base(promptPath))
		pf.MarkApproved()
		if err := pf.Save(ctx); err != nil {
			return errors.Wrap(ctx, err, "save prompt after failed resume")
		}
		return nil
	}

	baseName := strings.TrimSuffix(filepath.Base(promptPath), ".md")
	baseName = sanitizeContainerName(baseName)

	logFile, err := filepath.Abs(filepath.Join(p.logDir, baseName+".log"))
	if err != nil {
		return errors.Wrap(ctx, err, "resolve log file path for resume")
	}

	title := pf.Title()
	if title == "" {
		title = baseName
	}

	// Reconstruct workflowState from frontmatter and filesystem
	ws, ok, err := p.reconstructWorkflowState(ctx, baseName, pf)
	if err != nil {
		return errors.Wrap(ctx, err, "reconstruct workflow state")
	}
	if !ok {
		// Clone missing for PR workflow — reset to approved
		slog.Info("resetting prompt: clone directory missing for PR workflow",
			"file", filepath.Base(promptPath))
		pf.MarkApproved()
		if err := pf.Save(ctx); err != nil {
			return errors.Wrap(ctx, err, "save prompt after clone missing")
		}
		return nil
	}

	slog.Info(
		"resuming executing prompt",
		"file",
		filepath.Base(promptPath),
		"container",
		containerName,
	)

	remainingDuration, elapsed, exceeded := p.computeReattachDuration(pf.Frontmatter.Started)
	if exceeded {
		return p.killTimedOutContainer(ctx, pf, containerName, elapsed)
	}

	if err := p.executor.Reattach(ctx, logFile, containerName, remainingDuration); err != nil {
		return errors.Wrap(ctx, err, "reattach to container")
	}

	slog.Info("reattached container exited", "file", filepath.Base(promptPath))

	// Reload prompt file (state may have changed)
	pf, err = p.promptManager.Load(ctx, promptPath)
	if err != nil {
		return errors.Wrap(ctx, err, "reload prompt after reattach")
	}

	return p.handlePostExecution(ctx, pf, promptPath, title, logFile, ws)
}

// killTimedOutContainer stops a container that has already exceeded its timeout on reattach,
// marks the prompt as failed, and saves it.
func (p *processor) killTimedOutContainer(
	ctx context.Context,
	pf *prompt.PromptFile,
	containerName string,
	elapsed time.Duration,
) error {
	slog.Warn("container exceeded maxPromptDuration, killing without reattach",
		"container", containerName,
		"started", pf.Frontmatter.Started,
		"elapsed", elapsed)
	p.executor.StopAndRemoveContainer(ctx, containerName)
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
func (p *processor) computeReattachDuration(started string) (time.Duration, time.Duration, bool) {
	if p.maxPromptDuration == 0 || started == "" {
		return p.maxPromptDuration, 0, false
	}
	t, err := time.Parse(time.RFC3339, started)
	if err != nil {
		slog.Warn(
			"cannot parse started timestamp, using full timeout",
			"started",
			started,
			"error",
			err,
		)
		return p.maxPromptDuration, 0, false
	}
	elapsed := time.Since(t)
	remaining := p.maxPromptDuration - elapsed
	if remaining <= 0 {
		return 0, elapsed, true
	}
	slog.Info("computed remaining timeout for reattach",
		"remaining", remaining,
		"elapsed", elapsed,
		"maxPromptDuration", p.maxPromptDuration)
	return remaining, elapsed, false
}

// reconstructWorkflowState reconstructs the workflowState for a prompt being resumed.
// Returns (state, true, nil) on success, (nil, false, nil) when the clone is missing (signals reset-to-approved).
func (p *processor) reconstructWorkflowState(
	ctx context.Context,
	baseName string,
	pf *prompt.PromptFile,
) (*workflowState, bool, error) {
	if !p.worktree {
		// Direct or in-place workflow: no clone needed
		return &workflowState{}, true, nil
	}
	// PR/clone workflow: check clone directory exists
	clonePath := filepath.Join(os.TempDir(), "dark-factory", p.projectName+"-"+baseName)
	if _, err := os.Stat(clonePath); err != nil {
		// Clone missing — signal reset-to-approved
		return nil, false, nil
	}
	branchName := pf.Branch()
	if branchName == "" {
		branchName = "dark-factory/" + baseName
	}
	originalDir, err := os.Getwd()
	if err != nil {
		return nil, false, errors.Wrap(ctx, err, "get working directory for resume")
	}
	return &workflowState{
		clonePath:   clonePath,
		branchName:  branchName,
		originalDir: originalDir,
	}, true, nil
}

// processExistingQueued scans for and processes any existing queued prompts.
func (p *processor) processExistingQueued(ctx context.Context) error {
	// Block if any prompt is pending human verification
	if p.hasPendingVerification(ctx) {
		slog.Info("queue blocked: prompt pending verification")
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// Scan for queued prompts
		queued, err := p.promptManager.ListQueued(ctx)
		if err != nil {
			return errors.Wrap(ctx, err, "list queued prompts")
		}

		// No more queued prompts - done
		if len(queued) == 0 {
			slog.Debug("queue scan complete", "queuedCount", 0)
			return nil
		}

		slog.Debug("queue scan complete", "queuedCount", len(queued))

		// Pick first prompt (already sorted alphabetically)
		pr := queued[0]

		// Auto-set status to queued if empty or created (folder location is source of truth)
		if err := p.autoSetQueuedStatus(ctx, &pr); err != nil {
			return errors.Wrap(ctx, err, "auto-set queued status")
		}

		// Check if prompt should be skipped (validation or previously failed)
		if p.shouldSkipPrompt(ctx, pr) {
			continue
		}

		// Check ordering - all previous prompts must be completed
		if !p.promptManager.AllPreviousCompleted(ctx, pr.Number()) {
			p.logBlockedOnce(ctx, pr)
			return nil // blocked — wait for watcher signal or periodic scan
		}
		p.lastBlockedMsg = ""

		slog.Info("found queued prompt", "file", filepath.Base(pr.Path))

		// Process the prompt (includes moving to completed/ and committing)
		if err := p.processPrompt(ctx, pr); err != nil {
			if stopErr := p.handleProcessError(ctx, pr.Path, err); stopErr != nil {
				return stopErr
			}
			continue // re-queued or permanently failed — process next prompt
		}

		slog.Info("watching for queued prompts", "dir", p.queueDir)

		// Loop again to process next prompt
	}
}

// shouldSkipPrompt checks if a prompt should be skipped due to validation failure.
// Returns true if the prompt should be skipped, false if it's ready to process.
// Handles both previously-failed prompts (silent skip) and new validation failures (logged).
func (p *processor) shouldSkipPrompt(ctx context.Context, pr prompt.Prompt) bool {
	// Check if this prompt was previously skipped and hasn't been modified
	fileInfo, err := os.Stat(pr.Path)
	if err == nil {
		if lastSkipped, wasSkipped := p.skippedPrompts[pr.Path]; wasSkipped {
			if fileInfo.ModTime().Equal(lastSkipped) {
				// File hasn't changed since we last skipped it - skip silently
				slog.Debug(
					"skipping previously-failed prompt (unchanged)",
					"file",
					filepath.Base(pr.Path),
				)
				return true
			}
			// File was modified - remove from skipped list and re-validate
			delete(p.skippedPrompts, pr.Path)
		}
	}

	// Validate prompt before execution
	if err := pr.ValidateForExecution(ctx); err != nil {
		slog.Warn("skipping prompt", "file", filepath.Base(pr.Path), "reason", err.Error())
		// Record this prompt as skipped so we don't spam logs on next cycle
		if fileInfo != nil {
			p.skippedPrompts[pr.Path] = fileInfo.ModTime()
		}
		return true
	}

	return false
}

// logBlockedOnce logs the "prompt blocked" message only when the missing-prompt details change,
// suppressing repeated identical messages on every poll cycle.
func (p *processor) logBlockedOnce(ctx context.Context, pr prompt.Prompt) {
	missing := p.promptManager.FindMissingCompleted(ctx, pr.Number())
	details := make([]string, 0, len(missing))
	for _, num := range missing {
		status := p.promptManager.FindPromptStatusInProgress(ctx, num)
		if status != "" {
			details = append(details, fmt.Sprintf("%03d(%s)", num, status))
		} else {
			details = append(details, fmt.Sprintf("%03d(not found)", num))
		}
	}
	msg := strings.Join(details, ", ")
	if msg == p.lastBlockedMsg {
		return
	}
	slog.Info(
		"prompt blocked",
		"file", filepath.Base(pr.Path),
		"reason", "previous prompt not completed",
		"missing", msg,
	)
	p.lastBlockedMsg = msg
}

// autoSetQueuedStatus sets status to "queued" for any non-terminal status.
// This makes the folder location the source of truth - if a file is in queue/, it should be queued.
func (p *processor) autoSetQueuedStatus(ctx context.Context, pr *prompt.Prompt) error {
	switch pr.Status {
	case prompt.ApprovedPromptStatus,
		prompt.ExecutingPromptStatus,
		prompt.CompletedPromptStatus,
		prompt.FailedPromptStatus,
		prompt.PendingVerificationPromptStatus,
		prompt.CancelledPromptStatus:
		// Already in a valid processing state — don't override
		return nil
	}
	// Any other status (empty, "created", "draft", etc.) → auto-set to approved
	baseName := filepath.Base(pr.Path)
	previousStatus := pr.Status
	slog.Info(
		"auto-setting status to approved",
		"file",
		baseName,
		"previousStatus",
		previousStatus,
	)
	if err := p.promptManager.SetStatus(ctx, pr.Path, string(prompt.ApprovedPromptStatus)); err != nil {
		return errors.Wrap(ctx, err, "set status to approved")
	}
	// Update local status so ValidateForExecution passes
	pr.Status = prompt.ApprovedPromptStatus
	return nil
}

// handleProcessError is called when processPrompt returns an error.
// If the context is cancelled it returns an error to propagate shutdown.
// If the prompt was already moved to completed/ (post-execution failure) it stops the daemon.
// Otherwise it calls handlePromptFailure and returns nil so the loop continues.
func (p *processor) handleProcessError(ctx context.Context, path string, err error) error {
	if ctx.Err() != nil {
		slog.Info("daemon shutting down, prompt stays executing", "file", filepath.Base(path))
		return errors.Wrap(ctx, err, "prompt failed")
	}
	if stopErr := p.checkPostExecutionFailure(ctx, path, err); stopErr != nil {
		return stopErr
	}
	p.handlePromptFailure(ctx, path, err)
	return nil
}

// checkPostExecutionFailure returns a non-nil error when the prompt file is gone from its
// in-progress path but found in completed/ — indicating the container succeeded yet a
// post-execution git step failed. Returning an error stops the daemon loop so uncommitted
// code changes are not overwritten by the next prompt's git fetch/merge.
// Returns nil when the file still exists at path (normal pre-execution failure).
func (p *processor) checkPostExecutionFailure(
	ctx context.Context,
	path string,
	origErr error,
) error {
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		return nil
	}
	completedFilePath := filepath.Join(p.completedDir, filepath.Base(path))
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
func (p *processor) handlePromptFailure(ctx context.Context, path string, err error) {
	slog.Error("prompt failed", "file", filepath.Base(path), "error", err)

	pf, loadErr := p.promptManager.Load(ctx, path)
	if loadErr != nil {
		slog.Error("failed to load prompt for failure handling", "error", loadErr)
		return
	}

	reason := err.Error()
	pf.SetLastFailReason(reason)

	if p.autoRetryLimit > 0 && pf.RetryCount() < p.autoRetryLimit {
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
			p.notifyFailed(ctx, path)
			return
		}
		slog.Info("prompt re-queued for retry",
			"file", filepath.Base(path),
			"retryCount", pf.RetryCount(),
			"autoRetryLimit", p.autoRetryLimit)
		return
	}

	// Retries exhausted or autoRetryLimit == 0 — mark failed
	pf.MarkFailed()
	if saveErr := pf.Save(ctx); saveErr != nil {
		slog.Error("failed to set failed status", "error", saveErr)
	}
	p.notifyFailed(ctx, path)
}

// notifyFailed fires a notification for a failed prompt.
func (p *processor) notifyFailed(ctx context.Context, path string) {
	_ = p.notifier.Notify(ctx, notifier.Event{
		ProjectName: p.projectName,
		EventType:   "prompt_failed",
		PromptName:  filepath.Base(path),
	})
}

// notifyFromReport checks the completion report in logFile and fires a partial notification
// if the report status is "partial".
func (p *processor) notifyFromReport(ctx context.Context, logFile string, promptPath string) {
	completionReport, err := report.ParseFromLog(ctx, logFile)
	if err != nil || completionReport == nil {
		return
	}
	if completionReport.Status == "partial" {
		_ = p.notifier.Notify(ctx, notifier.Event{
			ProjectName: p.projectName,
			EventType:   "prompt_partial",
			PromptName:  filepath.Base(promptPath),
		})
	}
}

// checkPromptedSpecs scans all specs and calls CheckAndComplete for any in "prompted" status.
// This catches specs that were stuck in prompted state across daemon restarts.
func (p *processor) checkPromptedSpecs(ctx context.Context) error {
	specs, err := p.specLister.List(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "list specs")
	}

	for _, sf := range specs {
		if sf.Frontmatter.Status != string(spec.StatusPrompted) {
			continue
		}
		slog.Info("startup: checking prompted spec", "spec", sf.Name)
		if err := p.autoCompleter.CheckAndComplete(ctx, sf.Name); err != nil {
			return errors.Wrap(ctx, err, "check and complete spec")
		}
	}

	return nil
}

// waitForContainerSlot blocks until the system-wide running container count
// is below maxContainers, then returns. Checks every 10 seconds.
// Returns immediately if maxContainers <= 0 (no limit).
// Returns ctx.Err() if context is cancelled while waiting.
func (p *processor) waitForContainerSlot(ctx context.Context) error {
	if p.maxContainers <= 0 {
		return nil
	}
	for {
		count, err := p.containerCounter.CountRunning(ctx)
		if err != nil {
			slog.Warn("failed to count running containers, proceeding anyway", "error", err)
			return nil
		}
		if count < p.maxContainers {
			return nil
		}
		slog.Info(
			"waiting for container slot",
			"running", count,
			"limit", p.maxContainers,
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(p.containerPollInterval):
		}
	}
}

// prepareContainerSlot acquires the global container lock and waits for a free slot.
// Returns an idempotent release function to be deferred, and an error on failure.
// When containerLock is nil the release function is a safe no-op.
func (p *processor) prepareContainerSlot(ctx context.Context) (func(), error) {
	releaseLock := func() {}
	if p.containerLock != nil {
		if err := p.containerLock.Acquire(ctx); err != nil {
			return releaseLock, errors.Wrap(ctx, err, "acquire container lock")
		}
		var once sync.Once
		releaseLock = func() {
			once.Do(func() { _ = p.containerLock.Release(ctx) })
		}
	}
	if err := p.waitForContainerSlot(ctx); err != nil {
		releaseLock()
		return func() {}, errors.Wrap(ctx, err, "wait for container slot")
	}
	return releaseLock, nil
}

// startContainerLockRelease spawns a goroutine that releases the container lock
// as soon as the named container is confirmed running (or after a 30 s timeout).
// This limits how long the lock is held to the check-and-start window only.
func (p *processor) startContainerLockRelease(ctx context.Context, name string, release func()) {
	if p.containerChecker == nil {
		return
	}
	cc := p.containerChecker
	go func() {
		_ = cc.WaitUntilRunning(ctx, name, 30*time.Second)
		release()
	}()
}

// checkDirtyFileThreshold returns (true, nil) when the prompt should be skipped
// because the working tree has too many dirty files. Returns (false, nil) when
// the check is disabled or the count is within threshold.
func (p *processor) checkDirtyFileThreshold(ctx context.Context) (bool, error) {
	if p.dirtyFileThreshold <= 0 {
		return false, nil
	}
	count, err := p.dirtyFileChecker.CountDirtyFiles(ctx)
	if err != nil {
		return false, errors.Wrap(ctx, err, "count dirty files")
	}
	if count > p.dirtyFileThreshold {
		slog.Warn(
			"dirty file threshold exceeded, skipping prompt",
			"dirtyFiles", count,
			"threshold", p.dirtyFileThreshold,
		)
		return true, nil
	}
	return false, nil
}

// checkGitIndexLock returns true when the prompt should be skipped
// because .git/index.lock exists, false otherwise.
func (p *processor) checkGitIndexLock() bool {
	return p.gitLockChecker != nil && p.gitLockChecker.Exists()
}

// checkPreflightConditions runs all pre-execution skip checks in order.
// Returns (true, nil) if the prompt should be skipped this cycle.
func (p *processor) checkPreflightConditions(ctx context.Context) (bool, error) {
	if p.checkGitIndexLock() {
		slog.Warn("git index lock exists, skipping prompt — will retry next cycle")
		return true, nil
	}
	return p.checkDirtyFileThreshold(ctx)
}

// syncWithRemote fetches and merges from the remote default branch.
func (p *processor) syncWithRemote(ctx context.Context) error {
	slog.Info("syncing with remote default branch")
	fetchCtx, fetchCancel := context.WithTimeout(ctx, 30*time.Second)
	defer fetchCancel()
	if err := p.brancher.Fetch(fetchCtx); err != nil {
		return errors.Wrap(ctx, err, "git fetch origin")
	}
	if err := p.brancher.MergeOriginDefault(ctx); err != nil {
		return errors.Wrap(ctx, err, "git merge origin default branch")
	}
	return nil
}

// processPrompt executes a single prompt and commits the result.
func (p *processor) processPrompt(ctx context.Context, pr prompt.Prompt) error {
	if skip, err := p.checkPreflightConditions(ctx); err != nil {
		return errors.Wrap(ctx, err, "check preflight conditions")
	} else if skip {
		return nil // skip this cycle, re-check on next poll
	}

	if err := p.syncWithRemote(ctx); err != nil {
		return errors.Wrap(ctx, err, "sync with remote")
	}

	pf, err := p.promptManager.Load(ctx, pr.Path)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}
	content, err := pf.Content()
	if err != nil {
		return p.handleEmptyPrompt(ctx, pr.Path, err)
	}

	baseName, containerName, title, err := preparePromptForExecution(
		ctx,
		pf,
		pr.Path,
		p.versionGetter.Get(),
		p.projectName,
	)
	if err != nil {
		return errors.Wrap(ctx, err, "prepare prompt for execution")
	}
	content = p.enrichPromptContent(ctx, content)

	slog.Info("executing prompt", "title", title)

	// Derive log file path before setupWorkflow, which may os.Chdir to clone dir.
	logFile, err := filepath.Abs(filepath.Join(p.logDir, baseName+".log"))
	if err != nil {
		return errors.Wrap(ctx, err, "resolve log file path")
	}

	// Setup workflow (branch or clone) before execution
	workflowState, err := p.setupWorkflow(ctx, baseName, pf)
	if err != nil {
		return errors.Wrap(ctx, err, "setup workflow")
	}

	// Ensure clone cleanup on error (success path cleanup is in handleCloneWorkflow)
	if p.worktree && workflowState.clonePath != "" {
		defer p.cleanupCloneOnError(ctx, workflowState)
	}

	// Acquire container lock only for the check-and-start window, not during prep work above.
	releaseLock, err := p.prepareContainerSlot(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "prepare container slot")
	}
	defer releaseLock()

	// Release the container lock once the container has started (not after it exits).
	p.startContainerLockRelease(ctx, containerName, releaseLock)

	cancelled, execErr := p.runContainer(ctx, content, logFile, containerName, pr.Path)
	if cancelled {
		return nil // proceed to next prompt; status is already set to cancelled
	}
	if execErr != nil {
		return execErr
	}
	return p.handlePostExecution(ctx, pf, pr.Path, title, logFile, workflowState)
}

// runContainer starts the YOLO container with a cancellation watcher and returns whether
// the prompt was cancelled by the user and any execution error.
func (p *processor) runContainer(
	ctx context.Context,
	content, logFile, containerName, promptPath string,
) (cancelled bool, err error) {
	execCtx, execCancel := context.WithCancel(ctx)
	var cancelledByUser bool
	go p.watchForCancellation(execCtx, execCancel, promptPath, containerName, &cancelledByUser)

	execErr := p.executor.Execute(execCtx, content, logFile, containerName)
	execCancel() // always stop the watcher goroutine

	if cancelledByUser {
		slog.Info("prompt cancelled", "file", filepath.Base(promptPath))
		return true, nil
	}
	if execErr != nil {
		if ctx.Err() != nil {
			slog.Info("daemon shutting down, leaving container running")
		} else {
			slog.Info("docker container exited with error", "error", execErr)
		}
		return false, errors.Wrap(ctx, execErr, "execute prompt")
	}
	if ctx.Err() != nil {
		slog.Info("daemon shutting down, leaving container running")
		return false, errors.Wrap(ctx, ctx.Err(), "daemon shutdown during execution")
	}
	slog.Info("docker container exited", "exitCode", 0)
	return false, nil
}

// enrichPromptContent prepends additionalInstructions and appends machine-parseable suffixes and project-level validation to prompt content.
func (p *processor) enrichPromptContent(ctx context.Context, content string) string {
	if p.additionalInstructions != "" {
		content = p.additionalInstructions + "\n\n" + content
	}
	// Append completion report suffix to make output machine-parseable
	content = content + report.Suffix()
	// Append changelog instructions when the project has a CHANGELOG.md
	if p.releaser.HasChangelog(ctx) {
		content = content + report.ChangelogSuffix()
	}
	// Inject project-level test command for fast iteration feedback
	if p.testCommand != "" {
		content = content + report.TestCommandSuffix(p.testCommand)
	}
	// Inject project-level validation command (overrides prompt-level <verification>)
	if p.validationCommand != "" {
		content = content + report.ValidationSuffix(p.validationCommand)
	}
	// Inject project-level validation prompt criteria (AI-judged, runs after validationCommand)
	if criteria, ok := resolveValidationPrompt(ctx, p.validationPrompt); ok {
		content = content + report.ValidationPromptSuffix(criteria)
	}
	return content
}

// watchForCancellation watches the prompt file for changes using fsnotify.
// When the status changes to cancelled, it stops and removes the Docker container,
// then cancels execCancel to unblock the executor.
func (p *processor) watchForCancellation(
	ctx context.Context,
	execCancel context.CancelFunc,
	promptPath string,
	containerName string,
	cancelled *bool,
) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("failed to create cancel watcher", "error", err)
		return
	}
	defer fsWatcher.Close()

	if err := fsWatcher.Add(promptPath); err != nil {
		slog.Warn("failed to watch prompt file", "path", promptPath, "error", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-fsWatcher.Errors:
			if !ok {
				return
			}
			slog.Debug("cancel watcher error", "error", err)
		case event, ok := <-fsWatcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == 0 {
				continue
			}
			pf, err := p.promptManager.Load(ctx, promptPath)
			if err != nil {
				continue
			}
			if pf.Frontmatter.Status == string(prompt.CancelledPromptStatus) {
				*cancelled = true
				slog.Info("prompt cancelled, stopping container",
					"file", filepath.Base(promptPath),
					"container", containerName,
				)
				p.executor.StopAndRemoveContainer(ctx, containerName)
				execCancel()
				return
			}
		}
	}
}

// workflowState holds state needed for workflow cleanup and completion.
type workflowState struct {
	branchName           string
	clonePath            string
	originalDir          string
	cleanedUp            bool
	inPlaceBranch        string // non-empty when in-place branch was switched
	inPlaceDefaultBranch string // branch to restore after in-place execution
}

// cleanupCloneOnError restores the original directory and removes the clone
// when processPrompt exits with an error (success path is handled by handleCloneWorkflow).
func (p *processor) cleanupCloneOnError(ctx context.Context, state *workflowState) {
	if state.cleanedUp {
		return // Already cleaned up by handleCloneWorkflow
	}
	if state.originalDir != "" {
		if err := os.Chdir(state.originalDir); err != nil {
			slog.Warn("failed to chdir back to original directory on error", "error", err)
		}
	}
	if err := p.cloner.Remove(ctx, state.clonePath); err != nil {
		slog.Warn("failed to remove clone on error", "path", state.clonePath, "error", err)
	}
}

// handlePostExecution handles validation, moving to completed, and workflow completion.
func (p *processor) handlePostExecution(
	ctx context.Context,
	pf *prompt.PromptFile,
	promptPath string,
	title string,
	logFile string,
	state *workflowState,
) error {
	// Validate completion report from log
	summary, err := validateCompletionReport(ctx, logFile)
	if err != nil {
		p.notifyFromReport(ctx, logFile, promptPath)
		return errors.Wrap(ctx, err, "validate completion report")
	}

	// Store summary in frontmatter before moving to completed
	if summary != "" {
		pf.SetSummary(summary)
		if err := pf.Save(ctx); err != nil {
			return errors.Wrap(ctx, err, "save summary")
		}
	}

	// Use a non-cancellable context for git ops so they aren't interrupted by shutdown.
	gitCtx := context.WithoutCancel(ctx)

	// Verification gate: pause before git operations if enabled
	if p.verificationGate {
		return p.enterPendingVerification(ctx, pf, promptPath)
	}

	completedPath := filepath.Join(p.completedDir, filepath.Base(promptPath))

	if p.worktree {
		// Clone workflow: commit only code changes in clone, then manage prompt in original repo.
		return p.handleCloneWorkflow(gitCtx, ctx, pf, title, promptPath, completedPath, state)
	}

	// Direct workflow: move prompt to completed and commit in the same repo.
	featureBranch := state.inPlaceBranch
	if err := p.moveToCompletedAndCommit(ctx, gitCtx, pf, promptPath, completedPath); err != nil {
		p.restoreDefaultBranch(ctx, state)
		return errors.Wrap(ctx, err, "move to completed and commit")
	}

	if err := p.handleDirectWorkflow(gitCtx, ctx, title, featureBranch); err != nil {
		p.restoreDefaultBranch(ctx, state)
		return errors.Wrap(ctx, err, "handle direct workflow")
	}
	p.restoreDefaultBranch(ctx, state)

	// After restoring to default, check if this is the last prompt on the branch and merge+release.
	if featureBranch != "" && !p.pr {
		if err := p.handleBranchCompletion(gitCtx, ctx, promptPath, title, featureBranch); err != nil {
			return errors.Wrap(ctx, err, "handle branch completion")
		}
	}
	return nil
}

// moveToCompletedAndCommit moves the prompt to completed/, triggers spec auto-complete, and commits the file.
func (p *processor) moveToCompletedAndCommit(
	ctx context.Context,
	gitCtx context.Context,
	pf *prompt.PromptFile,
	promptPath string,
	completedPath string,
) error {
	// Move to completed/ before commit so it's included in the release
	if err := p.promptManager.MoveToCompleted(ctx, promptPath); err != nil {
		return errors.Wrap(ctx, err, "move to completed")
	}

	slog.Info("moved to completed", "file", filepath.Base(promptPath))

	// Auto-complete linked specs if all their prompts are now done
	for _, specID := range pf.Specs() {
		if err := p.autoCompleter.CheckAndComplete(ctx, specID); err != nil {
			slog.Warn("spec auto-complete failed", "spec", specID, "error", err)
		}
	}

	// Commit the completed file separately (YOLO may have already committed code changes)
	if err := p.releaser.CommitCompletedFile(gitCtx, completedPath); err != nil {
		return errors.Wrap(ctx, err, "commit completed file")
	}
	return nil
}

// setupWorkflow sets up the appropriate workflow before execution.
func (p *processor) setupWorkflow(
	ctx context.Context,
	baseName string,
	pf *prompt.PromptFile,
) (*workflowState, error) {
	state := &workflowState{}
	if p.worktree {
		return p.setupCloneWorkflowState(ctx, baseName, pf, state)
	}
	// In-place branch switching only applies when pr is enabled.
	// When pr is false (and worktree is false) we work on the current branch and ignore the branch field.
	if p.pr {
		if branch := pf.Branch(); branch != "" {
			return p.setupInPlaceBranchState(ctx, branch, state)
		}
	}
	return state, nil
}

// setupInPlaceBranchState configures in-place branch switching for non-worktree execution.
func (p *processor) setupInPlaceBranchState(
	ctx context.Context,
	branch string,
	state *workflowState,
) (*workflowState, error) {
	// Check working tree is clean before switching
	clean, err := p.brancher.IsClean(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "check working tree")
	}
	if !clean {
		return nil, errors.Errorf(
			ctx,
			"working tree is not clean; cannot switch to branch %q",
			branch,
		)
	}

	// Record default branch for restoration
	defaultBranch, err := p.brancher.DefaultBranch(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "get default branch")
	}
	state.inPlaceDefaultBranch = defaultBranch
	state.inPlaceBranch = branch

	// Check if branch exists remotely; if so switch, else create
	if err := p.brancher.FetchAndVerifyBranch(ctx, branch); err == nil {
		// Branch exists remotely — switch to it
		if err := p.brancher.Switch(ctx, branch); err != nil {
			return nil, errors.Wrap(ctx, err, "switch to existing branch")
		}
	} else {
		// Branch does not exist — create from default
		if err := p.brancher.CreateAndSwitch(ctx, branch); err != nil {
			return nil, errors.Wrap(ctx, err, "create and switch to branch")
		}
	}
	slog.Info("switched to branch for in-place execution", "branch", branch)
	return state, nil
}

// restoreDefaultBranch switches back to the default branch after in-place execution.
// It is a no-op when state.inPlaceDefaultBranch is empty.
func (p *processor) restoreDefaultBranch(ctx context.Context, state *workflowState) {
	if state.inPlaceDefaultBranch == "" {
		return
	}
	if err := p.brancher.Switch(ctx, state.inPlaceDefaultBranch); err != nil {
		slog.Warn(
			"failed to restore default branch",
			"branch",
			state.inPlaceDefaultBranch,
			"error",
			err,
		)
	} else {
		slog.Info("restored default branch", "branch", state.inPlaceDefaultBranch)
	}
}

// setupCloneWorkflowState configures state for the clone workflow.
func (p *processor) setupCloneWorkflowState(
	ctx context.Context,
	baseName string,
	pf *prompt.PromptFile,
	state *workflowState,
) (*workflowState, error) {
	if branch := pf.Branch(); branch != "" {
		state.branchName = branch
	} else {
		state.branchName = "dark-factory/" + baseName
	}
	state.clonePath = filepath.Join(os.TempDir(), "dark-factory", p.projectName+"-"+baseName)
	var err error
	state.originalDir, err = p.setupCloneForExecution(ctx, state.clonePath, state.branchName)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "setup clone for execution")
	}
	return state, nil
}

// hasPendingVerification returns true if any prompt in queueDir has pending_verification status.
func (p *processor) hasPendingVerification(ctx context.Context) bool {
	entries, err := os.ReadDir(p.queueDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		pf, err := p.promptManager.Load(ctx, filepath.Join(p.queueDir, entry.Name()))
		if err != nil {
			continue
		}
		if pf.Frontmatter.Status == string(prompt.PendingVerificationPromptStatus) {
			return true
		}
	}
	return false
}

// enterPendingVerification transitions a prompt to pending_verification state and logs the verification hint.
func (p *processor) enterPendingVerification(
	ctx context.Context,
	pf *prompt.PromptFile,
	promptPath string,
) error {
	pf.MarkPendingVerification()
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save pending verification status")
	}
	hint := pf.VerificationSection()
	if hint != "" {
		slog.Info(
			"prompt pending verification — run the following checks, then: dark-factory prompt verify <file>",
			"file",
			filepath.Base(promptPath),
			"verification",
			hint,
		)
	} else {
		slog.Info("prompt pending verification",
			"file", filepath.Base(promptPath),
			"hint", "run: dark-factory prompt verify <file> when ready",
		)
	}
	return nil
}

// handleEmptyPrompt handles empty prompts by moving them to completed without execution.
func (p *processor) handleEmptyPrompt(
	ctx context.Context,
	promptPath string,
	contentErr error,
) error {
	// If prompt is empty, move to completed and skip execution
	if stderrors.Is(contentErr, prompt.ErrEmptyPrompt) {
		slog.Debug(
			"skipping empty prompt",
			"file",
			filepath.Base(promptPath),
			"reason",
			"file may still be in progress",
		)
		// Move empty prompts to completed/ (but don't commit)
		if err := p.promptManager.MoveToCompleted(ctx, promptPath); err != nil {
			return errors.Wrap(ctx, err, "move empty prompt to completed")
		}
		return nil
	}
	return errors.Wrap(ctx, contentErr, "get prompt content")
}

// postMergeActions performs post-merge actions: switch to default branch, pull, and optionally release.
func (p *processor) postMergeActions(
	gitCtx context.Context,
	ctx context.Context,
	title string,
) error {
	// PR merged successfully — switch to default branch
	defaultBranch, err := p.brancher.DefaultBranch(gitCtx)
	if err != nil {
		return errors.Wrap(ctx, err, "get default branch")
	}

	if err := p.brancher.Switch(gitCtx, defaultBranch); err != nil {
		return errors.Wrap(ctx, err, "switch to default branch")
	}

	if err := p.brancher.Pull(gitCtx); err != nil {
		return errors.Wrap(ctx, err, "pull default branch")
	}

	slog.Info("merged PR and updated default branch", "branch", defaultBranch)

	// If autoRelease enabled and has changelog, create release
	if p.autoRelease && p.releaser.HasChangelog(gitCtx) {
		if err := p.handleDirectWorkflow(gitCtx, ctx, title, ""); err != nil {
			return errors.Wrap(ctx, err, "auto-release after merge")
		}
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

// findOrCreatePR checks for an existing open PR on the branch and creates one if absent.
// Returns the PR URL (existing or newly created).
func (p *processor) findOrCreatePR(
	gitCtx context.Context,
	ctx context.Context,
	branchName string,
	title string,
	issue string,
) (string, error) {
	prURL, err := p.prCreator.FindOpenPR(gitCtx, branchName)
	if err != nil {
		slog.Warn("failed to check for existing PR", "branch", branchName, "error", err)
		// Fall through to create attempt — may result in duplicate, user can resolve
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
	prURL, err = p.prCreator.Create(gitCtx, title, buildPRBody(issue))
	if err != nil {
		return "", errors.Wrap(ctx, err, "create pull request")
	}
	slog.Info("created PR", "url", prURL)
	return prURL, nil
}

// handleAutoMergeForClone handles the auto-merge decision after clone workflow:
// defers merge when more prompts remain on branch, otherwise merges immediately.
func (p *processor) handleAutoMergeForClone(
	gitCtx context.Context,
	ctx context.Context,
	pf *prompt.PromptFile,
	branchName string,
	promptPath string,
	completedPath string,
	prURL string,
	title string,
) error {
	hasMore, err := p.promptManager.HasQueuedPromptsOnBranch(ctx, branchName, promptPath)
	if err != nil {
		slog.Warn("failed to check remaining prompts on branch", "branch", branchName, "error", err)
		// Fall through: merge anyway (safe default, avoids blocking forever)
	}
	if hasMore {
		slog.Info("more prompts queued on branch — deferring auto-merge", "branch", branchName)
		if err := p.moveToCompletedAndCommit(ctx, gitCtx, pf, promptPath, completedPath); err != nil {
			return errors.Wrap(ctx, err, "move to completed and commit")
		}
		p.savePRURLToFrontmatter(gitCtx, completedPath, prURL)
		return nil
	}
	// Last prompt on branch — proceed with merge
	if err := p.prMerger.WaitAndMerge(gitCtx, prURL); err != nil {
		return errors.Wrap(ctx, err, "wait and merge PR")
	}
	if err := p.moveToCompletedAndCommit(ctx, gitCtx, pf, promptPath, completedPath); err != nil {
		return errors.Wrap(ctx, err, "move to completed and commit")
	}
	return p.postMergeActions(gitCtx, ctx, title)
}

// handleCloneWorkflow handles the clone-based workflow: commit code in clone, push, create PR,
// then manage prompt lifecycle in the original repo.
func (p *processor) handleCloneWorkflow(
	gitCtx context.Context,
	ctx context.Context,
	pf *prompt.PromptFile,
	title string,
	promptPath string,
	completedPath string,
	state *workflowState,
) error {
	branchName := state.branchName
	clonePath := state.clonePath
	originalDir := state.originalDir

	// Commit only code changes in the clone (no prompt files)
	if err := p.releaser.CommitOnly(gitCtx, title); err != nil {
		return errors.Wrap(ctx, err, "commit changes")
	}

	// Push branch
	if err := p.brancher.Push(gitCtx, branchName); err != nil {
		return errors.Wrap(ctx, err, "push branch")
	}

	// Find or create PR (idempotent)
	prURL, err := p.findOrCreatePR(gitCtx, ctx, branchName, title, pf.Issue())
	if err != nil {
		return errors.Wrap(ctx, err, "find or create PR")
	}

	// Switch back to original directory before managing prompt
	if err := os.Chdir(originalDir); err != nil {
		return errors.Wrap(ctx, err, "chdir back to original directory")
	}

	// Remove clone (best-effort cleanup)
	if err := p.cloner.Remove(gitCtx, clonePath); err != nil {
		slog.Warn("failed to remove clone", "path", clonePath, "error", err)
	}
	state.cleanedUp = true

	// --- From here, we're back in the original repo ---

	if p.autoMerge {
		return p.handleAutoMergeForClone(
			gitCtx, ctx, pf, branchName, promptPath, completedPath, prURL, title,
		)
	}

	// autoReview mode: keep prompt in queueDir with in_review status + PR URL
	if p.autoReview {
		p.savePRURLToFrontmatter(gitCtx, promptPath, prURL)
		if err := p.promptManager.SetStatus(ctx, promptPath, string(prompt.InReviewPromptStatus)); err != nil {
			return errors.Wrap(ctx, err, "set in_review status")
		}
		slog.Info("PR created, waiting for review", "url", prURL)
		return nil
	}

	// Default: move prompt to completed in original repo with PR URL
	if err := p.moveToCompletedAndCommit(ctx, gitCtx, pf, promptPath, completedPath); err != nil {
		return errors.Wrap(ctx, err, "move to completed and commit")
	}
	p.savePRURLToFrontmatter(gitCtx, completedPath, prURL)

	return nil
}

// setupCloneForExecution creates a clone, switches to it, and sets up cleanup.
// Returns the original directory path for later restoration.
func (p *processor) setupCloneForExecution(
	ctx context.Context,
	clonePath string,
	branchName string,
) (string, error) {
	originalDir, err := os.Getwd()
	if err != nil {
		return "", errors.Wrap(ctx, err, "get current directory")
	}

	if err := p.cloner.Clone(ctx, originalDir, clonePath, branchName); err != nil {
		return "", errors.Wrap(ctx, err, "clone repo")
	}

	// Switch to clone directory
	if err := os.Chdir(clonePath); err != nil {
		return "", errors.Wrap(ctx, err, "chdir to clone")
	}

	return originalDir, nil
}

// handleDirectWorkflow handles the direct commit workflow: commit, tag, push.
// When featureBranch is non-empty, only commits (no release) — release happens after the branch merges.
func (p *processor) handleDirectWorkflow(
	gitCtx context.Context,
	ctx context.Context,
	title string,
	featureBranch string,
) error {
	// On a feature branch: commit only, never release
	if featureBranch != "" {
		if err := p.releaser.CommitOnly(gitCtx, title); err != nil {
			return errors.Wrap(ctx, err, "commit on feature branch")
		}
		slog.Info("committed changes on feature branch (no release)", "branch", featureBranch)
		return nil
	}

	// Without CHANGELOG: simple commit only (no tag, no push)
	if !p.releaser.HasChangelog(gitCtx) {
		if err := p.releaser.CommitOnly(gitCtx, title); err != nil {
			return errors.Wrap(ctx, err, "commit")
		}
		slog.Info("committed changes")
		return nil
	}

	// With CHANGELOG but autoRelease disabled: commit only, keep "## Unreleased"
	if !p.autoRelease {
		if err := p.releaser.CommitOnly(gitCtx, title); err != nil {
			return errors.Wrap(ctx, err, "commit without release")
		}
		slog.Info("committed changes (autoRelease disabled, skipping tag)")
		return nil
	}

	// With CHANGELOG and autoRelease enabled: rename ## Unreleased to version, tag, push
	bump := git.DetermineBumpFromChangelog(ctx, ".")
	nextVersion, err := p.releaser.GetNextVersion(gitCtx, bump)
	if err != nil {
		return errors.Wrap(ctx, err, "get next version")
	}

	if err := p.releaser.CommitAndRelease(gitCtx, bump); err != nil {
		return errors.Wrap(ctx, err, "commit and release")
	}

	slog.Info("committed and tagged", "version", nextVersion)

	return nil
}

// handleBranchCompletion checks if this was the last prompt on a feature branch.
// If so, merges the branch to default and triggers a release.
func (p *processor) handleBranchCompletion(
	gitCtx context.Context,
	ctx context.Context,
	promptPath string,
	title string,
	featureBranch string,
) error {
	// Check if any other queued prompts share the same branch
	hasMore, err := p.promptManager.HasQueuedPromptsOnBranch(ctx, featureBranch, promptPath)
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

	// Merge feature branch to default (we're already on default after restoreDefaultBranch)
	if err := p.brancher.MergeToDefault(gitCtx, featureBranch); err != nil {
		return errors.Wrap(ctx, err, "merge feature branch to default")
	}

	// Release on default branch (pass empty featureBranch so release logic runs)
	if err := p.handleDirectWorkflow(gitCtx, ctx, title, ""); err != nil {
		return errors.Wrap(ctx, err, "release after branch merge")
	}

	return nil
}

// preparePromptForExecution sets up the prompt metadata and returns execution parameters.
func preparePromptForExecution(
	ctx context.Context,
	pf *prompt.PromptFile,
	promptPath string,
	version string,
	projectName string,
) (string, string, string, error) {
	baseName := strings.TrimSuffix(filepath.Base(promptPath), ".md")
	baseName = sanitizeContainerName(baseName)
	containerName := projectName + "-" + baseName

	pf.PrepareForExecution(containerName, version)
	if err := pf.Save(ctx); err != nil {
		return "", "", "", errors.Wrap(ctx, err, "save prompt metadata")
	}

	title := pf.Title()
	if title == "" {
		// Fallback to filename if no title found
		title = strings.TrimSuffix(filepath.Base(promptPath), ".md")
	}

	return baseName, containerName, title, nil
}

// validateCompletionReport parses and validates the completion report from the log file.
// Returns the summary and an error if the report indicates failure.
// Returns ("", nil) if no report found (backwards compatible) or parse error.
// Returns (summary, nil) if report indicates success.
func validateCompletionReport(ctx context.Context, logFile string) (string, error) {
	completionReport, err := report.ParseFromLog(ctx, logFile)
	if err != nil {
		slog.Debug("failed to parse completion report", "error", err)
		// Continue — don't fail the prompt just because report parsing failed
		return "", nil
	}
	if completionReport == nil {
		// No report found — backwards compatible
		return "", nil
	}

	slog.Info(
		"completion report",
		"status",
		completionReport.Status,
		"summary",
		completionReport.Summary,
	)

	// Validate consistency between status and verification results
	correctedStatus, overridden := completionReport.ValidateConsistency()
	if overridden {
		slog.Warn(
			"overriding self-reported status",
			"reported", completionReport.Status,
			"corrected", correctedStatus,
			"verificationCommand", completionReport.Verification.Command,
			"verificationExitCode", completionReport.Verification.ExitCode,
		)
		completionReport.Status = correctedStatus
	}

	if completionReport.Status != "success" {
		// Report says not success — treat as failure
		slog.Info("completion report indicates failure", "status", completionReport.Status)
		if len(completionReport.Blockers) > 0 {
			slog.Info("blockers reported", "blockers", completionReport.Blockers)
		}
		return "", errors.Errorf(ctx, "completion report status: %s", completionReport.Status)
	}

	return completionReport.Summary, nil
}

// savePRURLToFrontmatter saves the PR URL to the prompt frontmatter.
// This is best-effort and non-fatal — all failures are logged as warnings.
func (p *processor) savePRURLToFrontmatter(
	ctx context.Context,
	completedPath string,
	prURL string,
) {
	// Preserve existing pr-url for follow-up prompts
	if existingPF, err := p.promptManager.Load(ctx, completedPath); err == nil &&
		existingPF.PRURL() != "" {
		slog.Debug("pr-url already set, preserving existing value")
		return
	}
	if err := p.promptManager.SetPRURL(ctx, completedPath, prURL); err != nil {
		slog.Warn("failed to save PR URL to frontmatter", "error", err)
	}
}

// sanitizeContainerName ensures the name only contains Docker-safe characters [a-zA-Z0-9_-]
func sanitizeContainerName(name string) string {
	// Replace any character that is not alphanumeric, underscore, or hyphen with hyphen
	return sanitizeContainerNameRegexp.ReplaceAllString(name, "-")
}

// resolveValidationPrompt resolves the validationPrompt config value.
// If value is a relative path to an existing file, the file contents are returned.
// If value is non-empty but the file does not exist, ("", false) is returned (caller logs warning).
// If value is empty, ("", false) is returned silently.
// The resolved result is the criteria text to inject, or empty string to skip injection.
func resolveValidationPrompt(ctx context.Context, value string) (string, bool) {
	if value == "" {
		return "", false
	}
	// Check if value is a path to an existing file
	if _, err := os.Stat(value); err == nil {
		data, readErr := os.ReadFile(
			value,
		) // #nosec G304 -- path is validated by config (no absolute path, no .. traversal)
		if readErr != nil {
			slog.WarnContext(
				ctx,
				"failed to read validationPrompt file",
				"path",
				value,
				"error",
				readErr,
			)
			return "", false
		}
		return string(data), true
	}
	// Check if value looks like a file path (contains path separator or .md extension)
	// and the file doesn't exist — log a warning
	if strings.Contains(value, string(filepath.Separator)) || strings.HasSuffix(value, ".md") {
		slog.WarnContext(
			ctx,
			"validationPrompt file not found, skipping criteria evaluation",
			"path",
			value,
		)
		return "", false
	}
	// Value is inline criteria text
	return value, true
}
