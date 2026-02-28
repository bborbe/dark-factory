// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"context"
	"os"
	"path/filepath"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

// Factory orchestrates the main processing loop.
type Factory struct {
	promptsDir string
}

// New creates a new Factory.
func New() *Factory {
	return &Factory{
		promptsDir: "prompts",
	}
}

// Run executes the main processing loop:
// 1. Scan for queued prompts
// 2. Process first one (alphabetically)
// 3. On success: commit, version, push, mark completed, loop again
// 4. On failure: mark failed, exit 1
func (f *Factory) Run(ctx context.Context) error {
	for {
		// Scan for queued prompts
		queued, err := prompt.ListQueued(ctx, f.promptsDir)
		if err != nil {
			return errors.Wrap(ctx, err, "list queued prompts")
		}

		// No more queued prompts - exit successfully
		if len(queued) == 0 {
			return nil
		}

		// Pick first prompt (already sorted alphabetically)
		p := queued[0]

		// Process the prompt
		if err := f.processPrompt(ctx, p); err != nil {
			// Mark as failed
			if setErr := prompt.SetStatus(ctx, p.Path, "failed"); setErr != nil {
				return errors.Wrap(ctx, setErr, "set failed status")
			}
			return errors.Wrap(ctx, err, "process prompt")
		}

		// Mark as completed
		if err := prompt.SetStatus(ctx, p.Path, "completed"); err != nil {
			return errors.Wrap(ctx, err, "set completed status")
		}

		// Move to completed/
		if err := prompt.MoveToCompleted(ctx, p.Path); err != nil {
			return errors.Wrap(ctx, err, "move to completed")
		}

		// Loop again to process next prompt
	}
}

// processPrompt executes a single prompt and commits the result.
func (f *Factory) processPrompt(ctx context.Context, p prompt.Prompt) error {
	// Set status to executing
	if err := prompt.SetStatus(ctx, p.Path, "executing"); err != nil {
		return errors.Wrap(ctx, err, "set executing status")
	}

	// Get prompt content
	content, err := prompt.Content(ctx, p.Path)
	if err != nil {
		return errors.Wrap(ctx, err, "get prompt content")
	}

	// Execute via Docker
	if err := executor.Execute(ctx, content); err != nil {
		return errors.Wrap(ctx, err, "execute prompt")
	}

	// Get prompt title for changelog
	title, err := prompt.Title(ctx, p.Path)
	if err != nil {
		return errors.Wrap(ctx, err, "get prompt title")
	}

	// Commit and release
	if err := git.CommitAndRelease(ctx, title); err != nil {
		return errors.Wrap(ctx, err, "commit and release")
	}

	return nil
}

// SetPromptsDir sets the prompts directory (useful for testing).
func (f *Factory) SetPromptsDir(dir string) {
	f.promptsDir = dir
}

// GetPromptsDir returns the prompts directory.
func (f *Factory) GetPromptsDir() string {
	// If relative path, make it absolute
	if !filepath.IsAbs(f.promptsDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return f.promptsDir
		}
		return filepath.Join(cwd, f.promptsDir)
	}
	return f.promptsDir
}
