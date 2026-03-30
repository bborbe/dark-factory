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

//counterfeiter:generate -o ../../mocks/unapprove-command.go --fake-name UnapproveCommand . UnapproveCommand

// UnapproveCommand executes the unapprove subcommand.
type UnapproveCommand interface {
	Run(ctx context.Context, args []string) error
}

// unapproveCommand implements UnapproveCommand.
type unapproveCommand struct {
	inboxDir              string
	queueDir              string
	promptManager         prompt.Manager
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewUnapproveCommand creates a new UnapproveCommand.
func NewUnapproveCommand(
	inboxDir string,
	queueDir string,
	promptManager prompt.Manager,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) UnapproveCommand {
	return &unapproveCommand{
		inboxDir:              inboxDir,
		queueDir:              queueDir,
		promptManager:         promptManager,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// Run executes the unapprove command.
func (u *unapproveCommand) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.Errorf(ctx, "usage: dark-factory prompt unapprove <file>")
	}
	return u.unapproveByID(ctx, args[0])
}

// unapproveByID unapproves a prompt matching a short ID or exact filename.
func (u *unapproveCommand) unapproveByID(ctx context.Context, id string) error {
	queuePath, err := FindPromptFile(ctx, u.queueDir, id)
	if err != nil {
		return errors.Errorf(ctx, "file not found: %s", id)
	}
	return u.unapproveFromQueue(ctx, queuePath)
}

// unapproveFromQueue moves a file from queue back to inbox and sets status to draft.
// Only approved prompts may be unapproved (not executing, completed, etc.).
func (u *unapproveCommand) unapproveFromQueue(ctx context.Context, oldPath string) error {
	pf, err := prompt.Load(ctx, oldPath, u.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	if pf.Frontmatter.Status != string(prompt.ApprovedPromptStatus) {
		return errors.Errorf(
			ctx,
			"cannot unapprove prompt with status %q: only approved prompts can be unapproved",
			pf.Frontmatter.Status,
		)
	}

	filename := prompt.StripNumberPrefix(filepath.Base(oldPath))
	newPath := filepath.Join(u.inboxDir, filename)

	if err := os.Rename(oldPath, newPath); err != nil {
		return errors.Wrap(ctx, err, "move file to inbox")
	}

	pf2, err := prompt.Load(ctx, newPath, u.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt from inbox")
	}
	pf2.Frontmatter.Status = string(prompt.DraftPromptStatus)
	if err := pf2.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save prompt")
	}

	if _, err := u.promptManager.NormalizeFilenames(ctx, u.queueDir); err != nil {
		return errors.Wrap(ctx, err, "normalize filenames")
	}

	fmt.Printf("unapproved: %s\n", filename)
	return nil
}
