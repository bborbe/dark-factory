// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/spec-mark-prompted-command.go --fake-name SpecMarkPromptedCommand . SpecMarkPromptedCommand

// SpecMarkPromptedCommand executes the spec mark-prompted subcommand.
type SpecMarkPromptedCommand interface {
	Run(ctx context.Context, args []string) error
}

// specMarkPromptedCommand implements SpecMarkPromptedCommand.
type specMarkPromptedCommand struct {
	inboxDir              string
	inProgressDir         string
	completedDir          string
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewSpecMarkPromptedCommand creates a new SpecMarkPromptedCommand.
func NewSpecMarkPromptedCommand(
	inboxDir string,
	inProgressDir string,
	completedDir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) SpecMarkPromptedCommand {
	return &specMarkPromptedCommand{
		inboxDir:              inboxDir,
		inProgressDir:         inProgressDir,
		completedDir:          completedDir,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// Run executes the spec mark-prompted command.
func (s *specMarkPromptedCommand) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.Errorf(ctx, "spec identifier required")
	}

	path, err := FindSpecFileInDirs(ctx, args[0], s.inboxDir, s.inProgressDir, s.completedDir)
	if err != nil {
		return errors.Wrap(ctx, err, "find spec file")
	}

	sf, err := spec.Load(ctx, path, s.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load spec")
	}

	current := spec.Status(sf.Frontmatter.Status)

	switch current {
	case spec.StatusPrompted:
		fmt.Printf("already prompted: %s\n", filepath.Base(path))
		return nil
	case spec.StatusApproved:
		sf.SetStatus(string(spec.StatusGenerating))
		sf.SetStatus(string(spec.StatusPrompted))
	case spec.StatusGenerating:
		sf.SetStatus(string(spec.StatusPrompted))
	default:
		return errors.Errorf(
			ctx,
			"spec cannot be marked prompted from status %q (expected approved, generating, or prompted)",
			current,
		)
	}

	if err := sf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save spec")
	}

	fmt.Printf("prompted: %s\n", filepath.Base(path))
	return nil
}
