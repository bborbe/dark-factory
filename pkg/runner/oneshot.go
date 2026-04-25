// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

import (
	"context"
	"log/slog"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/generator"
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/slugmigrator"
)

// OneShotRunner processes all queued prompts and exits cleanly.
type OneShotRunner interface {
	Run(ctx context.Context) error
}

// NewOneShotRunner creates a new OneShotRunner.
// startupLogger is an optional func called after lock acquisition to emit the effective-config log line.
// Pass nil to skip the startup log.
func NewOneShotRunner(
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
	proc processor.Processor,
	specGen generator.SpecGenerator,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	containerChecker executor.ContainerChecker,
	autoApprove bool,
	slugMigrator slugmigrator.Migrator,
	mover prompt.FileMover,
	startupLogger func(),
) OneShotRunner {
	return &oneShotRunner{
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
		processor:             proc,
		specGenerator:         specGen,
		currentDateTimeGetter: currentDateTimeGetter,
		containerChecker:      containerChecker,
		autoApprove:           autoApprove,
		slugMigrator:          slugMigrator,
		mover:                 mover,
		startupLogger:         startupLogger,
	}
}

// oneShotRunner processes all queued prompts and exits.
type oneShotRunner struct {
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
	processor             processor.Processor
	specGenerator         generator.SpecGenerator
	currentDateTimeGetter libtime.CurrentDateTimeGetter
	containerChecker      executor.ContainerChecker
	autoApprove           bool
	slugMigrator          slugmigrator.Migrator
	mover                 prompt.FileMover
	startupLogger         func()
}

// Run acquires the lock, runs startup steps, then delegates to processor.Process.
// The onIdle callback injected into the processor drives exit for one-shot mode.
func (r *oneShotRunner) Run(ctx context.Context) error {
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

	if r.startupLogger != nil {
		r.startupLogger()
	}

	// Run the six shared startup steps (migrateQueueDir, createDirectories,
	// resumeOrResetExecuting, reindexAll, normalizeFilenames, migrateSpecSlugs).
	if err := startupSequence(ctx, r.startupDeps()); err != nil {
		return errors.Wrap(ctx, err, "startup sequence")
	}

	// Loop: generate from approved specs, then drain queue; repeat until idle.
	return r.drainLoop(ctx)
}

// drainLoop delegates to the processor's event loop. The onIdle callback (injected via factory)
// drives exit: daemon mode logs on idle ticks; one-shot mode cancels the context to exit.
func (r *oneShotRunner) drainLoop(ctx context.Context) error {
	return r.processor.Process(ctx)
}

// startupDeps builds a StartupDeps from this runner's fields.
func (r *oneShotRunner) startupDeps() StartupDeps {
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
		Notifier:              nil, // oneshot has no notifier field
		ProjectName:           "",  // oneshot has no projectName field
		SlugMigrator:          r.slugMigrator,
		Mover:                 r.mover,
		CurrentDateTimeGetter: r.currentDateTimeGetter,
	}
}
