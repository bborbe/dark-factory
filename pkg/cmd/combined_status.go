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
	"github.com/bborbe/dark-factory/pkg/status"
)

//counterfeiter:generate -o ../../mocks/combined-status-command.go --fake-name CombinedStatusCommand . CombinedStatusCommand

// CombinedStatusCommand shows combined prompt and spec status.
type CombinedStatusCommand interface {
	Run(ctx context.Context, args []string) error
}

// combinedStatusOutput is the JSON output structure for combined status.
type combinedStatusOutput struct {
	Prompts *status.Status `json:"prompts"`
	Specs   *spec.Summary  `json:"specs"`
}

// combinedStatusCommand implements CombinedStatusCommand.
type combinedStatusCommand struct {
	checker   status.Checker
	formatter status.Formatter
	lister    spec.Lister
	counter   prompt.Counter
}

// NewCombinedStatusCommand creates a new CombinedStatusCommand.
func NewCombinedStatusCommand(
	checker status.Checker,
	formatter status.Formatter,
	lister spec.Lister,
	counter prompt.Counter,
) CombinedStatusCommand {
	return &combinedStatusCommand{
		checker:   checker,
		formatter: formatter,
		lister:    lister,
		counter:   counter,
	}
}

// Run executes the combined status command.
func (c *combinedStatusCommand) Run(ctx context.Context, args []string) error {
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		}
	}

	st, err := c.checker.GetStatus(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "get prompt status")
	}

	summary, err := c.lister.Summary(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "get spec summary")
	}

	specs, err := c.lister.List(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "list specs")
	}

	for _, sf := range specs {
		completed, total, err := c.counter.CountBySpec(ctx, sf.Name)
		if err != nil {
			return errors.Wrap(ctx, err, "count prompts for spec")
		}
		summary.LinkedPromptsCompleted += completed
		summary.LinkedPromptsTotal += total
	}

	if jsonOutput {
		return c.outputJSON(st, summary)
	}
	return c.outputHuman(st, summary)
}

func (c *combinedStatusCommand) outputJSON(st *status.Status, summary *spec.Summary) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(combinedStatusOutput{
		Prompts: st,
		Specs:   summary,
	})
}

func (c *combinedStatusCommand) outputHuman(st *status.Status, summary *spec.Summary) error {
	fmt.Print(c.formatter.Format(st))
	fmt.Println()
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
