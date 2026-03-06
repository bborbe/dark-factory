// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
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
	specsDir string
}

// NewSpecApproveCommand creates a new SpecApproveCommand.
func NewSpecApproveCommand(specsDir string) SpecApproveCommand {
	return &specApproveCommand{specsDir: specsDir}
}

// Run executes the spec approve command.
func (s *specApproveCommand) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.Errorf(ctx, "spec identifier required")
	}

	id := args[0]
	path, err := FindSpecFile(ctx, s.specsDir, id)
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

	fmt.Printf("approved: %s\n", filepath.Base(path))
	return nil
}
