// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	stderrors "errors"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/cancellationwatcher"
	"github.com/bborbe/dark-factory/pkg/committingrecoverer"
	"github.com/bborbe/dark-factory/pkg/completionreport"
	"github.com/bborbe/dark-factory/pkg/containerslot"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/failurehandler"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/preflightconditions"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/promptenricher"
	"github.com/bborbe/dark-factory/pkg/promptresumer"
	"github.com/bborbe/dark-factory/pkg/queuescanner"
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
// Call SetScanner before invoking Process — the processor panics if queueScanner is nil.
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
	committingRecoverer committingrecoverer.Recoverer,
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
) *processor {
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
		queueInterval:             queueInterval,
		sweepInterval:             sweepInterval,
		onIdle:                    onIdle,
		completionReportValidator: completionReportValidator,
		promptEnricher:            promptEnricher,
		committingRecoverer:       committingRecoverer,
	}
}

// SetScanner injects the QueueScanner after construction (breaks the runtime cycle
// proc → scanner → proc.ProcessPrompt).
func (p *processor) SetScanner(s queuescanner.Scanner) {
	p.queueScanner = s
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
	resumer                   promptresumer.Resumer
	verificationGate          VerificationGate
	queueInterval             time.Duration
	sweepInterval             time.Duration
	onIdle                    NothingToDoCallback
	completionReportValidator completionreport.Validator
	promptEnricher            promptenricher.Enricher
	committingRecoverer       committingrecoverer.Recoverer
	queueScanner              queuescanner.Scanner
}

// Process starts processing queued prompts.
// It processes existing queued prompts on startup, then listens for signals from the watcher.
// When a tick ends with no progress, onIdle is called. Daemon mode logs; one-shot mode cancels.
func (p *processor) Process(ctx context.Context) error {
	if p.queueScanner == nil {
		panic("processor: queueScanner is nil — call SetScanner before Process")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	slog.Info("processor started")

	// Startup scans — do NOT fire onIdle here; that would cancel one-shot before work starts.
	if _, err := p.specSweeper.Sweep(ctx); err != nil {
		return errors.Wrap(ctx, err, "check prompted specs on startup")
	}

	if _, err := p.queueScanner.ScanAndProcess(ctx); err != nil {
		slog.Warn("prompt failed on startup scan; queue blocked until manual retry", "error", err)
		// do NOT return — daemon continues running
	}

	// After startup scan, also retry any committing prompts.
	p.committingRecoverer.RecoverAll(ctx)

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
	p.queueScanner.ClearSkippedCache()
	p.committingRecoverer.RecoverAll(ctx)
	completed, err := p.queueScanner.ScanAndProcess(ctx)
	if err != nil {
		slog.Warn("prompt failed; queue blocked until manual retry", "error", err)
	}
	return (tickResult{completedPrompts: completed}).madeProgress()
}

// runQueueTick handles a periodic queue poll. Returns true if the tick made progress.
func (p *processor) runQueueTick(ctx context.Context) bool {
	p.committingRecoverer.RecoverAll(ctx)
	completed, err := p.queueScanner.ScanAndProcess(ctx)
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
	p.committingRecoverer.RecoverAll(ctx)
	return nil // always non-fatal
}

// ProcessPrompt executes a single prompt and commits the result.
func (p *processor) ProcessPrompt(ctx context.Context, pr prompt.Prompt) error {
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
