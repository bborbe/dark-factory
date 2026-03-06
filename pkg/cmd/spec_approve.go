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

//counterfeiter:generate -o ../../mocks/spec-approve-command.go --fake-name SpecApproveCommand . SpecApproveCommand

// SpecApproveCommand executes the spec approve subcommand.
type SpecApproveCommand interface {
	Run(ctx context.Context, args []string) error
}

// specApproveCommand implements SpecApproveCommand.
type specApproveCommand struct {
	inboxDir      string
	inProgressDir string
}

// NewSpecApproveCommand creates a new SpecApproveCommand.
func NewSpecApproveCommand(inboxDir string, inProgressDir string) SpecApproveCommand {
	return &specApproveCommand{
		inboxDir:      inboxDir,
		inProgressDir: inProgressDir,
	}
}

// Run executes the spec approve command.
func (s *specApproveCommand) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.Errorf(ctx, "spec identifier required")
	}

	id := args[0]
	path, err := FindSpecFile(ctx, s.inboxDir, id)
	if err != nil {
		return err
	}

	sf, err := spec.Load(ctx, path)
	if err != nil {
		return errors.Wrap(ctx, err, "load spec")
	}

	if sf.Frontmatter.Status == string(spec.StatusApproved) {
		return errors.Errorf(ctx, "spec is already approved")
	}

	sf.SetStatus(string(spec.StatusApproved))
	if err := sf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save spec")
	}

	// Ensure inProgressDir exists
	if err := os.MkdirAll(s.inProgressDir, 0750); err != nil {
		return errors.Wrap(ctx, err, "create in-progress dir")
	}

	// Move file to inProgressDir — the file move is the signal to SpecWatcher
	dest := filepath.Join(s.inProgressDir, filepath.Base(path))
	if err := os.Rename(path, dest); err != nil {
		return errors.Wrap(ctx, err, "move spec to in-progress")
	}

	fmt.Printf("approved: %s\n", filepath.Base(dest))
	return nil
}
