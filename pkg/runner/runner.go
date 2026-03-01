// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/bborbe/errors"
	"github.com/bborbe/run"

	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/server"
	"github.com/bborbe/dark-factory/pkg/watcher"
)

// Runner orchestrates the main processing loop.
type Runner interface {
	Run(ctx context.Context) error
}

// runner orchestrates the main processing loop.
type runner struct {
	inboxDir      string
	queueDir      string
	completedDir  string
	promptManager prompt.Manager
	locker        lock.Locker
	watcher       watcher.Watcher
	processor     processor.Processor
	server        server.Server
}

// NewRunner creates a new Runner.
func NewRunner(
	inboxDir string,
	queueDir string,
	completedDir string,
	promptManager prompt.Manager,
	locker lock.Locker,
	watcher watcher.Watcher,
	processor processor.Processor,
	server server.Server,
) Runner {
	return &runner{
		inboxDir:      inboxDir,
		queueDir:      queueDir,
		completedDir:  completedDir,
		promptManager: promptManager,
		locker:        locker,
		watcher:       watcher,
		processor:     processor,
		server:        server,
	}
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
			log.Printf("dark-factory: failed to release lock: %v", err)
		}
	}()

	log.Printf("dark-factory: acquired lock .dark-factory.lock")

	// Set up signal handling
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create directories if they don't exist
	if err := r.createDirectories(ctx); err != nil {
		return errors.Wrap(ctx, err, "create directories")
	}

	log.Printf("dark-factory: watching %s for queued prompts...", r.queueDir)

	// Reset any stuck "executing" prompts from previous crash
	if err := r.promptManager.ResetExecuting(ctx); err != nil {
		return errors.Wrap(ctx, err, "reset executing prompts")
	}

	// Normalize filenames before processing
	if err := r.normalizeFilenames(ctx); err != nil {
		return errors.Wrap(ctx, err, "normalize filenames")
	}

	// Run watcher, processor, and server in parallel
	// If any fails, context cancels the others automatically
	runners := []run.Func{
		r.watcher.Watch,
		r.processor.Process,
	}
	if r.server != nil {
		runners = append(runners, r.server.ListenAndServe)
	}
	return run.CancelOnFirstError(ctx, runners...)
}

// normalizeFilenames normalizes filenames in the queue directory only.
// The inbox directory is not normalized as it contains draft files.
func (r *runner) normalizeFilenames(ctx context.Context) error {
	renames, err := r.promptManager.NormalizeFilenames(ctx, r.queueDir)
	if err != nil {
		return errors.Wrap(ctx, err, "normalize queue filenames")
	}
	for _, rename := range renames {
		log.Printf("dark-factory: renamed %s -> %s",
			filepath.Base(rename.OldPath), filepath.Base(rename.NewPath))
	}
	return nil
}

// createDirectories creates the inbox, queue, and completed directories if they don't exist.
func (r *runner) createDirectories(ctx context.Context) error {
	dirs := []string{r.inboxDir, r.queueDir, r.completedDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return errors.Wrapf(ctx, err, "create directory %s", dir)
		}
	}
	return nil
}
