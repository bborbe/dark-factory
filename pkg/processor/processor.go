// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	stderrors "errors"
	"log"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/version"
)

// Processor processes queued prompts.
//
//counterfeiter:generate -o ../../mocks/processor.go --fake-name Processor . Processor
type Processor interface {
	Process(ctx context.Context) error
}

// processor implements Processor.
type processor struct {
	promptsDir    string
	executor      executor.Executor
	promptManager prompt.Manager
	releaser      git.Releaser
	versionGetter version.Getter
	ready         <-chan struct{}
}

// NewProcessor creates a new Processor.
func NewProcessor(
	promptsDir string,
	exec executor.Executor,
	promptManager prompt.Manager,
	releaser git.Releaser,
	versionGetter version.Getter,
	ready <-chan struct{},
) Processor {
	return &processor{
		promptsDir:    promptsDir,
		executor:      exec,
		promptManager: promptManager,
		releaser:      releaser,
		versionGetter: versionGetter,
		ready:         ready,
	}
}

// Process starts processing queued prompts.
// It processes existing queued prompts on startup, then listens for signals from the watcher.
func (p *processor) Process(ctx context.Context) error {
	log.Printf("dark-factory: processor started")

	// Process any existing queued prompts first
	if err := p.processExistingQueued(ctx); err != nil {
		return errors.Wrap(ctx, err, "process existing queued prompts")
	}

	// Listen for ready signals from watcher
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("dark-factory: processor shutting down")
			return nil

		case <-p.ready:
			// Watcher normalized files, check for new queued prompts
			if err := p.processExistingQueued(ctx); err != nil {
				return errors.Wrap(ctx, err, "process queued prompts")
			}

		case <-ticker.C:
			// Periodic scan for queued prompts (in case we missed a signal)
			if err := p.processExistingQueued(ctx); err != nil {
				return errors.Wrap(ctx, err, "periodic scan")
			}
		}
	}
}

// processExistingQueued scans for and processes any existing queued prompts.
func (p *processor) processExistingQueued(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// Scan for queued prompts
		queued, err := p.promptManager.ListQueued(ctx)
		if err != nil {
			return errors.Wrap(ctx, err, "list queued prompts")
		}

		// No more queued prompts - done
		if len(queued) == 0 {
			return nil
		}

		// Pick first prompt (already sorted alphabetically)
		pr := queued[0]

		// Validate prompt before execution
		if err := pr.ValidateForExecution(ctx); err != nil {
			log.Printf("dark-factory: skipping %s: %v", filepath.Base(pr.Path), err)
			continue
		}

		// Check ordering - all previous prompts must be completed
		if !p.promptManager.AllPreviousCompleted(ctx, pr.Number()) {
			log.Printf(
				"dark-factory: skipping %s: previous prompt not completed",
				filepath.Base(pr.Path),
			)
			continue
		}

		log.Printf("dark-factory: found queued prompt: %s", filepath.Base(pr.Path))

		// Process the prompt (includes moving to completed/ and committing)
		if err := p.processPrompt(ctx, pr); err != nil {
			// Mark as failed — file may have been moved to completed/ before the error.
			if setErr := p.promptManager.SetStatus(ctx, pr.Path, string(prompt.StatusFailed)); setErr != nil {
				log.Printf("dark-factory: failed to set failed status: %v", setErr)
			}
			return errors.Wrap(ctx, err, "process prompt")
		}

		log.Printf("dark-factory: watching %s for queued prompts...", p.promptsDir)

		// Loop again to process next prompt
	}
}

// processPrompt executes a single prompt and commits the result.
func (p *processor) processPrompt(ctx context.Context, pr prompt.Prompt) error {
	// Get prompt content first to check if empty
	content, err := p.promptManager.Content(ctx, pr.Path)
	if err != nil {
		// If prompt is empty, move to completed and skip execution
		if stderrors.Is(err, prompt.ErrEmptyPrompt) {
			log.Printf(
				"dark-factory: skipping empty prompt: %s (file may still be in progress)",
				filepath.Base(pr.Path),
			)
			// Move empty prompts to completed/ (but don't commit)
			if err := p.promptManager.MoveToCompleted(ctx, pr.Path); err != nil {
				return errors.Wrap(ctx, err, "move empty prompt to completed")
			}
			return nil
		}
		return errors.Wrap(ctx, err, "get prompt content")
	}

	// Prepare prompt metadata and set executing status
	baseName, containerName, err := p.setupPromptMetadata(ctx, pr.Path)
	if err != nil {
		return errors.Wrap(ctx, err, "setup prompt metadata")
	}

	// Get prompt title for logging
	title, err := p.promptManager.Title(ctx, pr.Path)
	if err != nil {
		return errors.Wrap(ctx, err, "get prompt title")
	}

	log.Printf("dark-factory: executing prompt: %s", title)

	// Derive log file path: prompts/log/{basename}.log
	logFile := filepath.Join(filepath.Dir(pr.Path), "log", baseName+".log")

	// Execute via executor
	if err := p.executor.Execute(ctx, content, logFile, containerName); err != nil {
		log.Printf("dark-factory: docker container exited with error: %v", err)
		return errors.Wrap(ctx, err, "execute prompt")
	}

	log.Printf("dark-factory: docker container exited with code 0")

	// Move to completed/ before commit so it's included in the release
	if err := p.promptManager.MoveToCompleted(ctx, pr.Path); err != nil {
		return errors.Wrap(ctx, err, "move to completed")
	}

	log.Printf("dark-factory: moved %s to completed/", filepath.Base(pr.Path))

	// Use a non-cancellable context for git ops so they aren't interrupted by shutdown.
	gitCtx := context.WithoutCancel(ctx)

	// Commit the completed file separately (YOLO may have already committed code changes)
	completedPath := filepath.Join(filepath.Dir(pr.Path), "completed", filepath.Base(pr.Path))
	if err := p.releaser.CommitCompletedFile(gitCtx, completedPath); err != nil {
		return errors.Wrap(ctx, err, "commit completed file")
	}

	// Without CHANGELOG: simple commit only (no tag, no push)
	if !p.releaser.HasChangelog(gitCtx) {
		if err := p.releaser.CommitOnly(gitCtx, title); err != nil {
			return errors.Wrap(ctx, err, "commit")
		}
		log.Printf("dark-factory: committed changes")
		return nil
	}

	// With CHANGELOG: update changelog, bump version, tag, push
	bump := determineBump(title)
	nextVersion, err := p.releaser.GetNextVersion(gitCtx, bump)
	if err != nil {
		return errors.Wrap(ctx, err, "get next version")
	}

	if err := p.releaser.CommitAndRelease(gitCtx, title, bump); err != nil {
		return errors.Wrap(ctx, err, "commit and release")
	}

	log.Printf("dark-factory: committed and tagged %s", nextVersion)

	return nil
}

// setupPromptMetadata sets container name, version, and executing status in frontmatter.
// Returns baseName and containerName for use in execution.
func (p *processor) setupPromptMetadata(
	ctx context.Context,
	path string,
) (string, string, error) {
	// Derive container name from prompt filename
	baseName := strings.TrimSuffix(filepath.Base(path), ".md")
	baseName = sanitizeContainerName(baseName)
	containerName := "dark-factory-" + baseName

	// Set container name in frontmatter
	if err := p.promptManager.SetContainer(ctx, path, containerName); err != nil {
		return "", "", errors.Wrap(ctx, err, "set container name")
	}

	// Set dark-factory version in frontmatter
	if err := p.promptManager.SetVersion(ctx, path, p.versionGetter.Get()); err != nil {
		return "", "", errors.Wrap(ctx, err, "set version")
	}

	// Set status to executing
	if err := p.promptManager.SetStatus(ctx, path, string(prompt.StatusExecuting)); err != nil {
		return "", "", errors.Wrap(ctx, err, "set executing status")
	}

	return baseName, containerName, nil
}

// sanitizeContainerName ensures the name only contains Docker-safe characters [a-zA-Z0-9_-]
func sanitizeContainerName(name string) string {
	// Replace any character that is not alphanumeric, underscore, or hyphen with hyphen
	re := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	return re.ReplaceAllString(name, "-")
}

// determineBump determines the version bump type based on the title.
// Returns MinorBump for new features, PatchBump for everything else.
func determineBump(title string) git.VersionBump {
	lower := strings.ToLower(title)
	for _, kw := range []string{"add", "implement", "new", "support", "feature"} {
		if strings.Contains(lower, kw) {
			return git.MinorBump
		}
	}
	return git.PatchBump
}
