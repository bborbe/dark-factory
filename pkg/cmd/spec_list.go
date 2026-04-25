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
	specpkg "github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/spec-list-command.go --fake-name SpecListCommand . SpecListCommand

// SpecListCommand executes the spec list subcommand.
type SpecListCommand interface {
	Run(ctx context.Context, args []string) error
}

// SpecEntry represents a single spec entry in the list output.
type SpecEntry struct {
	Status           string `json:"status"`
	File             string `json:"file"`
	PromptsCompleted int    `json:"prompts_completed"`
	PromptsTotal     int    `json:"prompts_total"`
}

// specListCommand implements SpecListCommand.
type specListCommand struct {
	lister  specpkg.Lister
	counter prompt.Counter
}

// NewSpecListCommand creates a new SpecListCommand.
func NewSpecListCommand(lister specpkg.Lister, counter prompt.Counter) SpecListCommand {
	return &specListCommand{lister: lister, counter: counter}
}

// Run executes the spec list command.
func (s *specListCommand) Run(ctx context.Context, args []string) error {
	jsonOutput := false
	showAll := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		case "--all":
			showAll = true
		}
	}

	specs, err := s.lister.List(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "list specs")
	}

	entries := make([]SpecEntry, 0, len(specs))
	for _, sf := range specs {
		if !showAll && (sf.Frontmatter.Status == string(specpkg.StatusCompleted) ||
			sf.Frontmatter.Status == string(specpkg.StatusRejected)) {
			continue
		}
		completed, total, err := s.counter.CountBySpec(ctx, sf.Name)
		if err != nil {
			return errors.Wrap(ctx, err, "count prompts for spec")
		}
		entries = append(entries, SpecEntry{
			Status:           sf.Frontmatter.Status,
			File:             sf.Name + ".md",
			PromptsCompleted: completed,
			PromptsTotal:     total,
		})
	}

	if jsonOutput {
		return outputSpecListJSON(entries)
	}
	return outputSpecListTable(entries)
}

// outputSpecListTable outputs spec entries as a human-readable table.
func outputSpecListTable(entries []SpecEntry) error {
	fmt.Printf("%-11s %-8s %s\n", "STATUS", "PROMPTS", "FILE")
	for _, e := range entries {
		prompts := fmt.Sprintf("%d/%d", e.PromptsCompleted, e.PromptsTotal)
		status := e.Status
		if specpkg.Status(e.Status) == specpkg.StatusVerifying {
			status = "!" + e.Status
		}
		fmt.Printf("%-11s %-8s %s\n", status, prompts, e.File)
	}
	return nil
}

// outputSpecListJSON outputs spec entries as JSON.
func outputSpecListJSON(entries []SpecEntry) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(entries)
}
