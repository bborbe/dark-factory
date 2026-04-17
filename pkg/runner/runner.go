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
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/review"
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
	reviewPoller review.ReviewPoller,
	specWatcher specwatcher.SpecWatcher,
	projectName string,
	containerChecker executor.ContainerChecker,
	n notifier.Notifier,
	slugMigrator slugmigrator.Migrator,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	mover prompt.FileMover,
	maxPromptDuration time.Duration,
	containerStopper executor.ContainerStopper,
	startupLogger func(),
	hideGit bool,
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
		reviewPoller:          reviewPoller,
		specWatcher:           specWatcher,
		projectName:           projectName,
		containerChecker:      containerChecker,
		notifier:              n,
		slugMigrator:          slugMigrator,
		currentDateTimeGetter: currentDateTimeGetter,
		mover:                 mover,
		maxPromptDuration:     maxPromptDuration,
		containerStopper:      containerStopper,
		startupLogger:         startupLogger,
		hideGit:               hideGit,
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
	reviewPoller          review.ReviewPoller
	specWatcher           specwatcher.SpecWatcher
	projectName           string
	containerChecker      executor.ContainerChecker
	notifier              notifier.Notifier
	slugMigrator          slugmigrator.Migrator
	currentDateTimeGetter libtime.CurrentDateTimeGetter
	mover                 prompt.FileMover
	maxPromptDuration     time.Duration
	containerStopper      executor.ContainerStopper
	startupLogger         func()
	hideGit               bool
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

	if logFile, err := os.Create(".dark-factory.log"); err != nil {
		slog.Warn("failed to create daemon log file, continuing without", "error", err)
	} else {
		defer logFile.Close()
		level := slog.LevelInfo
		if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			level = slog.LevelDebug
		}
		w := io.MultiWriter(os.Stderr, logFile)
		slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
	}

	if r.startupLogger != nil {
		r.startupLogger()
	}

	// Abort if .git/index.lock exists — all git operations will fail.
	// Skip the check when hideGit is enabled (container won't use git).
	if !r.hideGit {
		if _, err := os.Stat(filepath.Join(".", ".git", "index.lock")); err == nil {
			return errors.Errorf(
				ctx,
				".git/index.lock exists — remove it before starting the daemon (another git process may be running)",
			)
		}
	}

	// Set up signal handling
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("watching for queued prompts", "dir", r.inProgressDir)

	// Run the six shared startup steps (migrateQueueDir, createDirectories,
	// resumeOrResetExecuting, reindexAll, normalizeFilenames, migrateSpecSlugs).
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

	// Run watcher, processor, server, and optional reviewPoller in parallel
	// If any fails, context cancels the others automatically
	runners := []run.Func{
		r.watcher.Watch,
		r.processor.Process,
	}
	if r.server != nil {
		runners = append(runners, r.server.ListenAndServe)
	}
	if r.reviewPoller != nil {
		runners = append(runners, r.reviewPoller.Run)
	}
	if r.specWatcher != nil {
		runners = append(runners, r.specWatcher.Watch)
	}
	runners = append(runners, r.healthCheckLoop)
	return run.CancelOnFirstError(ctx, runners...)
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
		ContainerChecker:      r.containerChecker,
		Notifier:              r.notifier,
		ProjectName:           r.projectName,
		SlugMigrator:          r.slugMigrator,
		Mover:                 r.mover,
		CurrentDateTimeGetter: r.currentDateTimeGetter,
	}
}

// healthCheckLoop runs the periodic container health check loop.
func (r *runner) healthCheckLoop(ctx context.Context) error {
	return runHealthCheckLoop(
		ctx,
		30*time.Second,
		r.inProgressDir,
		r.specsInProgressDir,
		r.containerChecker,
		r.promptManager,
		r.notifier,
		r.projectName,
		r.currentDateTimeGetter,
		r.maxPromptDuration,
		r.containerStopper,
	)
}

// resumeOrResetGenerating selectively resumes or resets generating specs based on container liveness.
func (r *runner) resumeOrResetGenerating(ctx context.Context) error {
	return resumeOrResetGenerating(
		ctx,
		r.specsInProgressDir,
		r.containerChecker,
		r.currentDateTimeGetter,
	)
}
