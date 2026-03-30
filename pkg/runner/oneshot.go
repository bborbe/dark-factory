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

	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/generator"
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/slugmigrator"
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
	containerChecker executor.ContainerChecker,
	autoApprove bool,
	slugMigrator slugmigrator.Migrator,
	mover prompt.FileMover,
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
	containerChecker      executor.ContainerChecker
	autoApprove           bool
	slugMigrator          slugmigrator.Migrator
	mover                 prompt.FileMover
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

	// Selectively resume or reset executing prompts based on container liveness
	if err := r.resumeOrResetExecuting(ctx); err != nil {
		return errors.Wrap(ctx, err, "resume or reset executing prompts")
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
	if err := r.slugMigrator.MigrateDirs(ctx, []string{
		r.inboxDir, r.inProgressDir, r.completedDir, r.logDir,
	}); err != nil {
		return errors.Wrap(ctx, err, "migrate spec slugs")
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

// resumeOrResetExecuting selectively resumes or resets executing prompts based on container liveness.
// reindexAll runs the full reindex sequence for this runner's spec and prompt dirs.
func (r *oneShotRunner) reindexAll(ctx context.Context) error {
	specDirs := []string{r.specsInboxDir, r.specsInProgressDir, r.specsCompletedDir, r.specsLogDir}
	promptDirs := []string{r.inboxDir, r.inProgressDir, r.completedDir, r.logDir}
	return reindexAll(ctx, specDirs, promptDirs, r.mover, r.currentDateTimeGetter)
}

func (r *oneShotRunner) resumeOrResetExecuting(ctx context.Context) error {
	return resumeOrResetExecuting(
		ctx,
		r.inProgressDir,
		r.promptManager,
		r.containerChecker,
		nil,
		"",
	)
}

// generateFromApprovedSpecs scans specsInProgressDir for approved specs, generates prompts
// from each, and moves the generated prompts from inbox to in-progress. Returns the count
// of prompts moved to in-progress so the caller can decide whether to loop again.
func (r *oneShotRunner) generateFromApprovedSpecs(ctx context.Context) (int, error) {
	if r.specGenerator == nil {
		return 0, nil
	}

	foundApproved, err := r.generateSpecPrompts(ctx)
	if err != nil {
		return 0, err
	}
	if !foundApproved {
		return 0, nil
	}

	if r.autoApprove {
		moved, err := r.approveInboxPrompts(ctx)
		if err != nil {
			return 0, errors.Wrap(ctx, err, "approve inbox prompts")
		}
		return moved, nil
	}

	r.logInboxPrompts()
	return 0, nil
}

// generateSpecPrompts iterates approved specs and runs the generator for each.
// Returns true if at least one approved spec was found.
func (r *oneShotRunner) generateSpecPrompts(ctx context.Context) (bool, error) {
	entries, err := os.ReadDir(r.specsInProgressDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrap(ctx, err, "read specs in-progress dir")
	}

	foundApproved := false
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return false, errors.Wrap(ctx, ctx.Err(), "context cancelled during spec generation")
		default:
		}
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
	return foundApproved, nil
}

// logInboxPrompts logs each .md file in inboxDir and a hint for manual approval.
func (r *oneShotRunner) logInboxPrompts() {
	if inboxEntries, err := os.ReadDir(r.inboxDir); err == nil {
		for _, e := range inboxEntries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				slog.Info("generated prompt awaiting review", "file", e.Name())
			}
		}
	}
	slog.Info("generated prompts left in inbox — approve with: dark-factory prompt approve <name>")
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
		select {
		case <-ctx.Done():
			return 0, errors.Wrap(ctx, ctx.Err(), "context cancelled during inbox approval")
		default:
		}
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
	return normalizeFilenames(ctx, r.promptManager, r.inProgressDir)
}

func (r *oneShotRunner) migrateQueueDir(ctx context.Context) error {
	return migrateQueueDir(ctx, r.inProgressDir)
}

func (r *oneShotRunner) createDirectories(ctx context.Context) error {
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
