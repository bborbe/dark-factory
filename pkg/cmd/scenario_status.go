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

	"github.com/bborbe/dark-factory/pkg/scenario"
)

//counterfeiter:generate -o ../../mocks/scenario-status-command.go --fake-name ScenarioStatusCommand . ScenarioStatusCommand

// ScenarioStatusCommand executes the scenario status subcommand.
type ScenarioStatusCommand interface {
	Run(ctx context.Context, args []string) error
}

// scenarioStatusCommand implements ScenarioStatusCommand.
type scenarioStatusCommand struct {
	lister scenario.Lister
}

// NewScenarioStatusCommand creates a new ScenarioStatusCommand.
func NewScenarioStatusCommand(lister scenario.Lister) ScenarioStatusCommand {
	return &scenarioStatusCommand{lister: lister}
}

// Run executes the scenario status command.
func (s *scenarioStatusCommand) Run(ctx context.Context, args []string) error {
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		}
	}

	summary, err := s.lister.Summary(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "get scenario summary")
	}

	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	}

	fmt.Printf("idea:     %d\n", summary.Idea)
	fmt.Printf("draft:    %d\n", summary.Draft)
	fmt.Printf("active:   %d\n", summary.Active)
	fmt.Printf("outdated: %d\n", summary.Outdated)
	if summary.Unknown > 0 {
		fmt.Printf("unknown:  %d\n", summary.Unknown)
	}
	return nil
}
