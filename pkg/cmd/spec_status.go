// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/spec-status-command.go --fake-name SpecStatusCommand . SpecStatusCommand

// SpecStatusCommand executes the spec status subcommand.
type SpecStatusCommand interface {
	Run(ctx context.Context, args []string) error
}

// specStatusCommand implements SpecStatusCommand.
type specStatusCommand struct {
	lister  spec.Lister
	counter prompt.Counter
}

// NewSpecStatusCommand creates a new SpecStatusCommand.
func NewSpecStatusCommand(lister spec.Lister, counter prompt.Counter) SpecStatusCommand {
	return &specStatusCommand{lister: lister, counter: counter}
}

// Run executes the spec status command.
func (s *specStatusCommand) Run(ctx context.Context, args []string) error {
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		}
	}

	summary, err := s.lister.Summary(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "get spec summary")
	}

	specs, err := s.lister.List(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "list specs")
	}

	for _, sf := range specs {
		c, t, err := s.counter.CountBySpec(ctx, sf.Name)
		if err != nil {
			return errors.Wrap(ctx, err, "count prompts for spec")
		}
		summary.LinkedPromptsCompleted += c
		summary.LinkedPromptsTotal += t
	}

	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	}

	fmt.Printf(
		"Specs: %d total (%d draft, %d approved, %d prompted, %d completed) | Linked prompts: %d/%d\n",
		summary.Total,
		summary.Draft,
		summary.Approved,
		summary.Prompted,
		summary.Completed,
		summary.LinkedPromptsCompleted,
		summary.LinkedPromptsTotal,
	)
	return nil
}
