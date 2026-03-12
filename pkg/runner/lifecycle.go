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

	"github.com/bborbe/dark-factory/pkg/prompt"
)

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
