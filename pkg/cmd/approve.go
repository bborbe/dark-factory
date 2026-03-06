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

//counterfeiter:generate -o ../../mocks/approve-command.go --fake-name ApproveCommand . ApproveCommand

// ApproveCommand executes the approve subcommand.
type ApproveCommand interface {
	Run(ctx context.Context, args []string) error
}

// approveCommand implements ApproveCommand.
type approveCommand struct {
	inboxDir      string
	queueDir      string
	promptManager prompt.Manager
}

// NewApproveCommand creates a new ApproveCommand.
func NewApproveCommand(
	inboxDir string,
	queueDir string,
	promptManager prompt.Manager,
) ApproveCommand {
	return &approveCommand{
		inboxDir:      inboxDir,
		queueDir:      queueDir,
		promptManager: promptManager,
	}
}

// Run executes the approve command.
func (a *approveCommand) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.Errorf(ctx, "usage: dark-factory approve <file>")
	}
	return a.approveByID(ctx, args[0])
}

// approveByID approves a prompt matching a short ID or exact filename.
func (a *approveCommand) approveByID(ctx context.Context, id string) error {
	// Search inbox first
	inboxMatch, err := a.findFile(a.inboxDir, id)
	if err != nil {
		return errors.Wrap(ctx, err, "search inbox")
	}
	if inboxMatch != "" {
		return a.approveFromInbox(ctx, inboxMatch)
	}

	// Then search queue
	queueMatch, err := a.findFile(a.queueDir, id)
	if err != nil {
		return errors.Wrap(ctx, err, "search queue")
	}
	if queueMatch != "" {
		return a.approveInQueue(ctx, queueMatch)
	}

	return errors.Errorf(ctx, "file not found: %s", id)
}

// findFile finds a file in a directory matching the given ID (exact name or NNN- prefix match).
func (a *approveCommand) findFile(dir string, id string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := entry.Name()
		if name == id || strings.HasPrefix(name, id+"-") {
			return name, nil
		}
	}
	return "", nil
}

// approveFromInbox moves a file from inbox to queue and sets status to queued.
func (a *approveCommand) approveFromInbox(ctx context.Context, filename string) error {
	oldPath := filepath.Join(a.inboxDir, filename)
	newPath := filepath.Join(a.queueDir, filename)

	if err := os.Rename(oldPath, newPath); err != nil {
		return errors.Wrap(ctx, err, "move file to queue")
	}

	pf, err := prompt.Load(ctx, newPath)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}
	pf.MarkQueued()
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save prompt")
	}

	if _, err := a.promptManager.NormalizeFilenames(ctx, a.queueDir); err != nil {
		return errors.Wrap(ctx, err, "normalize filenames")
	}

	fmt.Printf("approved: %s\n", filename)
	return nil
}

// approveInQueue sets a prompt already in the queue to queued status.
func (a *approveCommand) approveInQueue(ctx context.Context, filename string) error {
	path := filepath.Join(a.queueDir, filename)

	pf, err := prompt.Load(ctx, path)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.MarkQueued()
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save prompt")
	}

	fmt.Printf("approved: %s\n", filename)
	return nil
}
