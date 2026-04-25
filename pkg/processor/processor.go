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
	libtime "github.com/bborbe/time"
	"github.com/fsnotify/fsnotify"

	"github.com/bborbe/dark-factory/pkg/containerlock"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/preflight"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/report"
	"github.com/bborbe/dark-factory/pkg/spec"
	"github.com/bborbe/dark-factory/pkg/version"
)

var sanitizeContainerNameRegexp = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// errPreflightSkip is returned by processPrompt when the baseline preflight check
// failed and the prompt should NOT be retried within the same scan cycle.
// The caller in processExistingQueued recognizes this sentinel and returns,
// which gives control back to the 5s ticker in Process().
//
// Do NOT use this for the other skip conditions (git-index-lock, dirty-files) —
// those are transient and it is safe to advance to the next prompt in the queue.
var errPreflightSkip = stderrors.New("preflight baseline broken — skip cycle")

//counterfeiter:generate -o ../../mocks/processor.go --fake-name Processor . Processor

// Processor processes queued prompts.
type Processor interface {
	Process(ctx context.Context) error
	// ResumeExecuting resumes any prompts still in "executing" state on startup.
	// Called once by the runner before the normal event loop begins.
	// For each executing prompt, it reattaches to the running container and drives
	// the prompt to completion through the normal post-execution flow.
	ResumeExecuting(ctx context.Context) error
	// ResumeCommitting retries git commits for any prompts in "committing" state on startup.
	// Called once by the runner before the normal event loop begins.
	// Unlike ResumeExecuting, failures are non-fatal: the prompt stays committing and is
	// retried on the next daemon cycle.
	ResumeCommitting(ctx context.Context) error
}

// NothingToDoCallback fires when a Process tick ends with no progress made.
// Daemon mode passes a log-only callback. One-shot mode passes one that calls cancel().
type NothingToDoCallback func(ctx context.Context, cancel context.CancelFunc)

// tickResult aggregates progress signals from a single Process tick.
type tickResult struct {
	completedPrompts  int
	transitionedSpecs int
}

func (r tickResult) madeProgress() bool {
	return r.completedPrompts > 0 || r.transitionedSpecs > 0
}

// NewProcessor creates a new Processor.
func NewProcessor(
	queueDir string,
	completedDir string,
	logDir string,
	projectName string,
	exec executor.Executor,
	promptManager PromptManager,
	releaser git.Releaser,
	versionGetter version.Getter,
	ready <-chan struct{},
	workflowExecutor WorkflowExecutor,
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
	// queueInterval controls how often the daemon polls for queued prompts.
	// Pass 0 to use the default of 5s.
	queueInterval time.Duration,
	// sweepInterval controls the auto-complete sweep cadence.
	// Pass 0 to use the default of 60s.
	sweepInterval time.Duration,
	preflightChecker preflight.Checker,
	// onIdle is invoked at the end of any tick that made no progress.
	// Pass a log-only callback for daemon mode, or one that calls cancel() for one-shot mode.
	// If nil, a no-op callback is used (safe for tests that do not need idle detection).
	onIdle NothingToDoCallback,
) Processor {
	if queueInterval <= 0 {
		queueInterval = 5 * time.Second
	}
	if sweepInterval <= 0 {
		sweepInterval = 60 * time.Second
	}
	if onIdle == nil {
		onIdle = func(_ context.Context, _ context.CancelFunc) {}
	}
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
		workflowExecutor:       workflowExecutor,
		autoCompleter:          autoCompleter,
		specLister:             specLister,
		validationCommand:      validationCommand,
		validationPrompt:       validationPrompt,
		testCommand:            testCommand,
		verificationGate:       verificationGate,
		skippedPrompts:         make(map[string]libtime.DateTime),
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
		queueInterval:          queueInterval,
		sweepInterval:          sweepInterval,
		preflightChecker:       preflightChecker,
		onIdle:                 onIdle,
	}
}

// processor implements Processor.
type processor struct {
	queueDir               string
	completedDir           string
	logDir                 string
	projectName            string
	executor               executor.Executor
	promptManager          PromptManager
	releaser               git.Releaser
	versionGetter          version.Getter
	ready                  <-chan struct{}
	workflowExecutor       WorkflowExecutor
	autoCompleter          spec.AutoCompleter
	specLister             spec.Lister
	validationCommand      string
	validationPrompt       string
	testCommand            string
	verificationGate       bool
	skippedPrompts         map[string]libtime.DateTime // filename → mod time when skipped
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
	queueInterval          time.Duration
	sweepInterval          time.Duration
	preflightChecker       preflight.Checker // nil = disabled
	onIdle                 NothingToDoCallback
}

// Process starts processing queued prompts.
// It processes existing queued prompts on startup, then listens for signals from the watcher.
// When a tick ends with no progress, onIdle is called. Daemon mode logs; one-shot mode cancels.
func (p *processor) Process(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	slog.Info("processor started")

	// Startup scans — do NOT fire onIdle here; that would cancel one-shot before work starts.
	if _, err := p.checkPromptedSpecs(ctx); err != nil {
		return errors.Wrap(ctx, err, "check prompted specs on startup")
	}

	if _, err := p.processExistingQueued(ctx); err != nil {
		slog.Warn("prompt failed on startup scan; queue blocked until manual retry", "error", err)
		// do NOT return — daemon continues running
	}

	// After startup scan, also retry any committing prompts.
	p.processCommittingPrompts(ctx)

	slog.Info("waiting for changes")

	// Listen for ready signals from watcher
	ticker := time.NewTicker(p.queueInterval)
	defer ticker.Stop()

	// Slow self-healing sweep: catches specs stuck in `prompted` if the per-prompt
	// CheckAndComplete missed (daemon crash mid-completion, race, future regression).
	// Cadence kept slower than the queue ticker because the sweep is more expensive.
	sweepTicker := time.NewTicker(p.sweepInterval)
	defer sweepTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("processor shutting down")
			return nil

		case <-p.ready:
			if !p.runReadyTick(ctx) {
				p.onIdle(ctx, cancel)
			}

		case <-ticker.C:
			if !p.runQueueTick(ctx) {
				p.onIdle(ctx, cancel)
			}

		case <-sweepTicker.C:
			if !p.runSweepTick(ctx) {
				p.onIdle(ctx, cancel)
			}
		}
	}
}

// runReadyTick handles a watcher-ready event. Returns true if the tick made progress.
func (p *processor) runReadyTick(ctx context.Context) bool {
	// Clear skipped prompts so all files get re-evaluated after fsnotify event.
	p.skippedPrompts = make(map[string]libtime.DateTime)
	p.processCommittingPrompts(ctx)
	completed, err := p.processExistingQueued(ctx)
	if err != nil {
		slog.Warn("prompt failed; queue blocked until manual retry", "error", err)
	}
	return (tickResult{completedPrompts: completed}).madeProgress()
}

// runQueueTick handles a periodic queue poll. Returns true if the tick made progress.
func (p *processor) runQueueTick(ctx context.Context) bool {
	p.processCommittingPrompts(ctx)
	completed, err := p.processExistingQueued(ctx)
	if err != nil {
		slog.Warn("prompt failed; queue blocked until manual retry", "error", err)
	}
	return (tickResult{completedPrompts: completed}).madeProgress()
}

// runSweepTick handles a periodic spec sweep. Returns true if the tick made progress.
func (p *processor) runSweepTick(ctx context.Context) bool {
	transitioned, err := p.checkPromptedSpecs(ctx)
	if err != nil {
		slog.Warn("periodic checkPromptedSpecs failed", "error", err)
	}
	return (tickResult{transitionedSpecs: transitioned}).madeProgress()
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

// ResumeCommitting retries git commits for any prompts still in "committing" state on startup.
func (p *processor) ResumeCommitting(ctx context.Context) error {
	p.processCommittingPrompts(ctx)
	return nil // always non-fatal
}

// processCommittingPrompts retries git commits for prompts in "committing" state.
// Used on startup and on each daemon cycle. Failures are non-fatal.
func (p *processor) processCommittingPrompts(ctx context.Context) {
	paths, err := p.promptManager.FindCommitting(ctx)
	if err != nil {
		slog.Warn("failed to scan for committing prompts", "error", err)
		return
	}
	for _, promptPath := range paths {
		if ctx.Err() != nil {
			return
		}
		if err := p.recoverCommittingPrompt(ctx, promptPath); err != nil {
			slog.Error("git commit failed after all retries, will retry next cycle",
				"file", filepath.Base(promptPath), "error", err)
		}
	}
}

// recoverCommittingPrompt attempts to commit dirty work files and move the prompt to completed/.
// If dirty work files exist, they are committed first (the container's code changes).
// If no dirty files exist, the code was already committed — only the prompt move is needed.
func (p *processor) recoverCommittingPrompt(ctx context.Context, promptPath string) error {
	gitCtx := context.WithoutCancel(ctx)
	completedPath := filepath.Join(p.completedDir, filepath.Base(promptPath))

	pf, err := p.promptManager.Load(ctx, promptPath)
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
		if err := p.autoCompleter.CheckAndComplete(ctx, specID); err != nil {
			slog.Warn("spec auto-complete failed", "spec", specID, "error", err)
		}
	}

	if err := p.promptManager.MoveToCompleted(ctx, promptPath); err != nil {
		return errors.Wrap(ctx, err, "move to completed during recovery")
	}

	if err := git.CommitWithRetry(gitCtx, git.DefaultCommitBackoff, func(retryCtx context.Context) error {
		return p.releaser.CommitCompletedFile(retryCtx, completedPath)
	}); err != nil {
		return errors.Wrap(ctx, err, "commit completed file during recovery")
	}

	slog.Info("git commit recovery succeeded", "file", filepath.Base(completedPath))
	return nil
}

// resumePrompt resumes a single prompt that is in "executing" state.
func (p *processor) resumePrompt(ctx context.Context, promptPath string) error {
	pf, containerName, baseName, logFile, title, err := p.prepareResume(ctx, promptPath)
	if err != nil || pf == nil {
		return err
	}

	// Reconstruct workflow state via executor
	canResume, err := p.workflowExecutor.ReconstructState(ctx, baseName, pf)
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

	gitCtx := context.WithoutCancel(ctx)
	completedPath := filepath.Join(p.completedDir, filepath.Base(promptPath))

	completionReport, err := validateCompletionReport(ctx, logFile)
	if err != nil {
		p.notifyFromReport(ctx, logFile, promptPath)
		return errors.Wrap(ctx, err, "validate completion report")
	}
	if completionReport != nil && completionReport.Summary != "" {
		pf.SetSummary(completionReport.Summary)
		if err := pf.Save(ctx); err != nil {
			return errors.Wrap(ctx, err, "save summary")
		}
	}

	return p.workflowExecutor.Complete(gitCtx, ctx, pf, title, promptPath, completedPath)
}

// prepareResume loads and validates the prompt for resume, returning nil pf when the prompt
// should not be resumed (not executing, or missing container — caller should return err).
func (p *processor) prepareResume(
	ctx context.Context,
	promptPath string,
) (*prompt.PromptFile, string, string, string, string, error) {
	pf, err := p.promptManager.Load(ctx, promptPath)
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

	baseName, _ := computePromptMetadata(promptPath, p.projectName)
	logFile, err := filepath.Abs(filepath.Join(p.logDir, baseName+".log"))
	if err != nil {
		return nil, "", "", "", "", errors.Wrap(ctx, err, "resolve log file path for resume")
	}
	title := pf.Title()
	if title == "" {
		title = baseName
	}
	return pf, containerName, baseName, logFile, title, nil
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

// processExistingQueued scans for and processes any existing queued prompts.
// Returns the count of prompts successfully processed (moved to completed) and any fatal error.
func (p *processor) processExistingQueued(ctx context.Context) (int, error) {
	if p.hasPendingVerification(ctx) {
		slog.Info("queue blocked: prompt pending verification")
		return 0, nil
	}

	completed := 0
	for {
		select {
		case <-ctx.Done():
			return completed, nil
		default:
		}

		done, err := p.processSingleQueued(ctx)
		if err != nil {
			return completed, err
		}
		if done {
			return completed, nil
		}
		completed++
	}
}

// processSingleQueued picks the next queued prompt and processes it.
// Returns (true, nil) when the scan loop should stop (queue empty, blocked, or preflight broken).
// Returns (false, nil) to continue scanning for the next prompt.
// Returns (true, err) when a fatal error requires the daemon to stop.
func (p *processor) processSingleQueued(ctx context.Context) (bool, error) {
	queued, err := p.promptManager.ListQueued(ctx)
	if err != nil {
		return true, errors.Wrap(ctx, err, "list queued prompts")
	}

	if len(queued) == 0 {
		slog.Debug("queue scan complete", "queuedCount", 0)
		return true, nil
	}

	slog.Debug("queue scan complete", "queuedCount", len(queued))

	pr := queued[0]

	if err := p.autoSetQueuedStatus(ctx, &pr); err != nil {
		return true, errors.Wrap(ctx, err, "auto-set queued status")
	}

	if p.shouldSkipPrompt(ctx, pr) {
		return false, nil
	}

	if !p.promptManager.AllPreviousCompleted(ctx, pr.Number()) {
		p.logBlockedOnce(ctx, pr)
		return true, nil // blocked — wait for watcher signal or periodic scan
	}
	p.lastBlockedMsg = ""

	slog.Info("found queued prompt", "file", filepath.Base(pr.Path))

	if err := p.processPrompt(ctx, pr); err != nil {
		if stderrors.Is(err, errPreflightSkip) {
			// Baseline is broken — exit scan loop and wait for next 5s tick.
			return true, nil
		}
		if stopErr := p.handleProcessError(ctx, pr.Path, err); stopErr != nil {
			return true, stopErr
		}
		return false, nil // re-queued or permanently failed — process next prompt
	}

	slog.Info("watching for queued prompts", "dir", p.queueDir)
	return false, nil
}

// shouldSkipPrompt checks if a prompt should be skipped due to validation failure.
// Returns true if the prompt should be skipped, false if it's ready to process.
// Handles both previously-failed prompts (silent skip) and new validation failures (logged).
func (p *processor) shouldSkipPrompt(ctx context.Context, pr prompt.Prompt) bool {
	// Check if this prompt was previously skipped and hasn't been modified
	fileInfo, err := os.Stat(pr.Path)
	if err == nil {
		if lastSkipped, wasSkipped := p.skippedPrompts[pr.Path]; wasSkipped {
			if fileInfo.ModTime().Equal(time.Time(lastSkipped)) {
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
			p.skippedPrompts[pr.Path] = libtime.DateTime(fileInfo.ModTime())
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
// Returns the count of specs checked and any fatal error.
// This catches specs that were stuck in prompted state across daemon restarts.
func (p *processor) checkPromptedSpecs(ctx context.Context) (int, error) {
	specs, err := p.specLister.List(ctx)
	if err != nil {
		return 0, errors.Wrap(ctx, err, "list specs")
	}

	count := 0
	for _, sf := range specs {
		if sf.Frontmatter.Status != string(spec.StatusPrompted) {
			continue
		}
		slog.Info("startup: checking prompted spec", "spec", sf.Name)
		if err := p.autoCompleter.CheckAndComplete(ctx, sf.Name); err != nil {
			return count, errors.Wrap(ctx, err, "check and complete spec")
		}
		count++
	}

	return count, nil
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

// hasFreeSlot returns true when maxContainers is unlimited (<=0) or when
// the current system-wide running count is below maxContainers.
// On counter error, the behaviour matches waitForContainerSlot's existing
// tolerance: log a warning and return true so the daemon makes forward
// progress — docker itself will reject a start if resources are truly absent.
func (p *processor) hasFreeSlot(ctx context.Context) bool {
	if p.maxContainers <= 0 {
		return true
	}
	count, err := p.containerCounter.CountRunning(ctx)
	if err != nil {
		slog.Warn("failed to count running containers, proceeding anyway", "error", err)
		return true
	}
	return count < p.maxContainers
}

// prepareContainerSlot acquires the global container lock only for the
// check-and-start window. If the slot is full, it RELEASES the lock and
// sleeps before retrying, so other daemons (possibly with higher limits)
// are not blocked while this daemon waits.
//
// On success, returns with the lock held and an idempotent release function.
// The caller is responsible for starting the container and calling
// startContainerLockRelease, which releases the lock after the container is
// confirmed running.
//
// When p.containerLock is nil, no locking is performed and the only wait is
// the unlocked waitForContainerSlot poll (unchanged behaviour for nil-lock case).
func (p *processor) prepareContainerSlot(ctx context.Context) (func(), error) {
	if p.containerLock == nil {
		if err := p.waitForContainerSlot(ctx); err != nil {
			return func() {}, errors.Wrap(ctx, err, "wait for container slot")
		}
		return func() {}, nil
	}

	for {
		if err := p.containerLock.Acquire(ctx); err != nil {
			return func() {}, errors.Wrap(ctx, err, "acquire container lock")
		}

		// Idempotent release — the caller MAY call this early, the retry
		// branch below WILL call this before sleeping, and startContainerLockRelease
		// will call it after the container is confirmed running.
		var once sync.Once
		releaseLock := func() {
			once.Do(func() { _ = p.containerLock.Release(ctx) })
		}

		// Lock held — count must be stable for this daemon's decision.
		if p.hasFreeSlot(ctx) {
			// Lock stays held; caller does docker run + startContainerLockRelease.
			return releaseLock, nil
		}

		// No slot — release before sleeping so other daemons can proceed.
		releaseLock()
		slog.Info(
			"waiting for container slot",
			"limit", p.maxContainers,
		)
		select {
		case <-ctx.Done():
			return func() {}, errors.Wrapf(ctx, ctx.Err(), "wait for container slot cancelled")
		case <-time.After(p.containerPollInterval):
		}
	}
}

// startContainerLockRelease spawns a goroutine that releases the container lock
// as soon as the named container is confirmed running (or after a 30 s timeout).
// This limits how long the lock is held to the check-and-start window only.
//
// A goroutine is required here because lock release must happen asynchronously
// while the caller continues with prompt execution. release() is deferred to
// guarantee the lock is always freed, even if ctx is cancelled before
// WaitUntilRunning returns (e.g. on shutdown).
func (p *processor) startContainerLockRelease(ctx context.Context, name string, release func()) {
	if p.containerChecker == nil {
		return
	}
	cc := p.containerChecker
	go func() {
		defer release()
		_ = cc.WaitUntilRunning(ctx, name, 30*time.Second)
	}()
}

// checkDirtyFileThreshold returns (true, nil) when the prompt should be skipped
// because the working tree has too many dirty files. Returns (false, nil) when
// the check is disabled or the count is within threshold.
func (p *processor) checkDirtyFileThreshold(ctx context.Context) (bool, error) {
	if p.dirtyFileThreshold <= 0 || p.dirtyFileChecker == nil {
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
// Returns (true, nil) if the prompt should be skipped this cycle (transient conditions).
// Returns (false, errPreflightSkip) if the preflight baseline is broken — the caller
// must exit the scan loop and wait for the next ticker/watcher event.
func (p *processor) checkPreflightConditions(ctx context.Context) (bool, error) {
	// Baseline preflight check — must pass before any container starts
	if p.preflightChecker != nil {
		ok, err := p.preflightChecker.Check(ctx)
		if err != nil {
			slog.Warn("preflight checker error, skipping cycle", "error", err)
			return false, errPreflightSkip
		}
		if !ok {
			slog.Info("preflight: baseline broken — prompt stays queued until baseline is fixed")
			return false, errPreflightSkip
		}
	}

	if p.checkGitIndexLock() {
		slog.Warn("git index lock exists, skipping prompt — will retry next cycle")
		return true, nil
	}
	return p.checkDirtyFileThreshold(ctx)
}

// processPrompt executes a single prompt and commits the result.
func (p *processor) processPrompt(ctx context.Context, pr prompt.Prompt) error {
	if skip, err := p.checkPreflightConditions(ctx); err != nil {
		if stderrors.Is(err, errPreflightSkip) {
			return err // propagate sentinel unwrapped so caller can recognize it
		}
		return errors.Wrap(ctx, err, "check preflight conditions")
	} else if skip {
		return nil // transient skip (git lock / dirty files) — advance to next prompt
	}

	pf, err := p.promptManager.Load(ctx, pr.Path)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}
	content, err := pf.Content()
	if err != nil {
		return p.handleEmptyPrompt(ctx, pr.Path, err)
	}

	baseName, containerName := computePromptMetadata(pr.Path, p.projectName)
	title := pf.Title()
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(pr.Path), ".md")
	}
	content = p.enrichPromptContent(ctx, content)

	slog.Info("executing prompt", "title", title)

	// Derive log file path before Setup, which may os.Chdir to clone/worktree dir.
	logFile, err := filepath.Abs(filepath.Join(p.logDir, baseName+".log"))
	if err != nil {
		return errors.Wrap(ctx, err, "resolve log file path")
	}

	// Setup workflow (sync, branch or clone) before execution.
	// This is intentionally done BEFORE persisting the container name (pf.Save) so that
	// if sync fails, the prompt file is not modified and checkPostExecutionFailure can
	// correctly detect pre-execution failures vs post-execution failures.
	if err := p.workflowExecutor.Setup(ctx, baseName, pf); err != nil {
		return errors.Wrap(ctx, err, "setup workflow")
	}
	defer p.workflowExecutor.CleanupOnError(ctx)

	// Persist container name and version AFTER sync succeeds (so resume can find the container).
	pf.PrepareForExecution(containerName, p.versionGetter.Get())
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save prompt metadata")
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

	gitCtx := context.WithoutCancel(ctx)
	completedPath := filepath.Join(p.completedDir, filepath.Base(pr.Path))

	// Verification gate: pause before git operations if enabled
	if p.verificationGate {
		return p.enterPendingVerification(ctx, pf, pr.Path)
	}

	completionReport, err := validateCompletionReport(ctx, logFile)
	if err != nil {
		p.notifyFromReport(ctx, logFile, pr.Path)
		return errors.Wrap(ctx, err, "validate completion report")
	}
	if completionReport != nil && completionReport.Summary != "" {
		pf.SetSummary(completionReport.Summary)
		if err := pf.Save(ctx); err != nil {
			return errors.Wrap(ctx, err, "save summary")
		}
	}

	return p.workflowExecutor.Complete(gitCtx, ctx, pf, title, pr.Path, completedPath)
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
			if !event.Has(fsnotify.Write) {
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

// computePromptMetadata derives the baseName and containerName from the prompt path and project name.
// It does NOT save to disk — call pf.PrepareForExecution + pf.Save separately after sync succeeds.
func computePromptMetadata(promptPath, projectName string) (string, string) {
	baseName := strings.TrimSuffix(filepath.Base(promptPath), ".md")
	baseName = sanitizeContainerName(baseName)
	containerName := projectName + "-" + baseName
	return baseName, containerName
}

// validateCompletionReport parses the completion report from the log and detects claude-CLI-level failures.
// Returns (report, nil) when a report is present and indicates success.
// Returns (nil, nil) when no report is present AND no critical failure is detected in the log
// (backwards compatible — old prompts without reports are treated as successful).
// Returns (nil, error) when:
//   - the log shows a claude-CLI critical failure (auth error, API error) even without a report
//   - a parseable report indicates non-success status (after consistency check)
//   - the report exists but is malformed
func validateCompletionReport(
	ctx context.Context,
	logFile string,
) (*report.CompletionReport, error) {
	completionReport, err := report.ParseFromLog(ctx, logFile)
	if err != nil {
		slog.Debug("failed to parse completion report", "error", err)
		// Parse error — downgrade to "no report" and fall through to critical failure scan.
		completionReport = nil
	}

	if completionReport == nil {
		// No report found (or parse error) — scan for claude-CLI-level critical failures.
		reason, scanErr := report.ScanForCriticalFailures(ctx, logFile)
		if scanErr != nil {
			slog.Debug("failed to scan for critical failures", "error", scanErr)
			// I/O error during scan — treat as no failure detected (don't block backwards compat).
			return nil, nil //nolint:nilnil
		}
		if reason != "" {
			return nil, errors.Errorf(ctx, "claude CLI critical failure: %s", reason)
		}
		// No report, no critical failure — backwards compatible success.
		return nil, nil //nolint:nilnil
	}

	slog.Info(
		"completion report",
		"status",
		completionReport.Status,
		"summary",
		completionReport.Summary,
	)

	// Validate consistency between status and verification results.
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
		// Report says not success — treat as failure.
		slog.Info("completion report indicates failure", "status", completionReport.Status)
		if len(completionReport.Blockers) > 0 {
			slog.Info("blockers reported", "blockers", completionReport.Blockers)
		}
		return nil, errors.Errorf(ctx, "completion report status: %s", completionReport.Status)
	}

	return completionReport, nil
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
