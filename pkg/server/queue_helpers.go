// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

// queueSingleFile moves a single file from inbox to queue.
func queueSingleFile(
	ctx context.Context,
	inboxDir string,
	queueDir string,
	promptManager prompt.Manager,
	filename string,
) (QueuedFile, error) {
	oldPath := filepath.Join(inboxDir, filename)

	// Check if file exists
	if _, err := os.Stat(oldPath); err != nil {
		// Don't wrap os.Stat errors - caller checks for os.IsNotExist
		return QueuedFile{}, err
	}

	newFilename, err := moveToQueue(ctx, inboxDir, queueDir, promptManager, filename)
	if err != nil {
		return QueuedFile{}, errors.Wrap(ctx, err, "move to queue")
	}

	return QueuedFile{Old: filename, New: newFilename}, nil
}

// queueAllFiles moves all .md files from inbox to queue.
func queueAllFiles(
	ctx context.Context,
	inboxDir string,
	queueDir string,
	promptManager prompt.Manager,
) ([]QueuedFile, error) {
	entries, err := os.ReadDir(inboxDir)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "read inbox directory")
	}

	queued := make([]QueuedFile, 0, len(entries))
	for _, entry := range entries {
		// Skip directories and non-.md files
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		newFilename, err := moveToQueue(ctx, inboxDir, queueDir, promptManager, entry.Name())
		if err != nil {
			return nil, errors.Wrap(ctx, err, "move to queue")
		}

		queued = append(queued, QueuedFile{Old: entry.Name(), New: newFilename})
	}

	return queued, nil
}

// moveToQueue moves a file from inbox to queue with normalization.
func moveToQueue(
	ctx context.Context,
	inboxDir string,
	queueDir string,
	promptManager prompt.Manager,
	filename string,
) (string, error) {
	oldPath := filepath.Join(inboxDir, filename)

	// Move file to queue directory with same name
	newPath := filepath.Join(queueDir, filename)
	if err := os.Rename(oldPath, newPath); err != nil {
		return "", errors.Wrap(ctx, err, "move file to queue")
	}

	// Set status to queued
	if err := prompt.SetStatus(ctx, newPath, string(prompt.StatusQueued)); err != nil {
		return "", errors.Wrap(ctx, err, "set queued status")
	}

	// Normalize filenames in queue (this will add NNN- prefix if needed)
	renames, err := promptManager.NormalizeFilenames(ctx, queueDir)
	if err != nil {
		return "", errors.Wrap(ctx, err, "normalize filenames")
	}

	// Find the new filename after normalization
	for _, rename := range renames {
		if filepath.Base(rename.OldPath) == filename {
			return filepath.Base(rename.NewPath), nil
		}
	}

	// If no rename happened, the file kept its original name
	return filename, nil
}
