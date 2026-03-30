// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

import (
	"context"
	"log/slog"
	"os/signal"
	"syscall"

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
func NewRunner(
	inboxDir string,
	inProgressDir string,
	completedDir string,
	logDir string,
	specsInboxDir string,
	specsInProgressDir string,
	specsCompletedDir string,
	specsLogDir string,
	promptManager prompt.Manager,
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
	promptManager         prompt.Manager
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

	// Set up signal handling
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Migrate old prompts/queue/ → prompts/in-progress/ if needed
	if err := r.migrateQueueDir(ctx); err != nil {
		return errors.Wrap(ctx, err, "migrate queue dir")
	}

	// Create directories if they don't exist
	if err := r.createDirectories(ctx); err != nil {
		return errors.Wrap(ctx, err, "create directories")
	}

	slog.Info("watching for queued prompts", "dir", r.inProgressDir)

	// Selectively resume or reset executing prompts based on container liveness
	if err := r.resumeOrResetExecuting(ctx); err != nil {
		return errors.Wrap(ctx, err, "resume or reset executing prompts")
	}

	// Resume any prompts still in executing state (container was still running on restart)
	if err := r.processor.ResumeExecuting(ctx); err != nil {
		return errors.Wrap(ctx, err, "resume executing prompts")
	}

	// Reindex all spec and prompt dirs to resolve cross-directory number conflicts
	if err := r.reindexAll(ctx); err != nil {
		return errors.Wrap(ctx, err, "reindex files")
	}

	// Normalize filenames before processing
	if err := r.normalizeFilenames(ctx); err != nil {
		return errors.Wrap(ctx, err, "normalize filenames")
	}

	// Migrate bare spec number refs to full slugs in all prompt lifecycle dirs
	if err := r.migrateSpecSlugs(ctx); err != nil {
		return errors.Wrap(ctx, err, "migrate spec slugs")
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
	return run.CancelOnFirstError(ctx, runners...)
}

// reindexAll runs the full reindex sequence for this runner's spec and prompt dirs.
func (r *runner) reindexAll(ctx context.Context) error {
	specDirs := []string{r.specsInboxDir, r.specsInProgressDir, r.specsCompletedDir, r.specsLogDir}
	promptDirs := []string{r.inboxDir, r.inProgressDir, r.completedDir, r.logDir}
	return reindexAll(ctx, specDirs, promptDirs, r.mover, r.currentDateTimeGetter)
}

// migrateSpecSlugs replaces bare spec number references with full slugs in all prompt dirs.
func (r *runner) migrateSpecSlugs(ctx context.Context) error {
	return r.slugMigrator.MigrateDirs(ctx, []string{
		r.inboxDir, r.inProgressDir, r.completedDir, r.logDir,
	})
}

// normalizeFilenames normalizes filenames in the in-progress directory only.
// The inbox directory is not normalized as it contains draft files.
func (r *runner) normalizeFilenames(ctx context.Context) error {
	return normalizeFilenames(ctx, r.promptManager, r.inProgressDir)
}

// migrateQueueDir renames prompts/queue/ → prompts/in-progress/ if the old path
// exists and the new path does not. This is a one-time migration.
func (r *runner) migrateQueueDir(ctx context.Context) error {
	return migrateQueueDir(ctx, r.inProgressDir)
}

// resumeOrResetExecuting selectively resumes or resets executing prompts based on container liveness.
func (r *runner) resumeOrResetExecuting(ctx context.Context) error {
	return resumeOrResetExecuting(
		ctx,
		r.inProgressDir,
		r.promptManager,
		r.containerChecker,
		r.notifier,
		r.projectName,
	)
}

// createDirectories creates all eight lifecycle directories if they don't exist.
func (r *runner) createDirectories(ctx context.Context) error {
	return createDirectories(ctx, []string{
		r.inboxDir,
		r.inProgressDir,
		r.completedDir,
		r.logDir,
		r.specsInboxDir,
		r.specsInProgressDir,
		r.specsCompletedDir,
		r.specsLogDir,
	})
}
