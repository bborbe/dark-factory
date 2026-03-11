// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bborbe/errors"
	"github.com/bborbe/run"

	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/review"
	"github.com/bborbe/dark-factory/pkg/server"
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
	n notifier.Notifier,
) Runner {
	return &runner{
		inboxDir:           inboxDir,
		inProgressDir:      inProgressDir,
		completedDir:       completedDir,
		logDir:             logDir,
		specsInboxDir:      specsInboxDir,
		specsInProgressDir: specsInProgressDir,
		specsCompletedDir:  specsCompletedDir,
		specsLogDir:        specsLogDir,
		promptManager:      promptManager,
		locker:             locker,
		watcher:            watcher,
		processor:          processor,
		server:             server,
		reviewPoller:       reviewPoller,
		specWatcher:        specWatcher,
		projectName:        projectName,
		notifier:           n,
	}
}

// runner orchestrates the main processing loop.
type runner struct {
	inboxDir           string
	inProgressDir      string
	completedDir       string
	logDir             string
	specsInboxDir      string
	specsInProgressDir string
	specsCompletedDir  string
	specsLogDir        string
	promptManager      prompt.Manager
	locker             lock.Locker
	watcher            watcher.Watcher
	processor          processor.Processor
	server             server.Server
	reviewPoller       review.ReviewPoller
	specWatcher        specwatcher.SpecWatcher
	projectName        string
	notifier           notifier.Notifier
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

	// Notify about stuck containers before resetting them
	r.notifyStuckContainers(ctx)

	// Reset any stuck "executing" prompts from previous crash
	if err := r.promptManager.ResetExecuting(ctx); err != nil {
		return errors.Wrap(ctx, err, "reset executing prompts")
	}

	// Normalize filenames before processing
	if err := r.normalizeFilenames(ctx); err != nil {
		return errors.Wrap(ctx, err, "normalize filenames")
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

// normalizeFilenames normalizes filenames in the in-progress directory only.
// The inbox directory is not normalized as it contains draft files.
func (r *runner) normalizeFilenames(ctx context.Context) error {
	renames, err := r.promptManager.NormalizeFilenames(ctx, r.inProgressDir)
	if err != nil {
		return errors.Wrap(ctx, err, "normalize queue filenames")
	}
	for _, rename := range renames {
		slog.Debug("renamed file",
			"from", filepath.Base(rename.OldPath),
			"to", filepath.Base(rename.NewPath))
	}
	return nil
}

// migrateQueueDir renames prompts/queue/ → prompts/in-progress/ if the old path
// exists and the new path does not. This is a one-time migration.
func (r *runner) migrateQueueDir(ctx context.Context) error {
	oldQueue := filepath.Join(filepath.Dir(r.inProgressDir), "queue")
	// Only migrate if old dir exists
	if _, err := os.Stat(oldQueue); os.IsNotExist(err) {
		return nil
	}
	// Skip if new dir already exists (migration already done or manually created)
	if _, err := os.Stat(r.inProgressDir); err == nil {
		slog.Info("skipping queue migration: in-progress dir already exists",
			"old", oldQueue, "new", r.inProgressDir)
		return nil
	}
	if err := os.Rename(oldQueue, r.inProgressDir); err != nil {
		return errors.Wrap(ctx, err, "migrate queue dir to in-progress")
	}
	slog.Info("migrated queue dir to in-progress", "old", oldQueue, "new", r.inProgressDir)
	return nil
}

// notifyStuckContainers scans inProgressDir for prompts with "executing" status
// and fires a stuck_container notification for each one found before ResetExecuting clears them.
func (r *runner) notifyStuckContainers(ctx context.Context) {
	entries, err := os.ReadDir(r.inProgressDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(r.inProgressDir, entry.Name())
		fm, err := r.promptManager.ReadFrontmatter(ctx, path)
		if err != nil || fm == nil {
			continue
		}
		if prompt.PromptStatus(fm.Status) == prompt.ExecutingPromptStatus {
			_ = r.notifier.Notify(ctx, notifier.Event{
				ProjectName: r.projectName,
				EventType:   "stuck_container",
				PromptName:  entry.Name(),
			})
		}
	}
}

// createDirectories creates all eight lifecycle directories if they don't exist.
func (r *runner) createDirectories(ctx context.Context) error {
	dirs := []string{
		r.inboxDir,
		r.inProgressDir,
		r.completedDir,
		r.logDir,
		r.specsInboxDir,
		r.specsInProgressDir,
		r.specsCompletedDir,
		r.specsLogDir,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return errors.Wrapf(ctx, err, "create directory %s", dir)
		}
	}
	return nil
}
