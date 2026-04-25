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
	"strings"
	"time"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/cancellationwatcher"
	"github.com/bborbe/dark-factory/pkg/completionreport"
	"github.com/bborbe/dark-factory/pkg/containerslot"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/failurehandler"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/preflightconditions"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/promptenricher"
	"github.com/bborbe/dark-factory/pkg/promptresumer"
	"github.com/bborbe/dark-factory/pkg/spec"
	"github.com/bborbe/dark-factory/pkg/specsweeper"
	"github.com/bborbe/dark-factory/pkg/version"
)

// ErrPreflightSkip re-exports preflightconditions.ErrPreflightSkip so existing
// stderrors.Is(err, processor.ErrPreflightSkip) callers continue to match without rewriting.
var ErrPreflightSkip = preflightconditions.ErrPreflightSkip

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
	exec executor.Executor,
	promptManager PromptManager,
	releaser git.Releaser,
	versionGetter version.Getter,
	workflowExecutor WorkflowExecutor,
	autoCompleter spec.AutoCompleter,
	specSweeper specsweeper.Sweeper,
	preflightConditions preflightconditions.Conditions,
	containerSlotManager containerslot.Manager,
	cancellationWatcher cancellationwatcher.Watcher,
	wakeup <-chan struct{},
	dirs Dirs,
	projectName ProjectName,
	failureHandler failurehandler.Handler,
	resumer promptresumer.Resumer,
	verificationGate VerificationGate,
	completionReportValidator completionreport.Validator,
	promptEnricher promptenricher.Enricher,
	// queueInterval controls how often the daemon polls for queued prompts.
	// Pass 0 to use the default of 5s.
	queueInterval time.Duration,
	// sweepInterval controls the auto-complete sweep cadence.
	// Pass 0 to use the default of 60s.
	sweepInterval time.Duration,
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
		executor:                  exec,
		promptManager:             promptManager,
		releaser:                  releaser,
		versionGetter:             versionGetter,
		workflowExecutor:          workflowExecutor,
		autoCompleter:             autoCompleter,
		specSweeper:               specSweeper,
		failureHandler:            failureHandler,
		preflightConditions:       preflightConditions,
		containerSlotManager:      containerSlotManager,
		cancellationWatcher:       cancellationWatcher,
		wakeup:                    wakeup,
		dirs:                      dirs,
		projectName:               projectName,
		resumer:                   resumer,
		verificationGate:          verificationGate,
		skippedPrompts:            make(map[string]libtime.DateTime),
		queueInterval:             queueInterval,
		sweepInterval:             sweepInterval,
		onIdle:                    onIdle,
		completionReportValidator: completionReportValidator,
		promptEnricher:            promptEnricher,
	}
}

// processor implements Processor.
type processor struct {
	executor                  executor.Executor
	promptManager             PromptManager
	releaser                  git.Releaser
	versionGetter             version.Getter
	workflowExecutor          WorkflowExecutor
	autoCompleter             spec.AutoCompleter
	specSweeper               specsweeper.Sweeper
	failureHandler            failurehandler.Handler
	preflightConditions       preflightconditions.Conditions
	containerSlotManager      containerslot.Manager
	cancellationWatcher       cancellationwatcher.Watcher
	wakeup                    <-chan struct{}
	dirs                      Dirs
	projectName               ProjectName
	lastBlockedMsg            string
	resumer                   promptresumer.Resumer
	verificationGate          VerificationGate
	skippedPrompts            map[string]libtime.DateTime // filename → mod time when skipped
	queueInterval             time.Duration
	sweepInterval             time.Duration
	onIdle                    NothingToDoCallback
	completionReportValidator completionreport.Validator
	promptEnricher            promptenricher.Enricher
}

// Process starts processing queued prompts.
// It processes existing queued prompts on startup, then listens for signals from the watcher.
// When a tick ends with no progress, onIdle is called. Daemon mode logs; one-shot mode cancels.
func (p *processor) Process(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	slog.Info("processor started")

	// Startup scans — do NOT fire onIdle here; that would cancel one-shot before work starts.
	if _, err := p.specSweeper.Sweep(ctx); err != nil {
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

		case <-p.wakeup:
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
	transitioned, err := p.specSweeper.Sweep(ctx)
	if err != nil {
		slog.Warn("periodic spec sweep failed", "error", err)
	}
	return (tickResult{transitionedSpecs: transitioned}).madeProgress()
}

// ResumeExecuting resumes any prompts still in "executing" state on startup.
// Called once by the runner before the normal event loop begins.
func (p *processor) ResumeExecuting(ctx context.Context) error {
	return p.resumer.ResumeAll(ctx)
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
	completedPath := filepath.Join(p.dirs.Completed, filepath.Base(promptPath))

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
		if stderrors.Is(err, ErrPreflightSkip) {
			// Baseline is broken — exit scan loop and wait for next 5s tick.
			return true, nil
		}
		if stopErr := p.failureHandler.Handle(ctx, pr.Path, err); stopErr != nil {
			return true, stopErr
		}
		return false, nil // re-queued or permanently failed — process next prompt
	}

	slog.Info("watching for queued prompts", "dir", p.dirs.Queue)
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

// processPrompt executes a single prompt and commits the result.
func (p *processor) processPrompt(ctx context.Context, pr prompt.Prompt) error {
	if skip, err := p.preflightConditions.ShouldSkip(ctx); err != nil {
		if stderrors.Is(err, ErrPreflightSkip) {
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
	content = p.promptEnricher.Enrich(ctx, content)

	slog.Info("executing prompt", "title", title)

	// Derive log file path before Setup, which may os.Chdir to clone/worktree dir.
	logFile, err := filepath.Abs(filepath.Join(p.dirs.Log, string(baseName)+".log"))
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
	pf.PrepareForExecution(containerName.String(), p.versionGetter.Get())
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save prompt metadata")
	}

	// Acquire container lock only for the check-and-start window, not during prep work above.
	releaseLock, err := p.containerSlotManager.Acquire(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "prepare container slot")
	}
	defer releaseLock()

	// Release the container lock once the container has started (not after it exits).
	p.containerSlotManager.ReleaseAfterStart(ctx, containerName.String(), releaseLock)

	cancelled, execErr := p.runContainer(ctx, content, logFile, containerName, pr.Path)
	if cancelled {
		return nil // proceed to next prompt; status is already set to cancelled
	}
	if execErr != nil {
		return execErr
	}

	gitCtx := context.WithoutCancel(ctx)
	completedPath := filepath.Join(p.dirs.Completed, filepath.Base(pr.Path))

	// Verification gate: pause before git operations if enabled
	if p.verificationGate {
		return p.enterPendingVerification(ctx, pf, pr.Path)
	}

	completionReport, err := p.completionReportValidator.Validate(ctx, logFile)
	if err != nil {
		p.failureHandler.NotifyFromReport(ctx, logFile, pr.Path)
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
	content, logFile string,
	containerName ContainerName,
	promptPath string,
) (cancelled bool, err error) {
	execCtx, execCancel := context.WithCancel(ctx)
	defer execCancel()

	cancelledCh := p.cancellationWatcher.Watch(execCtx, promptPath, containerName.String())

	// Track whether cancellation closed before Execute returned.
	// A bool written by the select goroutine and read after Execute blocks — no overlap.
	cancelledByUser := false
	go func() {
		select {
		case <-execCtx.Done():
			return
		case <-cancelledCh:
			cancelledByUser = true
			execCancel()
		}
	}()

	execErr := p.executor.Execute(execCtx, content, logFile, containerName.String())

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

// hasPendingVerification returns true if any prompt in queueDir has pending_verification status.
func (p *processor) hasPendingVerification(ctx context.Context) bool {
	entries, err := os.ReadDir(p.dirs.Queue)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		pf, err := p.promptManager.Load(ctx, filepath.Join(p.dirs.Queue, entry.Name()))
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
func computePromptMetadata(promptPath string, projectName ProjectName) (BaseName, ContainerName) {
	base := BaseName(strings.TrimSuffix(filepath.Base(promptPath), ".md"))
	name := ContainerName(string(projectName) + "-" + string(base)).Sanitize()
	return base, name
}
