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
)

//counterfeiter:generate -o ../../mocks/list-command.go --fake-name ListCommand . ListCommand

// ListCommand executes the list subcommand.
type ListCommand interface {
	Run(ctx context.Context, args []string) error
}

// PromptEntry represents a single prompt entry in the list output.
type PromptEntry struct {
	Status string `json:"status"`
	File   string `json:"file"`
}

// listCommand implements ListCommand.
type listCommand struct {
	inboxDir     string
	queueDir     string
	completedDir string
}

// NewListCommand creates a new ListCommand.
func NewListCommand(
	inboxDir string,
	queueDir string,
	completedDir string,
) ListCommand {
	return &listCommand{
		inboxDir:     inboxDir,
		queueDir:     queueDir,
		completedDir: completedDir,
	}
}

// Run executes the list command.
func (l *listCommand) Run(ctx context.Context, args []string) error {
	queueOnly := false
	failedOnly := false
	jsonOutput := false
	showAll := false

	for _, arg := range args {
		switch arg {
		case "--queue":
			queueOnly = true
		case "--failed":
			failedOnly = true
		case "--json":
			jsonOutput = true
		case "--all":
			showAll = true
		}
	}

	var entries []PromptEntry

	inboxEntries, err := l.scanDir(ctx, l.inboxDir)
	if err != nil {
		return errors.Wrap(ctx, err, "scan inbox")
	}
	entries = append(entries, inboxEntries...)

	queueEntries, err := l.scanDir(ctx, l.queueDir)
	if err != nil {
		return errors.Wrap(ctx, err, "scan queue")
	}
	entries = append(entries, queueEntries...)

	completedEntries, err := l.scanDir(ctx, l.completedDir)
	if err != nil {
		return errors.Wrap(ctx, err, "scan completed")
	}
	entries = append(entries, completedEntries...)

	switch {
	case queueOnly:
		entries = filterPromptsByStatus(
			entries,
			string(prompt.StatusQueued),
			string(prompt.StatusExecuting),
		)
	case failedOnly:
		entries = filterPromptsByStatus(entries, string(prompt.StatusFailed))
	case !showAll:
		entries = excludePromptStatus(entries, string(prompt.StatusCompleted))
	}

	if jsonOutput {
		return l.outputJSON(entries)
	}
	return l.outputTable(entries)
}

// scanDir scans a directory and returns prompt entries.
func (l *listCommand) scanDir(
	ctx context.Context,
	dir string,
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

		entries = append(entries, PromptEntry{
			Status: st,
			File:   entry.Name(),
		})
	}
	return entries, nil
}

// outputTable outputs entries as a human-readable table.
func (l *listCommand) outputTable(entries []PromptEntry) error {
	fmt.Printf("%-12s %s\n", "STATUS", "FILE")
	for _, e := range entries {
		fmt.Printf("%-12s %s\n", e.Status, e.File)
	}
	return nil
}

// outputJSON outputs entries as JSON.
func (l *listCommand) outputJSON(entries []PromptEntry) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(entries)
}

// filterPromptsByStatus returns only entries whose status matches one of the given values.
func filterPromptsByStatus(entries []PromptEntry, statuses ...string) []PromptEntry {
	filtered := entries[:0]
	for _, e := range entries {
		for _, s := range statuses {
			if e.Status == s {
				filtered = append(filtered, e)
				break
			}
		}
	}
	return filtered
}

// excludePromptStatus returns entries that do not have the given status.
func excludePromptStatus(entries []PromptEntry, status string) []PromptEntry {
	filtered := entries[:0]
	for _, e := range entries {
		if e.Status != status {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
