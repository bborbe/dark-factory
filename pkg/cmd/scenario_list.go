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

//counterfeiter:generate -o ../../mocks/scenario-list-command.go --fake-name ScenarioListCommand . ScenarioListCommand

// ScenarioListCommand executes the scenario list subcommand.
type ScenarioListCommand interface {
	Run(ctx context.Context, args []string) error
}

// ScenarioEntry represents a single scenario in the list output.
type ScenarioEntry struct {
	Number int    `json:"number"`
	Status string `json:"status"`
	Title  string `json:"title"`
	File   string `json:"file"`
}

// scenarioListCommand implements ScenarioListCommand.
type scenarioListCommand struct {
	lister scenario.Lister
}

// NewScenarioListCommand creates a new ScenarioListCommand.
func NewScenarioListCommand(lister scenario.Lister) ScenarioListCommand {
	return &scenarioListCommand{lister: lister}
}

// Run executes the scenario list command.
func (s *scenarioListCommand) Run(ctx context.Context, args []string) error {
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		}
	}

	files, err := s.lister.List(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "list scenarios")
	}

	entries := make([]ScenarioEntry, 0, len(files))
	for _, sf := range files {
		status := sf.Frontmatter.Status
		if status == "" || !scenario.IsKnown(scenario.Status(status)) {
			status = "unknown"
		}
		entries = append(entries, ScenarioEntry{
			Number: sf.Number,
			Status: status,
			Title:  sf.Title,
			File:   sf.Name + ".md",
		})
	}

	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(entries)
	}
	return outputScenarioListTable(entries)
}

// outputScenarioListTable prints scenarios as a fixed-width table.
func outputScenarioListTable(entries []ScenarioEntry) error {
	fmt.Printf("%-6s %-9s %s\n", "NUMBER", "STATUS", "TITLE")
	for _, e := range entries {
		num := fmt.Sprintf("%03d", e.Number)
		fmt.Printf("%-6s %-9s %s\n", num, e.Status, e.Title)
	}
	return nil
}
