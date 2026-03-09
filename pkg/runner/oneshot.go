// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

// OneShotRunner processes all queued prompts and exits cleanly.
type OneShotRunner interface {
	Run(ctx context.Context) error
}

// oneShotRunner processes all queued prompts and exits.
type oneShotRunner struct {
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
	processor          processor.Processor
}

// NewOneShotRunner creates a new OneShotRunner.
func NewOneShotRunner(
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
	proc processor.Processor,
) OneShotRunner {
	return &oneShotRunner{
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
		processor:          proc,
	}
}

// Run acquires the lock, initializes directories, drains the queue, and exits.
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

	// Migrate old prompts/queue/ → prompts/in-progress/ if needed
	if err := r.migrateQueueDir(ctx); err != nil {
		return errors.Wrap(ctx, err, "migrate queue dir")
	}

	// Create directories if they don't exist
	if err := r.createDirectories(ctx); err != nil {
		return errors.Wrap(ctx, err, "create directories")
	}

	// Reset any stuck "executing" prompts from previous crash
	if err := r.promptManager.ResetExecuting(ctx); err != nil {
		return errors.Wrap(ctx, err, "reset executing prompts")
	}

	// Normalize filenames before processing
	if err := r.normalizeFilenames(ctx); err != nil {
		return errors.Wrap(ctx, err, "normalize filenames")
	}

	// Process all queued prompts and return
	return r.processor.ProcessQueue(ctx)
}

func (r *oneShotRunner) normalizeFilenames(ctx context.Context) error {
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

func (r *oneShotRunner) migrateQueueDir(ctx context.Context) error {
	oldQueue := filepath.Join(filepath.Dir(r.inProgressDir), "queue")
	if _, err := os.Stat(oldQueue); os.IsNotExist(err) {
		return nil
	}
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

func (r *oneShotRunner) createDirectories(ctx context.Context) error {
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
