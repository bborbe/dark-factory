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
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/reindex"
)

// reindexAll runs the full reindex sequence:
//  1. Reindex spec dirs (resolve cross-directory spec number conflicts)
//  2. Update spec cross-references in prompt dirs (propagate spec renames)
//  3. Reindex prompt dirs (resolve cross-directory prompt number conflicts)
func reindexAll(
	ctx context.Context,
	specDirs []string,
	promptDirs []string,
	mover prompt.FileMover,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	// Step 1: Reindex spec files
	specReindexer := reindex.NewReindexer(specDirs, mover)
	specRenames, err := specReindexer.Reindex(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "reindex spec files")
	}

	// Step 2: Propagate spec renames to prompt cross-references
	if len(specRenames) > 0 {
		if _, err := reindex.UpdateSpecRefs(ctx, specRenames, promptDirs, mover, currentDateTimeGetter); err != nil {
			return errors.Wrap(ctx, err, "update spec refs")
		}
	}

	// Step 3: Reindex prompt files
	promptReindexer := reindex.NewReindexer(promptDirs, mover)
	if _, err := promptReindexer.Reindex(ctx); err != nil {
		return errors.Wrap(ctx, err, "reindex prompt files")
	}

	return nil
}

// normalizeFilenames normalizes filenames in the given inProgressDir using the
// provided prompt.Manager and logs each rename at debug level.
func normalizeFilenames(ctx context.Context, mgr prompt.Manager, inProgressDir string) error {
	renames, err := mgr.NormalizeFilenames(ctx, inProgressDir)
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

// migrateQueueDir renames prompts/queue/ → prompts/in-progress/ (inProgressDir) if the
// old path exists and the new path does not. This is a one-time migration.
func migrateQueueDir(ctx context.Context, inProgressDir string) error {
	oldQueue := filepath.Join(filepath.Dir(inProgressDir), "queue")
	// Only migrate if old dir exists.
	if _, err := os.Stat(oldQueue); os.IsNotExist(err) {
		return nil
	}
	// Skip if new dir already exists (migration already done or manually created).
	if _, err := os.Stat(inProgressDir); err == nil {
		slog.Info("skipping queue migration: in-progress dir already exists",
			"old", oldQueue, "new", inProgressDir)
		return nil
	}
	if err := os.Rename(oldQueue, inProgressDir); err != nil {
		return errors.Wrap(ctx, err, "migrate queue dir to in-progress")
	}
	slog.Info("migrated queue dir to in-progress", "old", oldQueue, "new", inProgressDir)
	return nil
}

// createDirectories creates each directory in dirs using os.MkdirAll with mode 0750.
func createDirectories(ctx context.Context, dirs []string) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return errors.Wrapf(ctx, err, "create directory %s", dir)
		}
	}
	return nil
}

// resumeOrResetExecuting scans inProgressDir for prompts with "executing" status.
// If the container is still running, the prompt is left in executing state (to be resumed).
// If the container is gone, a stuck_container notification is fired and the prompt is reset to approved.
// n may be nil (no notification fired) and projectName may be empty.
func resumeOrResetExecuting(
	ctx context.Context,
	inProgressDir string,
	mgr prompt.Manager,
	checker executor.ContainerChecker,
	n notifier.Notifier,
	projectName string,
) error {
	entries, err := os.ReadDir(inProgressDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(ctx, err, "read in-progress dir")
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if err := resumeOrResetExecutingEntry(ctx, inProgressDir, entry.Name(), mgr, checker, n, projectName); err != nil {
			return err
		}
	}
	return nil
}

// resumeOrResetExecutingEntry handles a single prompt file: checks liveness and resumes or resets.
func resumeOrResetExecutingEntry(
	ctx context.Context,
	inProgressDir string,
	name string,
	mgr prompt.Manager,
	checker executor.ContainerChecker,
	n notifier.Notifier,
	projectName string,
) error {
	path := filepath.Join(inProgressDir, name)
	pf, err := mgr.Load(ctx, path)
	if err != nil || pf == nil {
		return nil
	}
	if prompt.PromptStatus(pf.Frontmatter.Status) != prompt.ExecutingPromptStatus {
		return nil
	}
	containerName := pf.Frontmatter.Container
	running, err := checker.IsRunning(ctx, containerName)
	if err != nil {
		slog.Warn("failed to check container liveness, resetting prompt",
			"file", name, "container", containerName, "error", err)
		running = false
	}
	if running {
		slog.Info(
			"resuming prompt, container still running",
			"file",
			name,
			"container",
			containerName,
		)
		return nil
	}
	slog.Info("resetting prompt, container not found", "file", name, "container", containerName)
	if n != nil {
		_ = n.Notify(ctx, notifier.Event{
			ProjectName: projectName,
			EventType:   "stuck_container",
			PromptName:  name,
		})
	}
	pf.MarkApproved()
	return errors.Wrap(ctx, pf.Save(ctx), "reset executing prompt")
}
