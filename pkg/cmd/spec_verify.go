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

	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/spec-verify-command.go --fake-name SpecVerifyCommand . SpecVerifyCommand

// SpecVerifyCommand executes the spec verify subcommand.
type SpecVerifyCommand interface {
	Run(ctx context.Context, args []string) error
}

// specVerifyCommand implements SpecVerifyCommand.
type specVerifyCommand struct {
	inboxDir      string
	inProgressDir string
	completedDir  string
}

// NewSpecVerifyCommand creates a new SpecVerifyCommand.
func NewSpecVerifyCommand(inboxDir, inProgressDir, completedDir string) SpecVerifyCommand {
	return &specVerifyCommand{
		inboxDir:      inboxDir,
		inProgressDir: inProgressDir,
		completedDir:  completedDir,
	}
}

// Run executes the spec verify command.
func (s *specVerifyCommand) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.Errorf(ctx, "spec identifier required")
	}

	id := args[0]
	// Search all three dirs — a verifying spec is in inProgressDir, but allow
	// searching all dirs for convenience
	path, err := FindSpecFileInDirs(ctx, id, s.inboxDir, s.inProgressDir, s.completedDir)
	if err != nil {
		return err
	}

	sf, err := spec.Load(ctx, path)
	if err != nil {
		return errors.Wrap(ctx, err, "load spec")
	}

	if sf.Frontmatter.Status != string(spec.StatusVerifying) {
		return errors.Errorf(
			ctx,
			"spec is not in verifying state (current: %s)",
			sf.Frontmatter.Status,
		)
	}

	sf.MarkCompleted()
	if err := sf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save spec")
	}

	// Ensure completedDir exists
	if err := os.MkdirAll(s.completedDir, 0750); err != nil {
		return errors.Wrap(ctx, err, "create completed dir")
	}

	// Move file to completedDir
	dest := filepath.Join(s.completedDir, filepath.Base(path))
	if err := os.Rename(path, dest); err != nil {
		return errors.Wrap(ctx, err, "move spec to completed")
	}

	fmt.Printf("verified: %s\n", filepath.Base(dest))
	return nil
}
