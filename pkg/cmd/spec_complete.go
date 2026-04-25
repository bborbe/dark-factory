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

	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/spec-complete-command.go --fake-name SpecCompleteCommand . SpecCompleteCommand

// SpecCompleteCommand executes the spec complete subcommand.
type SpecCompleteCommand interface {
	Run(ctx context.Context, args []string) error
}

// specCompleteCommand implements SpecCompleteCommand.
type specCompleteCommand struct {
	inboxDir              string
	inProgressDir         string
	completedDir          string
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewSpecCompleteCommand creates a new SpecCompleteCommand.
func NewSpecCompleteCommand(
	inboxDir string,
	inProgressDir string,
	completedDir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) SpecCompleteCommand {
	return &specCompleteCommand{
		inboxDir:              inboxDir,
		inProgressDir:         inProgressDir,
		completedDir:          completedDir,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// Run executes the spec complete command.
func (s *specCompleteCommand) Run(ctx context.Context, args []string) error {
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

	sf, err := spec.Load(ctx, path, s.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load spec")
	}

	if err := spec.Status(sf.Frontmatter.Status).CanTransitionTo(spec.StatusCompleted); err != nil {
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

	fmt.Printf("completed: %s\n", filepath.Base(dest))
	return nil
}
