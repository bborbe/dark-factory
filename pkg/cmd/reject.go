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

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/reject-command.go --fake-name RejectCommand . RejectCommand

// RejectCommand executes the prompt reject subcommand.
type RejectCommand interface {
	Run(ctx context.Context, args []string) error
}

// rejectCommand implements RejectCommand.
type rejectCommand struct {
	inboxDir      string
	inProgressDir string
	rejectedDir   string
	promptManager PromptManager
}

// NewRejectCommand creates a new RejectCommand.
func NewRejectCommand(
	inboxDir string,
	inProgressDir string,
	rejectedDir string,
	promptManager PromptManager,
) RejectCommand {
	return &rejectCommand{
		inboxDir:      inboxDir,
		inProgressDir: inProgressDir,
		rejectedDir:   rejectedDir,
		promptManager: promptManager,
	}
}

// Run executes the prompt reject command.
func (r *rejectCommand) Run(ctx context.Context, args []string) error {
	reason, remaining, err := parseReasonFlag(args)
	if err != nil {
		return errors.Errorf(ctx, "%v", err)
	}
	if len(remaining) == 0 {
		return errors.Errorf(ctx, "usage: dark-factory prompt reject <file> --reason <text>")
	}
	id := remaining[0]
	return r.rejectByID(ctx, id, reason)
}

func (r *rejectCommand) rejectByID(ctx context.Context, id, reason string) error {
	path, err := FindPromptFileInDirs(ctx, id, r.inboxDir, r.inProgressDir)
	if err != nil {
		return errors.Errorf(ctx, "prompt not found: %s", id)
	}

	pf, err := r.promptManager.Load(ctx, path)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	status := prompt.PromptStatus(pf.Frontmatter.Status)
	if status == prompt.RejectedPromptStatus {
		return errors.Errorf(ctx, "%s is already rejected", filepath.Base(path))
	}
	if !status.IsRejectable() {
		return errors.Errorf(
			ctx,
			"cannot reject prompt with status %q — pre-execution states only (idea, draft, approved)",
			pf.Frontmatter.Status,
		)
	}

	pf.StampRejected(reason)
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save prompt")
	}

	if err := os.MkdirAll(r.rejectedDir, 0750); err != nil {
		return errors.Wrap(ctx, err, "create rejected dir")
	}

	dest := filepath.Join(r.rejectedDir, filepath.Base(path))
	if err := os.Rename(path, dest); err != nil {
		return errors.Wrap(ctx, err, "move prompt to rejected")
	}

	fmt.Printf("rejected: %s\n", filepath.Base(path))
	return nil
}

// parseReasonFlag extracts --reason <text> from args.
// Returns the reason string, remaining args (without --reason and its value), and an error if
// --reason is missing or has no value.
func parseReasonFlag(args []string) (string, []string, error) {
	var reason string
	var remaining []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--reason" {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--reason requires a value")
			}
			reason = args[i+1]
			i++ // skip the value
			continue
		}
		remaining = append(remaining, args[i])
	}
	if reason == "" {
		return "", nil, fmt.Errorf("--reason is required")
	}
	return reason, remaining, nil
}
