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
	promptManager PromptManager
}

// NewCancelCommand creates a new CancelCommand.
func NewCancelCommand(
	queueDir string,
	promptManager PromptManager,
) CancelCommand {
	return &cancelCommand{
		queueDir:      queueDir,
		promptManager: promptManager,
	}
}

// Run executes the cancel command.
func (a *cancelCommand) Run(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.Errorf(ctx, "usage: dark-factory prompt cancel <id>")
	}
	id := args[0]

	path, err := FindPromptFile(ctx, a.queueDir, id)
	if err != nil {
		return errors.Errorf(ctx, "prompt not found: %s", id)
	}

	pf, err := a.promptManager.Load(ctx, path)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	switch prompt.PromptStatus(pf.Frontmatter.Status) {
	case prompt.ApprovedPromptStatus, prompt.ExecutingPromptStatus:
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
