// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/generator"
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

// OneShotRunner processes all queued prompts and exits cleanly.
type OneShotRunner interface {
	Run(ctx context.Context) error
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
	specGen generator.SpecGenerator,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
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
	promptManager         prompt.Manager
	locker                lock.Locker
	processor             processor.Processor
	specGenerator         generator.SpecGenerator
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// Run acquires the lock, initializes directories, then loops: generate prompts from approved
// specs, drain the queue, repeat until no approved specs and no queued prompts remain.
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

	// Loop: generate from approved specs, then drain queue; repeat until idle.
	for {
		generated, err := r.generateFromApprovedSpecs(ctx)
		if err != nil {
			return errors.Wrap(ctx, err, "generate from approved specs")
		}

		queued, err := r.promptManager.ListQueued(ctx)
		if err != nil {
			return errors.Wrap(ctx, err, "list queued prompts")
		}

		if err := r.processor.ProcessQueue(ctx); err != nil {
			return errors.Wrap(ctx, err, "process queue")
		}

		if generated == 0 && len(queued) == 0 {
			break
		}
	}

	return nil
}

// generateFromApprovedSpecs scans specsInProgressDir for approved specs, generates prompts
// from each, and moves the generated prompts from inbox to in-progress. Returns the count
// of prompts moved to in-progress so the caller can decide whether to loop again.
func (r *oneShotRunner) generateFromApprovedSpecs(ctx context.Context) (int, error) {
	if r.specGenerator == nil {
		return 0, nil
	}

	entries, err := os.ReadDir(r.specsInProgressDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, errors.Wrap(ctx, err, "read specs in-progress dir")
	}

	foundApproved := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		specPath := filepath.Join(r.specsInProgressDir, entry.Name())
		sf, err := spec.Load(ctx, specPath, r.currentDateTimeGetter)
		if err != nil {
			slog.Warn("failed to load spec", "path", specPath, "error", err)
			continue
		}
		if sf.Frontmatter.Status != string(spec.StatusApproved) {
			continue
		}
		foundApproved = true
		slog.Info("generating prompts from approved spec", "spec", entry.Name())
		if err := r.specGenerator.Generate(ctx, specPath); err != nil {
			slog.Warn("spec generation failed", "spec", entry.Name(), "error", err)
		}
	}

	if !foundApproved {
		return 0, nil
	}

	moved, err := r.approveInboxPrompts(ctx)
	if err != nil {
		return 0, errors.Wrap(ctx, err, "approve inbox prompts")
	}
	return moved, nil
}

// approveInboxPrompts moves all .md files from inboxDir to inProgressDir, marks them
// approved, and normalizes filenames so the processor can pick them up. Returns count
// of files moved.
func (r *oneShotRunner) approveInboxPrompts(ctx context.Context) (int, error) {
	entries, err := os.ReadDir(r.inboxDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, errors.Wrap(ctx, err, "read inbox dir")
	}

	moved := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		oldPath := filepath.Join(r.inboxDir, entry.Name())
		newPath := filepath.Join(r.inProgressDir, entry.Name())
		if err := os.Rename(oldPath, newPath); err != nil {
			return moved, errors.Wrapf(ctx, err, "move %s to in-progress", entry.Name())
		}
		moved++
		pf, err := prompt.Load(ctx, newPath, r.currentDateTimeGetter)
		if err != nil {
			slog.Warn("failed to load moved prompt", "path", newPath, "error", err)
			continue
		}
		pf.MarkApproved()
		if err := pf.Save(ctx); err != nil {
			slog.Warn("failed to save approved prompt", "path", newPath, "error", err)
			continue
		}
		slog.Info("auto-approved generated prompt", "file", entry.Name())
	}

	if _, err := r.promptManager.NormalizeFilenames(ctx, r.inProgressDir); err != nil {
		return moved, errors.Wrap(ctx, err, "normalize filenames after approve")
	}

	return moved, nil
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
