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

//counterfeiter:generate -o ../../mocks/prompt-show-command.go --fake-name PromptShowCommand . PromptShowCommand

// PromptShowCommand executes the prompt show subcommand.
type PromptShowCommand interface {
	Run(ctx context.Context, args []string) error
}

// promptShowCommand implements PromptShowCommand.
type promptShowCommand struct {
	inboxDir      string
	inProgressDir string
	completedDir  string
	logDir        string
}

// NewPromptShowCommand creates a new PromptShowCommand.
func NewPromptShowCommand(inboxDir, inProgressDir, completedDir, logDir string) PromptShowCommand {
	return &promptShowCommand{
		inboxDir:      inboxDir,
		inProgressDir: inProgressDir,
		completedDir:  completedDir,
		logDir:        logDir,
	}
}

// PromptShowOutput holds all fields for JSON output.
type PromptShowOutput struct {
	File      string   `json:"file"`
	Status    string   `json:"status"`
	Specs     []string `json:"specs,omitempty"`
	Summary   string   `json:"summary,omitempty"`
	Created   string   `json:"created,omitempty"`
	Queued    string   `json:"queued,omitempty"`
	Started   string   `json:"started,omitempty"`
	Completed string   `json:"completed,omitempty"`
	LogPath   string   `json:"log_path,omitempty"`
}

// Run executes the prompt show command.
func (p *promptShowCommand) Run(ctx context.Context, args []string) error {
	jsonOutput := false
	id := ""
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		} else if id == "" {
			id = arg
		}
	}

	if id == "" {
		return errors.Errorf(ctx, "prompt identifier required")
	}

	var path string
	var findErr error
	for _, dir := range []string{p.inboxDir, p.inProgressDir, p.completedDir} {
		path, findErr = FindPromptFile(ctx, dir, id)
		if findErr == nil {
			break
		}
	}
	if findErr != nil {
		return errors.Errorf(ctx, "prompt not found: %s", id)
	}

	pf, err := prompt.Load(ctx, path)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	basename := strings.TrimSuffix(filepath.Base(path), ".md")
	logPath := filepath.Join(p.logDir, basename+".log")
	if _, err := os.Stat(logPath); err != nil {
		logPath = ""
	}

	out := PromptShowOutput{
		File:      filepath.Base(path),
		Status:    pf.Frontmatter.Status,
		Specs:     []string(pf.Frontmatter.Specs),
		Summary:   pf.Frontmatter.Summary,
		Created:   pf.Frontmatter.Created,
		Queued:    pf.Frontmatter.Queued,
		Started:   pf.Frontmatter.Started,
		Completed: pf.Frontmatter.Completed,
		LogPath:   logPath,
	}

	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(out)
	}

	fmt.Printf("File:    %s\n", out.File)
	fmt.Printf("Status:  %s\n", out.Status)
	if len(out.Specs) > 0 {
		fmt.Printf("Spec:    %s\n", strings.Join(out.Specs, ", "))
	}
	if out.Summary != "" {
		fmt.Printf("Summary: %s\n", out.Summary)
	}
	if out.Created != "" {
		fmt.Printf("Created:   %s\n", out.Created)
	}
	if out.Queued != "" {
		fmt.Printf("Queued:    %s\n", out.Queued)
	}
	if out.Started != "" {
		fmt.Printf("Started:   %s\n", out.Started)
	}
	if out.Completed != "" {
		fmt.Printf("Completed: %s\n", out.Completed)
	}
	if out.LogPath != "" {
		fmt.Printf("Log:     %s\n", out.LogPath)
	}
	return nil
}
