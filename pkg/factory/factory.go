// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"context"
	stderrors "errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
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
type Factory interface {
	Run(ctx context.Context) error
	SetPromptsDir(dir string)
	GetPromptsDir() string
}

// factory orchestrates the main processing loop.
type factory struct {
	promptsDir string
	executor   executor.Executor
}

// New creates a new Factory.
func New(exec executor.Executor) Factory {
	return &factory{
		promptsDir: "prompts",
		executor:   exec,
	}
}

// Run executes the main processing loop:
// 1. Scan for existing queued prompts and process them
// 2. Start watching prompts/ directory for changes
// 3. On file create/write events, check if status: queued and process
// 4. Run until context is cancelled or fatal error
func (f *factory) Run(ctx context.Context) error {
	// Set up signal handling
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("dark-factory: watching %s for queued prompts...", f.promptsDir)

	// Reset any stuck "executing" prompts from previous crash
	if err := prompt.ResetExecuting(ctx, f.promptsDir); err != nil {
		return errors.Wrap(ctx, err, "reset executing prompts")
	}

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
func (f *factory) watchLoop(
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
func (f *factory) handleWatchEvent(
	ctx context.Context,
	event fsnotify.Event,
	debounceMu *sync.Mutex,
	debounceTimers map[string]*time.Timer,
) {
	// Only process .md files on Write or Create events
	if !strings.HasSuffix(event.Name, ".md") {
		return
	}
	if event.Op&fsnotify.Write == 0 && event.Op&fsnotify.Create == 0 &&
		event.Op&fsnotify.Chmod == 0 {
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
func (f *factory) processExistingQueued(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
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

		// Process the prompt (includes moving to completed/ and committing)
		if err := f.processPrompt(ctx, p); err != nil {
			// Mark as failed
			if setErr := prompt.SetStatus(ctx, p.Path, "failed"); setErr != nil {
				return errors.Wrap(ctx, setErr, "set failed status")
			}
			return errors.Wrap(ctx, err, "process prompt")
		}

		log.Printf("dark-factory: watching %s for queued prompts...", f.promptsDir)

		// Loop again to process next prompt
	}
}

// handleFileEvent checks if a file should be picked up and processes it.
// Files are picked up UNLESS they have an explicit skip status (executing, completed, failed).
func (f *factory) handleFileEvent(ctx context.Context, filePath string) {
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

	// Process the prompt (includes moving to completed/ and committing)
	if err := f.processPrompt(ctx, p); err != nil {
		// Mark as failed
		if setErr := prompt.SetStatus(ctx, p.Path, "failed"); setErr != nil {
			log.Printf("dark-factory: failed to set failed status: %v", setErr)
			return
		}
		log.Printf("dark-factory: prompt failed: %v", err)
		return
	}

	log.Printf("dark-factory: watching %s for queued prompts...", f.promptsDir)
}

// processPrompt executes a single prompt and commits the result.
func (f *factory) processPrompt(ctx context.Context, p prompt.Prompt) error {
	// Get prompt content first to check if empty
	content, err := prompt.Content(ctx, p.Path)
	if err != nil {
		// If prompt is empty, move to completed and skip execution
		if stderrors.Is(err, prompt.ErrEmptyPrompt) {
			log.Printf(
				"dark-factory: skipping empty prompt: %s (file may still be in progress)",
				filepath.Base(p.Path),
			)
			// Move empty prompts to completed/ (but don't commit)
			if err := prompt.MoveToCompleted(ctx, p.Path); err != nil {
				return errors.Wrap(ctx, err, "move empty prompt to completed")
			}
			return nil
		}
		return errors.Wrap(ctx, err, "get prompt content")
	}

	// Derive container name from prompt filename
	baseName := strings.TrimSuffix(filepath.Base(p.Path), ".md")
	// Sanitize baseName to Docker-safe charset [a-zA-Z0-9_-]
	baseName = sanitizeContainerName(baseName)
	containerName := "dark-factory-" + baseName

	// Set container name in frontmatter
	if err := prompt.SetContainer(ctx, p.Path, containerName); err != nil {
		return errors.Wrap(ctx, err, "set container name")
	}

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

	// Derive log file path: prompts/log/{basename}.log
	logFile := filepath.Join(filepath.Dir(p.Path), "log", baseName+".log")

	// Execute via executor
	if err := f.executor.Execute(ctx, content, logFile, containerName); err != nil {
		log.Printf("dark-factory: docker container exited with error: %v", err)
		return errors.Wrap(ctx, err, "execute prompt")
	}

	log.Printf("dark-factory: docker container exited with code 0")

	// Move to completed/ before commit so it's included in the release
	if err := prompt.MoveToCompleted(ctx, p.Path); err != nil {
		return errors.Wrap(ctx, err, "move to completed")
	}

	log.Printf("dark-factory: moved %s to completed/", filepath.Base(p.Path))

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
func (f *factory) SetPromptsDir(dir string) {
	f.promptsDir = dir
}

// GetPromptsDir returns the prompts directory.
func (f *factory) GetPromptsDir() string {
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

// sanitizeContainerName ensures the name only contains Docker-safe characters [a-zA-Z0-9_-]
func sanitizeContainerName(name string) string {
	// Replace any character that is not alphanumeric, underscore, or hyphen with hyphen
	re := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	return re.ReplaceAllString(name, "-")
}
