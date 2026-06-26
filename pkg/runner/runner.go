// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bborbe/errors"
	"github.com/bborbe/run"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/healthcheckgate"
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/preflight"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/project"
	"github.com/bborbe/dark-factory/pkg/server"
	"github.com/bborbe/dark-factory/pkg/slugmigrator"
	"github.com/bborbe/dark-factory/pkg/specwatcher"
	"github.com/bborbe/dark-factory/pkg/watcher"
)

// Runner orchestrates the main processing loop.
type Runner interface {
	Run(ctx context.Context) error
}

// NewRunner creates a new Runner.
// startupLogger is an optional func called after lock acquisition to emit the effective-config log line.
// Pass nil to skip the startup log.
func NewRunner(
	inboxDir string,
	inProgressDir string,
	completedDir string,
	logDir string,
	specsInboxDir string,
	specsInProgressDir string,
	specsCompletedDir string,
	specsLogDir string,
	promptManager PromptManager,
	locker lock.Locker,
	watcher watcher.Watcher,
	processor processor.Processor,
	server server.Server,
	specWatcher specwatcher.SpecWatcher,
	projectName project.Name,
	executionChecker executor.ExecutionChecker,
	n notifier.Notifier,
	slugMigrator slugmigrator.Migrator,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	maxPromptDuration time.Duration,
	executionStopper executor.ExecutionStopper,
	startupLogger func(),
	hideGit bool,
	preflightChecker preflight.Checker,
	logWriter io.Writer,
	healthcheckGate healthcheckgate.Gate,
) Runner {
	return &runner{
		inboxDir:              inboxDir,
		inProgressDir:         inProgressDir,
		completedDir:          completedDir,
		logDir:                logDir,
		specsInboxDir:         specsInboxDir,
		specsInProgressDir:    specsInProgressDir,
		specsCompletedDir:     specsCompletedDir,
		specsLogDir:           specsLogDir,
		promptManager:         promptManager,
		locker:                locker,
		watcher:               watcher,
		processor:             processor,
		server:                server,
		specWatcher:           specWatcher,
		projectName:           projectName,
		executionChecker:      executionChecker,
		notifier:              n,
		slugMigrator:          slugMigrator,
		currentDateTimeGetter: currentDateTimeGetter,
		maxPromptDuration:     maxPromptDuration,
		executionStopper:      executionStopper,
		startupLogger:         startupLogger,
		hideGit:               hideGit,
		preflightChecker:      preflightChecker,
		logWriter:             logWriter,
		healthcheckGate:       healthcheckGate,
	}
}

// runner orchestrates the main processing loop.
type runner struct {
	inboxDir              string
	inProgressDir         string
	completedDir          string
	logDir                string
	specsInboxDir         string
	specsInProgressDir    string
	specsCompletedDir     string
	specsLogDir           string
	promptManager         PromptManager
	locker                lock.Locker
	watcher               watcher.Watcher
	processor             processor.Processor
	server                server.Server
	specWatcher           specwatcher.SpecWatcher
	projectName           project.Name
	executionChecker      executor.ExecutionChecker
	notifier              notifier.Notifier
	slugMigrator          slugmigrator.Migrator
	currentDateTimeGetter libtime.CurrentDateTimeGetter
	maxPromptDuration     time.Duration
	executionStopper      executor.ExecutionStopper
	startupLogger         func()
	hideGit               bool
	preflightChecker      preflight.Checker
	logWriter             io.Writer
	healthcheckGate       healthcheckgate.Gate
}

// Run executes the main processing loop:
// 1. Acquire instance lock to prevent concurrent runs
// 2. Reset any stuck "executing" prompts from previous crash
// 3. Normalize filenames before processing
// 4. Run watcher and processor in parallel using run.CancelOnFirstError
func (r *runner) Run(ctx context.Context) error {
	// Acquire instance lock
	if err := r.locker.Acquire(ctx); err != nil {
		return errors.Wrap(ctx, err, "acquire lock")
	}
	defer func() {
		if err := r.locker.Release(context.WithoutCancel(ctx)); err != nil {
			slog.Info("failed to release lock", "error", err)
		}
	}()

	slog.Info("acquired lock", "file", ".dark-factory.lock")

	// Git safety checks — skip when hideGit is enabled (container won't use git).
	// 1. Refuse to start from a worktree or submodule CWD — the .git pointer
	//    points to the parent repo's worktrees/ directory, which is not mounted.
	// 2. Abort if .git/index.lock exists — all git operations will fail.
	if err := CheckGitSafety(ctx, r.hideGit); err != nil {
		return err
	}

	if r.logWriter != nil {
		if closer, ok := r.logWriter.(io.Closer); ok {
			defer closer.Close()
		}
		level := slog.LevelInfo
		if slog.Default().Enabled(ctx, slog.LevelDebug) {
			level = slog.LevelDebug
		}
		w := io.MultiWriter(os.Stderr, r.logWriter)
		slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
	}

	if r.startupLogger != nil {
		r.startupLogger()
	}

	// Set up signal handling
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("watching for queued prompts", "dir", r.inProgressDir)

	// Run the five shared startup steps (migrateQueueDir, createDirectories,
	// resumeOrResetExecuting, normalizeFilenames, migrateSpecSlugs).
	if err := startupSequence(ctx, r.startupDeps()); err != nil {
		return errors.Wrap(ctx, err, "startup sequence")
	}

	// Daemon-only: reset specs left in generating state if their container is gone.
	// Not in startupSequence because this step has no counterpart in the one-shot runner.
	if err := r.resumeOrResetGenerating(ctx); err != nil {
		return errors.Wrap(ctx, err, "resume or reset generating specs")
	}

	// Daemon-only: reattach to any prompts still in executing state from a prior run.
	// Not in startupSequence for the same reason.
	if err := r.processor.ResumeExecuting(ctx); err != nil {
		return errors.Wrap(ctx, err, "resume executing prompts")
	}

	// Daemon-only: retry git commits for any prompts left in "committing" state.
	if err := r.processor.ResumeCommitting(ctx); err != nil {
		slog.Warn("resume committing failed on startup, will retry on next cycle", "error", err)
		// non-fatal — continue startup
	}

	// Startup preflight: verify baseline is green before the watcher loop begins.
	if err := r.runStartupPreflight(ctx); err != nil {
		return err
	}

	// Startup healthcheck gate: verify the pipeline stack before the watcher loop begins.
	if err := r.runStartupHealthcheck(ctx); err != nil {
		return err
	}

	// Run watcher, processor, server, and optional specWatcher in parallel
	// If any fails, context cancels the others automatically
	runners := []run.Func{
		r.watcher.Watch,
		r.processor.Process,
	}
	if r.server != nil {
		runners = append(runners, r.server.ListenAndServe)
	}
	if r.specWatcher != nil {
		runners = append(runners, r.specWatcher.Watch)
	}
	runners = append(runners, r.healthCheckLoop)
	return run.CancelOnFirstError(ctx, runners...)
}

// runStartupHealthcheck runs the healthcheck startup gate before the watcher loop.
// Returns nil when the gate is nil (not wired), disabled, skipped, a fresh cache
// hit, or the probes pass. Returns a terminal error when the probes fail.
func (r *runner) runStartupHealthcheck(ctx context.Context) error {
	if r.healthcheckGate == nil {
		return nil
	}
	if err := r.healthcheckGate.Check(ctx); err != nil {
		return errors.Wrap(ctx, err, "healthcheck startup gate")
	}
	return nil
}

// runStartupPreflight verifies the baseline is green before the watcher loop begins.
// Returns ErrPreflightFailed when the check fails or returns an error.
// Returns nil when preflightChecker is nil (preflight disabled) or when the check passes.
func (r *runner) runStartupPreflight(ctx context.Context) error {
	if r.preflightChecker == nil {
		return nil
	}
	ok, err := r.preflightChecker.Check(ctx)
	if err != nil {
		slog.Warn("preflight checker error", "error", err)
		return processor.ErrPreflightFailed
	}
	if !ok {
		slog.Info("preflight: baseline broken — dark-factory will exit")
		return processor.ErrPreflightFailed
	}
	return nil
}

// startupDeps builds a StartupDeps from this runner's fields.
func (r *runner) startupDeps() StartupDeps {
	return StartupDeps{
		InboxDir:              r.inboxDir,
		InProgressDir:         r.inProgressDir,
		CompletedDir:          r.completedDir,
		LogDir:                r.logDir,
		SpecsInboxDir:         r.specsInboxDir,
		SpecsInProgressDir:    r.specsInProgressDir,
		SpecsCompletedDir:     r.specsCompletedDir,
		SpecsLogDir:           r.specsLogDir,
		PromptManager:         r.promptManager,
		ExecutionChecker:      r.executionChecker,
		Notifier:              r.notifier,
		ProjectName:           r.projectName.String(),
		SlugMigrator:          r.slugMigrator,
		CurrentDateTimeGetter: r.currentDateTimeGetter,
	}
}

// CheckGitSafety verifies git safety conditions before starting either
// the daemon or a one-shot run:
//  1. Refuse to start from a worktree or submodule CWD — the .git pointer
//     points to the parent repo's worktrees/ directory, which is not mounted.
//  2. Abort if .git/index.lock exists — all git operations will fail.
//
// Skips both checks when hideGit is true (the operator has explicitly opted
// into the .git mask, so the worktree pointer is hidden anyway).
func CheckGitSafety(ctx context.Context, hideGit bool) error {
	if hideGit {
		return nil
	}
	if err := DetectWorktreeOrSubmodule(ctx); err != nil {
		return errors.Wrap(ctx, err, "worktree/submodule CWD detected")
	}
	if _, err := os.Stat(filepath.Join(".", ".git", "index.lock")); err == nil {
		return errors.Errorf(
			ctx,
			".git/index.lock exists — remove it before starting the daemon (another git process may be running)",
		)
	}
	return nil
}

// healthCheckLoop runs the periodic container health check loop.
func (r *runner) healthCheckLoop(ctx context.Context) error {
	return runHealthCheckLoop(
		ctx,
		30*time.Second,
		r.inProgressDir,
		r.specsInProgressDir,
		r.executionChecker,
		r.promptManager,
		r.notifier,
		r.projectName.String(),
		r.currentDateTimeGetter,
		r.maxPromptDuration,
		r.executionStopper,
	)
}

// resumeOrResetGenerating selectively resumes or resets generating specs based on container liveness.
func (r *runner) resumeOrResetGenerating(ctx context.Context) error {
	return resumeOrResetGenerating(
		ctx,
		r.specsInProgressDir,
		r.executionChecker,
		r.currentDateTimeGetter,
		r.projectName.String(),
	)
}
