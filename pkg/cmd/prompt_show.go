// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"
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
	promptManager PromptManager
}

// NewPromptShowCommand creates a new PromptShowCommand.
func NewPromptShowCommand(
	inboxDir, inProgressDir, completedDir, logDir string,
	promptManager PromptManager,
) PromptShowCommand {
	return &promptShowCommand{
		inboxDir:      inboxDir,
		inProgressDir: inProgressDir,
		completedDir:  completedDir,
		logDir:        logDir,
		promptManager: promptManager,
	}
}

// PromptShowOutput holds all fields for JSON output.
type PromptShowOutput struct {
	File           string   `json:"file"`
	Status         string   `json:"status"`
	Specs          []string `json:"specs,omitempty"`
	Summary        string   `json:"summary,omitempty"`
	LastFailReason string   `json:"lastFailReason,omitempty"`
	Created        string   `json:"created,omitempty"`
	Queued         string   `json:"queued,omitempty"`
	Started        string   `json:"started,omitempty"`
	Completed      string   `json:"completed,omitempty"`
	LogPath        string   `json:"log_path,omitempty"`
}

// RenderPromptShow writes the human-readable text representation of out to w.
func RenderPromptShow(w io.Writer, out PromptShowOutput) {
	fmt.Fprintf(w, "File:    %s\n", out.File)
	fmt.Fprintf(w, "Status:  %s\n", out.Status)
	if len(out.Specs) > 0 {
		fmt.Fprintf(w, "Spec:    %s\n", strings.Join(out.Specs, ", "))
	}
	if out.Summary != "" {
		fmt.Fprintf(w, "Summary: %s\n", out.Summary)
	}
	if out.LastFailReason != "" {
		fmt.Fprintf(w, "Error:   %s\n", out.LastFailReason)
	}
	if out.Created != "" {
		fmt.Fprintf(w, "Created:   %s\n", out.Created)
	}
	if out.Queued != "" {
		fmt.Fprintf(w, "Queued:    %s\n", out.Queued)
	}
	if out.Started != "" {
		fmt.Fprintf(w, "Started:   %s\n", out.Started)
	}
	if out.Completed != "" {
		fmt.Fprintf(w, "Completed: %s\n", out.Completed)
	}
	if out.LogPath != "" {
		fmt.Fprintf(w, "Log:     %s\n", out.LogPath)
	}
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

	pf, err := p.promptManager.Load(ctx, path)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	basename := strings.TrimSuffix(filepath.Base(path), ".md")
	logPath := filepath.Join(p.logDir, basename+".log")
	if _, err := os.Stat(logPath); err != nil {
		logPath = ""
	}

	out := PromptShowOutput{
		File:           filepath.Base(path),
		Status:         pf.Frontmatter.Status,
		Specs:          []string(pf.Frontmatter.Specs),
		Summary:        pf.Frontmatter.Summary,
		LastFailReason: pf.Frontmatter.LastFailReason,
		Created:        pf.Frontmatter.Created,
		Queued:         pf.Frontmatter.Queued,
		Started:        pf.Frontmatter.Started,
		Completed:      pf.Frontmatter.Completed,
		LogPath:        logPath,
	}

	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(out)
	}

	RenderPromptShow(os.Stdout, out)
	return nil
}
