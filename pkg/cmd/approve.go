// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/approve-command.go --fake-name ApproveCommand . ApproveCommand

// ApproveCommand executes the approve subcommand.
type ApproveCommand interface {
	Run(ctx context.Context, args []string) error
}

// approveCommand implements ApproveCommand.
type approveCommand struct {
	inboxDir              string
	queueDir              string
	promptManager         PromptManager
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewApproveCommand creates a new ApproveCommand.
func NewApproveCommand(
	inboxDir string,
	queueDir string,
	promptManager PromptManager,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) ApproveCommand {
	return &approveCommand{
		inboxDir:              inboxDir,
		queueDir:              queueDir,
		promptManager:         promptManager,
		currentDateTimeGetter: currentDateTimeGetter,
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
	if inboxPath, err := FindPromptFile(ctx, a.inboxDir, id); err == nil {
		return a.approveFromInbox(ctx, inboxPath)
	}

	// Then search queue
	if queuePath, err := FindPromptFile(ctx, a.queueDir, id); err == nil {
		return a.approveInQueue(ctx, queuePath)
	}

	return errors.Errorf(ctx, "file not found: %s", id)
}

// approveFromInbox moves a file from inbox to queue and sets status to approved.
// Any numeric prefix is stripped so NormalizeFilenames assigns the correct number.
func (a *approveCommand) approveFromInbox(ctx context.Context, oldPath string) error {
	filename := prompt.StripNumberPrefix(filepath.Base(oldPath))
	newPath := filepath.Join(a.queueDir, filename)

	if err := os.Rename(oldPath, newPath); err != nil {
		return errors.Wrap(ctx, err, "move file to queue")
	}

	pf, err := prompt.Load(ctx, newPath, a.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}
	pf.MarkApproved()
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save prompt")
	}

	if _, err := a.promptManager.NormalizeFilenames(ctx, a.queueDir); err != nil {
		return errors.Wrap(ctx, err, "normalize filenames")
	}

	fmt.Printf("approved: %s\n", filename)
	return nil
}

// approveInQueue sets a prompt already in the queue to approved status.
func (a *approveCommand) approveInQueue(ctx context.Context, path string) error {
	pf, err := prompt.Load(ctx, path, a.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.MarkApproved()
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save prompt")
	}

	fmt.Printf("approved: %s\n", filepath.Base(path))
	return nil
}
