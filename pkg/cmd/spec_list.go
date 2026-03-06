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

	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/spec-list-command.go --fake-name SpecListCommand . SpecListCommand

// SpecListCommand executes the spec list subcommand.
type SpecListCommand interface {
	Run(ctx context.Context, args []string) error
}

// SpecEntry represents a single spec entry in the list output.
type SpecEntry struct {
	Status string `json:"status"`
	File   string `json:"file"`
}

// specListCommand implements SpecListCommand.
type specListCommand struct {
	lister spec.Lister
}

// NewSpecListCommand creates a new SpecListCommand.
func NewSpecListCommand(lister spec.Lister) SpecListCommand {
	return &specListCommand{lister: lister}
}

// Run executes the spec list command.
func (s *specListCommand) Run(ctx context.Context, args []string) error {
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		}
	}

	specs, err := s.lister.List(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "list specs")
	}

	entries := make([]SpecEntry, 0, len(specs))
	for _, sf := range specs {
		entries = append(entries, SpecEntry{
			Status: sf.Frontmatter.Status,
			File:   sf.Name + ".md",
		})
	}

	if jsonOutput {
		return outputSpecListJSON(entries)
	}
	return outputSpecListTable(entries)
}

// outputSpecListTable outputs spec entries as a human-readable table.
func outputSpecListTable(entries []SpecEntry) error {
	fmt.Printf("%-10s %s\n", "STATUS", "FILE")
	for _, e := range entries {
		fmt.Printf("%-10s %s\n", e.Status, e.File)
	}
	return nil
}

// outputSpecListJSON outputs spec entries as JSON.
func outputSpecListJSON(entries []SpecEntry) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(entries)
}
