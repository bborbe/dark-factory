// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/combined-list-command.go --fake-name CombinedListCommand . CombinedListCommand

// CombinedListCommand shows combined prompt and spec list.
type CombinedListCommand interface {
	Run(ctx context.Context, args []string) error
}

// combinedListOutput is the JSON output structure for combined list.
type combinedListOutput struct {
	Prompts []PromptEntry `json:"prompts"`
	Specs   []SpecEntry   `json:"specs"`
}

// combinedListCommand implements CombinedListCommand.
type combinedListCommand struct {
	inboxDir     string
	queueDir     string
	completedDir string
	lister       spec.Lister
	counter      prompt.Counter
}

// NewCombinedListCommand creates a new CombinedListCommand.
func NewCombinedListCommand(
	inboxDir string,
	queueDir string,
	completedDir string,
	lister spec.Lister,
	counter prompt.Counter,
) CombinedListCommand {
	return &combinedListCommand{
		inboxDir:     inboxDir,
		queueDir:     queueDir,
		completedDir: completedDir,
		lister:       lister,
		counter:      counter,
	}
}

// Run executes the combined list command.
func (c *combinedListCommand) Run(ctx context.Context, args []string) error {
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		}
	}

	promptEntries, err := c.collectPromptEntries(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "collect prompt entries")
	}

	specEntries, err := c.collectSpecEntries(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "collect spec entries")
	}

	if jsonOutput {
		return c.outputJSON(promptEntries, specEntries)
	}
	return c.outputHuman(promptEntries, specEntries)
}

func (c *combinedListCommand) collectPromptEntries(ctx context.Context) ([]PromptEntry, error) {
	var entries []PromptEntry
	for _, pair := range []struct{ dir, loc string }{
		{c.inboxDir, "inbox"},
		{c.queueDir, "queue"},
		{c.completedDir, "completed"},
	} {
		dirEntries, err := os.ReadDir(pair.dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, errors.Wrap(ctx, err, "read directory")
		}
		for _, entry := range dirEntries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			path := filepath.Join(pair.dir, entry.Name())
			pf, err := prompt.Load(ctx, path)
			if err != nil {
				continue
			}
			st := pf.Frontmatter.Status
			if st == "" {
				st = "created"
			}
			entries = append(entries, PromptEntry{
				Location: pair.loc,
				Status:   st,
				File:     entry.Name(),
			})
		}
	}
	return entries, nil
}

func (c *combinedListCommand) collectSpecEntries(ctx context.Context) ([]SpecEntry, error) {
	specs, err := c.lister.List(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "list specs")
	}

	entries := make([]SpecEntry, 0, len(specs))
	for _, sf := range specs {
		completed, total, err := c.counter.CountBySpec(ctx, sf.Name)
		if err != nil {
			return nil, errors.Wrap(ctx, err, "count prompts for spec")
		}
		entries = append(entries, SpecEntry{
			Status:           sf.Frontmatter.Status,
			File:             sf.Name + ".md",
			PromptsCompleted: completed,
			PromptsTotal:     total,
		})
	}
	return entries, nil
}

func (c *combinedListCommand) outputJSON(prompts []PromptEntry, specs []SpecEntry) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(combinedListOutput{
		Prompts: prompts,
		Specs:   specs,
	})
}

func (c *combinedListCommand) outputHuman(prompts []PromptEntry, specs []SpecEntry) error {
	fmt.Println("PROMPTS:")
	fmt.Printf("%-10s %-10s %s\n", "LOCATION", "STATUS", "FILE")
	for _, e := range prompts {
		fmt.Printf("%-10s %-10s %s\n", e.Location, e.Status, e.File)
	}
	fmt.Println()
	fmt.Println("SPECS:")
	fmt.Printf("%-10s %-8s %s\n", "STATUS", "PROMPTS", "FILE")
	for _, e := range specs {
		p := fmt.Sprintf("%d/%d", e.PromptsCompleted, e.PromptsTotal)
		fmt.Printf("%-10s %-8s %s\n", e.Status, p, e.File)
	}
	return nil
}
