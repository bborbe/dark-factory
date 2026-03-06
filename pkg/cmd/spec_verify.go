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

//counterfeiter:generate -o ../../mocks/spec-verify-command.go --fake-name SpecVerifyCommand . SpecVerifyCommand

// SpecVerifyCommand executes the spec verify subcommand.
type SpecVerifyCommand interface {
	Run(ctx context.Context, args []string) error
}

// specVerifyCommand implements SpecVerifyCommand.
type specVerifyCommand struct {
	specsDir string
}

// NewSpecVerifyCommand creates a new SpecVerifyCommand.
func NewSpecVerifyCommand(specsDir string) SpecVerifyCommand {
	return &specVerifyCommand{specsDir: specsDir}
}

// Run executes the spec verify command.
func (s *specVerifyCommand) Run(ctx context.Context, args []string) error {
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

	fmt.Printf("verified: %s\n", filepath.Base(path))
	return nil
}
