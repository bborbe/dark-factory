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
	showAll := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		case "--all":
			showAll = true
		}
	}

	promptEntries, err := c.collectPromptEntries(ctx, showAll)
	if err != nil {
		return errors.Wrap(ctx, err, "collect prompt entries")
	}

	specEntries, err := c.collectSpecEntries(ctx, showAll)
	if err != nil {
		return errors.Wrap(ctx, err, "collect spec entries")
	}

	if jsonOutput {
		return c.outputJSON(promptEntries, specEntries)
	}
	return c.outputHuman(promptEntries, specEntries)
}

func (c *combinedListCommand) collectPromptEntries(
	ctx context.Context,
	showAll bool,
) ([]PromptEntry, error) {
	dirs := []string{c.inboxDir, c.queueDir, c.completedDir}
	entries := make([]PromptEntry, 0, len(dirs))
	for _, dir := range dirs {
		dirEntries, err := c.collectPromptEntriesFromDir(ctx, dir, showAll)
		if err != nil {
			return nil, err
		}
		entries = append(entries, dirEntries...)
	}
	return entries, nil
}

func (c *combinedListCommand) collectPromptEntriesFromDir(
	ctx context.Context,
	dir string,
	showAll bool,
) ([]PromptEntry, error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrap(ctx, err, "read directory")
	}
	entries := make([]PromptEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		pf, err := prompt.Load(ctx, path)
		if err != nil {
			continue
		}
		st := pf.Frontmatter.Status
		if st == "" {
			st = "created"
		}
		if !showAll && st == string(prompt.CompletedPromptStatus) {
			continue
		}
		entries = append(entries, PromptEntry{Status: st, File: entry.Name()})
	}
	return entries, nil
}

func (c *combinedListCommand) collectSpecEntries(
	ctx context.Context,
	showAll bool,
) ([]SpecEntry, error) {
	specs, err := c.lister.List(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "list specs")
	}

	entries := make([]SpecEntry, 0, len(specs))
	for _, sf := range specs {
		if !showAll && sf.Frontmatter.Status == string(spec.StatusCompleted) {
			continue
		}
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
	fmt.Printf("%-12s %s\n", "STATUS", "FILE")
	for _, e := range prompts {
		fmt.Printf("%-12s %s\n", e.Status, e.File)
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
