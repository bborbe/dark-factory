// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

// QueueCommand executes the queue subcommand.
//
//counterfeiter:generate -o ../../mocks/queue-command.go --fake-name QueueCommand . QueueCommand
type QueueCommand interface {
	Run(ctx context.Context, args []string) error
}

// queueCommand implements QueueCommand.
type queueCommand struct {
	inboxDir      string
	queueDir      string
	promptManager prompt.Manager
}

// NewQueueCommand creates a new QueueCommand.
func NewQueueCommand(
	inboxDir string,
	queueDir string,
	promptManager prompt.Manager,
) QueueCommand {
	return &queueCommand{
		inboxDir:      inboxDir,
		queueDir:      queueDir,
		promptManager: promptManager,
	}
}

// Run executes the queue command.
func (q *queueCommand) Run(ctx context.Context, args []string) error {
	// If specific file provided, queue that file
	if len(args) > 0 {
		return q.queueFile(ctx, args[0])
	}

	// Otherwise queue all files in inbox
	return q.queueAll(ctx)
}

// queueFile queues a specific file from inbox to queue.
func (q *queueCommand) queueFile(ctx context.Context, filename string) error {
	oldPath := filepath.Join(q.inboxDir, filename)

	// Check if file exists
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return errors.Errorf(ctx, "file not found: %s", filename)
	}

	// Move to queue directory
	newFilename, err := q.moveToQueue(ctx, filename)
	if err != nil {
		return err
	}

	fmt.Printf("queued: %s -> %s\n", filename, newFilename)
	return nil
}

// queueAll queues all .md files from inbox to queue.
func (q *queueCommand) queueAll(ctx context.Context) error {
	entries, err := os.ReadDir(q.inboxDir)
	if err != nil {
		return errors.Wrap(ctx, err, "read inbox directory")
	}

	queued := 0
	for _, entry := range entries {
		// Skip directories and non-.md files
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		newFilename, err := q.moveToQueue(ctx, entry.Name())
		if err != nil {
			return err
		}

		fmt.Printf("queued: %s -> %s\n", entry.Name(), newFilename)
		queued++
	}

	if queued == 0 {
		fmt.Println("no files to queue")
	}

	return nil
}

// moveToQueue moves a file from inbox to queue with normalization.
func (q *queueCommand) moveToQueue(ctx context.Context, filename string) (string, error) {
	oldPath := filepath.Join(q.inboxDir, filename)

	// Move file to queue directory with same name
	newPath := filepath.Join(q.queueDir, filename)
	if err := os.Rename(oldPath, newPath); err != nil {
		return "", errors.Wrap(ctx, err, "move file to queue")
	}

	// Set status to queued
	if err := prompt.SetStatus(ctx, newPath, string(prompt.StatusQueued)); err != nil {
		return "", errors.Wrap(ctx, err, "set queued status")
	}

	// Normalize filenames in queue (this will add NNN- prefix if needed)
	renames, err := q.promptManager.NormalizeFilenames(ctx, q.queueDir)
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
