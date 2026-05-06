// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/cancel-command.go --fake-name CancelCommand . CancelCommand

// CancelCommand executes the cancel subcommand.
type CancelCommand interface {
	Run(ctx context.Context, args []string) error
}

// cancelCommand implements CancelCommand.
type cancelCommand struct {
	queueDir      string
	cancelledDir  string
	promptManager PromptManager
}

// NewCancelCommand creates a new CancelCommand.
func NewCancelCommand(
	queueDir string,
	cancelledDir string,
	promptManager PromptManager,
) CancelCommand {
	return &cancelCommand{
		queueDir:      queueDir,
		cancelledDir:  cancelledDir,
		promptManager: promptManager,
	}
}

// Run executes the cancel command.
func (a *cancelCommand) Run(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.Errorf(ctx, "usage: dark-factory prompt cancel <id>")
	}
	id := args[0]

	// Primary search: in-progress (the queue).
	path, err := FindPromptFile(ctx, a.queueDir, id)
	if err != nil {
		return a.handleNotFound(ctx, id)
	}

	pf, err := a.promptManager.Load(ctx, path)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	switch prompt.PromptStatus(pf.Frontmatter.Status) {
	case prompt.ApprovedPromptStatus:
		// Not yet running: mark cancelled and move the file immediately.
		if err := a.promptManager.MoveToCancelled(ctx, path); err != nil {
			return errors.Wrap(ctx, err, "move to cancelled")
		}
		fmt.Printf("cancelled: %s\n", filepath.Base(path))
		return nil

	case prompt.ExecutingPromptStatus:
		// Container is running: write status=cancelled to trigger the
		// cancellationwatcher (daemon-side), which will stop the container.
		// The processor moves the file to cancelled/ after the container exits.
		pf.MarkCancelled()
		if err := pf.Save(ctx); err != nil {
			return errors.Wrap(ctx, err, "save prompt")
		}
		fmt.Printf("cancelled: %s\n", filepath.Base(path))
		return nil

	default:
		return errors.Errorf(
			ctx,
			"cannot cancel prompt with status %q (only approved or executing prompts can be cancelled)",
			pf.Frontmatter.Status,
		)
	}
}

// handleNotFound checks whether the prompt was already moved to cancelled/ (idempotency).
// Returns nil if the prompt is already in cancelled/, otherwise returns a not-found error.
func (a *cancelCommand) handleNotFound(ctx context.Context, id string) error {
	if a.cancelledDir == "" {
		return errors.Errorf(ctx, "prompt not found: %s", id)
	}
	cancelledPath, findErr := FindPromptFile(ctx, a.cancelledDir, id)
	if findErr != nil {
		return errors.Errorf(ctx, "prompt not found: %s", id)
	}
	pf, loadErr := a.promptManager.Load(ctx, cancelledPath)
	if loadErr != nil {
		return errors.Errorf(ctx, "prompt not found: %s", id)
	}
	if pf.Frontmatter.Status != string(prompt.CancelledPromptStatus) {
		return errors.Errorf(ctx, "prompt not found: %s", id)
	}
	fmt.Printf("already cancelled: %s\n", filepath.Base(cancelledPath))
	return nil
}
