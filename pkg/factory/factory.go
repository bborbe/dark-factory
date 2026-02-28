// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bborbe/errors"
	"github.com/fsnotify/fsnotify"

	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

// Factory orchestrates the main processing loop.
type Factory struct {
	promptsDir string
	executor   executor.Executor
}

// New creates a new Factory.
func New(exec executor.Executor) *Factory {
	return &Factory{
		promptsDir: "prompts",
		executor:   exec,
	}
}

// Run executes the main processing loop:
// 1. Scan for existing queued prompts and process them
// 2. Start watching prompts/ directory for changes
// 3. On file create/write events, check if status: queued and process
// 4. Run until context is cancelled or fatal error
func (f *Factory) Run(ctx context.Context) error {
	// Set up signal handling
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("dark-factory: watching %s for queued prompts...", f.promptsDir)

	// Process any existing queued prompts first
	if err := f.processExistingQueued(ctx); err != nil {
		return errors.Wrap(ctx, err, "process existing queued prompts")
	}

	// Set up file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(ctx, err, "create watcher")
	}
	defer watcher.Close()

	// Get absolute path for prompts directory
	absPromptsDir := f.GetPromptsDir()

	// Watch the prompts directory
	if err := watcher.Add(absPromptsDir); err != nil {
		return errors.Wrap(ctx, err, "add watch path")
	}

	// Run watch loop
	return f.watchLoop(ctx, watcher)
}

// watchLoop handles the main event loop for file watching.
func (f *Factory) watchLoop(
	ctx context.Context,
	watcher *fsnotify.Watcher,
) error {
	// Debounce map: file path -> timer (protected by mutex)
	var debounceMu sync.Mutex
	debounceTimers := make(map[string]*time.Timer)

	for {
		select {
		case <-ctx.Done():
			log.Printf("dark-factory: shutting down...")
			return nil

		case err, ok := <-watcher.Errors:
			if !ok {
				return errors.Errorf(ctx, "watcher error channel closed")
			}
			log.Printf("dark-factory: watcher error: %v", err)
			return errors.Wrap(ctx, err, "watcher error")

		case event, ok := <-watcher.Events:
			if !ok {
				return errors.Errorf(ctx, "watcher events channel closed")
			}

			f.handleWatchEvent(ctx, event, &debounceMu, debounceTimers)
		}
	}
}

// handleWatchEvent processes a file system event with debouncing.
func (f *Factory) handleWatchEvent(
	ctx context.Context,
	event fsnotify.Event,
	debounceMu *sync.Mutex,
	debounceTimers map[string]*time.Timer,
) {
	// Only process .md files on Write or Create events
	if !strings.HasSuffix(event.Name, ".md") {
		return
	}
	if event.Op&fsnotify.Write == 0 && event.Op&fsnotify.Create == 0 {
		return
	}

	// Debounce: cancel existing timer for this file
	debounceMu.Lock()
	if timer, exists := debounceTimers[event.Name]; exists {
		timer.Stop()
	}

	// Set new timer
	eventName := event.Name // Capture for closure
	debounceTimers[eventName] = time.AfterFunc(500*time.Millisecond, func() {
		debounceMu.Lock()
		delete(debounceTimers, eventName)
		debounceMu.Unlock()
		f.handleFileEvent(ctx, eventName)
	})
	debounceMu.Unlock()
}

// processExistingQueued scans for and processes any existing queued prompts.
func (f *Factory) processExistingQueued(ctx context.Context) error {
	for {
		// Scan for queued prompts
		queued, err := prompt.ListQueued(ctx, f.promptsDir)
		if err != nil {
			return errors.Wrap(ctx, err, "list queued prompts")
		}

		// No more queued prompts - done
		if len(queued) == 0 {
			return nil
		}

		// Pick first prompt (already sorted alphabetically)
		p := queued[0]

		log.Printf("dark-factory: found queued prompt: %s", filepath.Base(p.Path))

		// Process the prompt
		if err := f.processPrompt(ctx, p); err != nil {
			// Mark as failed
			if setErr := prompt.SetStatus(ctx, p.Path, "failed"); setErr != nil {
				return errors.Wrap(ctx, setErr, "set failed status")
			}
			return errors.Wrap(ctx, err, "process prompt")
		}

		// Move to completed/ (this also sets status to "completed")
		if err := prompt.MoveToCompleted(ctx, p.Path); err != nil {
			return errors.Wrap(ctx, err, "move to completed")
		}

		log.Printf("dark-factory: moved %s to completed/", filepath.Base(p.Path))
		log.Printf("dark-factory: watching %s for queued prompts...", f.promptsDir)

		// Loop again to process next prompt
	}
}

// handleFileEvent checks if a file should be picked up and processes it.
// Files are picked up UNLESS they have an explicit skip status (executing, completed, failed).
func (f *Factory) handleFileEvent(ctx context.Context, filePath string) {
	// Read frontmatter to check status
	fm, err := prompt.ReadFrontmatter(ctx, filePath)
	if err != nil {
		// Ignore files with read errors
		return
	}

	// Pick up files UNLESS they have an explicit skip status
	if fm.Status == "executing" || fm.Status == "completed" || fm.Status == "failed" {
		return
	}

	log.Printf("dark-factory: found queued prompt: %s", filepath.Base(filePath))

	// Normalize status to "queued" for consistency
	status := fm.Status
	if status == "" {
		status = "queued"
	}

	p := prompt.Prompt{
		Path:   filePath,
		Status: status,
	}

	// Process the prompt
	if err := f.processPrompt(ctx, p); err != nil {
		// Mark as failed
		if setErr := prompt.SetStatus(ctx, p.Path, "failed"); setErr != nil {
			log.Printf("dark-factory: failed to set failed status: %v", setErr)
			return
		}
		log.Printf("dark-factory: prompt failed: %v", err)
		return
	}

	// MoveToCompleted now sets status to "completed" internally before moving
	if err := prompt.MoveToCompleted(ctx, p.Path); err != nil {
		log.Printf("dark-factory: failed to move to completed: %v", err)
		return
	}

	log.Printf("dark-factory: moved %s to completed/", filepath.Base(p.Path))
	log.Printf("dark-factory: watching %s for queued prompts...", f.promptsDir)
}

// processPrompt executes a single prompt and commits the result.
func (f *Factory) processPrompt(ctx context.Context, p prompt.Prompt) error {
	// Set status to executing
	if err := prompt.SetStatus(ctx, p.Path, "executing"); err != nil {
		return errors.Wrap(ctx, err, "set executing status")
	}

	// Get prompt title for logging
	title, err := prompt.Title(ctx, p.Path)
	if err != nil {
		return errors.Wrap(ctx, err, "get prompt title")
	}

	log.Printf("dark-factory: executing prompt: %s", title)

	// Get prompt content
	content, err := prompt.Content(ctx, p.Path)
	if err != nil {
		return errors.Wrap(ctx, err, "get prompt content")
	}

	// Execute via executor
	if err := f.executor.Execute(ctx, content); err != nil {
		log.Printf("dark-factory: docker container exited with error: %v", err)
		return errors.Wrap(ctx, err, "execute prompt")
	}

	log.Printf("dark-factory: docker container exited with code 0")

	// Commit and release
	nextVersion, err := git.GetNextVersion(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "get next version")
	}

	if err := git.CommitAndRelease(ctx, title); err != nil {
		return errors.Wrap(ctx, err, "commit and release")
	}

	log.Printf("dark-factory: committed and tagged %s", nextVersion)

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
