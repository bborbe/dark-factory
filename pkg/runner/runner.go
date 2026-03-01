// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

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
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

// Runner orchestrates the main processing loop.
type Runner interface {
	Run(ctx context.Context) error
}

// runner orchestrates the main processing loop.
type runner struct {
	promptsDir    string
	executor      executor.Executor
	promptManager prompt.Manager
	releaser      git.Releaser
	locker        lock.Locker
	processMu     sync.Mutex // Serializes prompt processing from file events
}

// NewRunner creates a new Runner.
func NewRunner(
	promptsDir string,
	exec executor.Executor,
	promptManager prompt.Manager,
	releaser git.Releaser,
	locker lock.Locker,
) Runner {
	return &runner{
		promptsDir:    promptsDir,
		executor:      exec,
		promptManager: promptManager,
		releaser:      releaser,
		locker:        locker,
	}
}

// Run executes the main processing loop:
// 1. Acquire instance lock to prevent concurrent runs
// 2. Scan for existing queued prompts and process them
// 3. Start watching prompts/ directory for changes
// 4. On file create/write events, check if status: queued and process
// 5. Run until context is cancelled or fatal error
func (r *runner) Run(ctx context.Context) error {
	// Acquire instance lock
	if err := r.locker.Acquire(ctx); err != nil {
		return errors.Wrap(ctx, err, "acquire lock")
	}
	defer func() {
		if err := r.locker.Release(context.Background()); err != nil {
			log.Printf("dark-factory: failed to release lock: %v", err)
		}
	}()

	log.Printf("dark-factory: acquired lock %s/.dark-factory.lock", r.promptsDir)

	// Set up signal handling
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("dark-factory: watching %s for queued prompts...", r.promptsDir)

	// Reset any stuck "executing" prompts from previous crash
	if err := r.promptManager.ResetExecuting(ctx); err != nil {
		return errors.Wrap(ctx, err, "reset executing prompts")
	}

	// Normalize filenames before processing
	renames, err := r.promptManager.NormalizeFilenames(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "normalize filenames")
	}
	for _, rename := range renames {
		log.Printf("dark-factory: renamed %s -> %s",
			filepath.Base(rename.OldPath), filepath.Base(rename.NewPath))
	}

	// Process any existing queued prompts first
	if err := r.processExistingQueued(ctx); err != nil {
		return errors.Wrap(ctx, err, "process existing queued prompts")
	}

	// Set up file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(ctx, err, "create watcher")
	}
	defer watcher.Close()

	// Get absolute path for prompts directory
	absPromptsDir := r.getPromptsDir()

	// Watch the prompts directory
	if err := watcher.Add(absPromptsDir); err != nil {
		return errors.Wrap(ctx, err, "add watch path")
	}

	// Run watch loop
	return r.watchLoop(ctx, watcher)
}

// watchLoop handles the main event loop for file watching.
func (r *runner) watchLoop(
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

			r.handleWatchEvent(ctx, event, &debounceMu, debounceTimers)
		}
	}
}

// handleWatchEvent processes a file system event with debouncing.
func (r *runner) handleWatchEvent(
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
		r.handleFileEvent(ctx, eventName)
	})
	debounceMu.Unlock()
}

// processExistingQueued scans for and processes any existing queued prompts.
func (r *runner) processExistingQueued(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		// Scan for queued prompts
		queued, err := r.promptManager.ListQueued(ctx)
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
		if err := r.processPrompt(ctx, p); err != nil {
			// Mark as failed — file may have been moved to completed/ before the error.
			if setErr := r.promptManager.SetStatus(ctx, p.Path, "failed"); setErr != nil {
				log.Printf("dark-factory: failed to set failed status: %v", setErr)
			}
			return errors.Wrap(ctx, err, "process prompt")
		}

		log.Printf("dark-factory: watching %s for queued prompts...", r.promptsDir)

		// Loop again to process next prompt
	}
}

// handleFileEvent checks if a file should be picked up and processes it.
// Files are picked up UNLESS they have an explicit skip status (executing, completed, failed).
func (r *runner) handleFileEvent(ctx context.Context, filePath string) {
	// Serialize processing to prevent concurrent execution
	r.processMu.Lock()
	defer r.processMu.Unlock()

	// Skip if another prompt is currently executing
	if r.promptManager.HasExecuting(ctx) {
		return
	}

	// Normalize filenames first (handles invalid naming)
	renames, err := r.promptManager.NormalizeFilenames(ctx)
	if err != nil {
		log.Printf("dark-factory: failed to normalize filenames: %v", err)
		return
	}

	// Check if our file was renamed
	actualPath := filePath
	for _, rename := range renames {
		log.Printf("dark-factory: renamed %s -> %s",
			filepath.Base(rename.OldPath), filepath.Base(rename.NewPath))
		if rename.OldPath == filePath {
			actualPath = rename.NewPath
		}
	}

	// Read frontmatter to check status
	fm, err := r.promptManager.ReadFrontmatter(ctx, actualPath)
	if err != nil {
		// Ignore files with read errors
		return
	}

	// Pick up files UNLESS they have an explicit skip status
	if fm.Status == "executing" || fm.Status == "completed" || fm.Status == "failed" {
		return
	}

	log.Printf("dark-factory: found queued prompt: %s", filepath.Base(actualPath))

	// Normalize status to "queued" for consistency
	status := fm.Status
	if status == "" {
		status = "queued"
	}

	p := prompt.Prompt{
		Path:   actualPath,
		Status: status,
	}

	// Process the prompt (includes moving to completed/ and committing)
	if err := r.processPrompt(ctx, p); err != nil {
		// Mark as failed
		if setErr := r.promptManager.SetStatus(ctx, p.Path, "failed"); setErr != nil {
			log.Printf("dark-factory: failed to set failed status: %v", setErr)
			return
		}
		log.Printf("dark-factory: prompt failed: %v", err)
		return
	}

	log.Printf("dark-factory: watching %s for queued prompts...", r.promptsDir)

	// Process any other queued prompts that arrived during execution
	if err := r.processExistingQueued(ctx); err != nil {
		log.Printf("dark-factory: failed to process queued prompts: %v", err)
	}
}

// processPrompt executes a single prompt and commits the result.
func (r *runner) processPrompt(ctx context.Context, p prompt.Prompt) error {
	// Get prompt content first to check if empty
	content, err := r.promptManager.Content(ctx, p.Path)
	if err != nil {
		// If prompt is empty, move to completed and skip execution
		if stderrors.Is(err, prompt.ErrEmptyPrompt) {
			log.Printf(
				"dark-factory: skipping empty prompt: %s (file may still be in progress)",
				filepath.Base(p.Path),
			)
			// Move empty prompts to completed/ (but don't commit)
			if err := r.promptManager.MoveToCompleted(ctx, p.Path); err != nil {
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
	if err := r.promptManager.SetContainer(ctx, p.Path, containerName); err != nil {
		return errors.Wrap(ctx, err, "set container name")
	}

	// Set status to executing
	if err := r.promptManager.SetStatus(ctx, p.Path, "executing"); err != nil {
		return errors.Wrap(ctx, err, "set executing status")
	}

	// Get prompt title for logging
	title, err := r.promptManager.Title(ctx, p.Path)
	if err != nil {
		return errors.Wrap(ctx, err, "get prompt title")
	}

	log.Printf("dark-factory: executing prompt: %s", title)

	// Derive log file path: prompts/log/{basename}.log
	logFile := filepath.Join(filepath.Dir(p.Path), "log", baseName+".log")

	// Execute via executor
	if err := r.executor.Execute(ctx, content, logFile, containerName); err != nil {
		log.Printf("dark-factory: docker container exited with error: %v", err)
		return errors.Wrap(ctx, err, "execute prompt")
	}

	log.Printf("dark-factory: docker container exited with code 0")

	// Move to completed/ before commit so it's included in the release
	if err := r.promptManager.MoveToCompleted(ctx, p.Path); err != nil {
		return errors.Wrap(ctx, err, "move to completed")
	}

	log.Printf("dark-factory: moved %s to completed/", filepath.Base(p.Path))

	// Use a non-cancellable context for git ops so they aren't interrupted by shutdown.
	gitCtx := context.WithoutCancel(ctx)

	// Commit the completed file separately (YOLO may have already committed code changes)
	completedPath := filepath.Join(filepath.Dir(p.Path), "completed", filepath.Base(p.Path))
	if err := r.releaser.CommitCompletedFile(gitCtx, completedPath); err != nil {
		return errors.Wrap(ctx, err, "commit completed file")
	}

	// Without CHANGELOG: simple commit only (no tag, no push)
	if !r.releaser.HasChangelog(gitCtx) {
		if err := r.releaser.CommitOnly(gitCtx, title); err != nil {
			return errors.Wrap(ctx, err, "commit")
		}
		log.Printf("dark-factory: committed changes")
		return nil
	}

	// With CHANGELOG: update changelog, bump version, tag, push
	bump := determineBump(title)
	nextVersion, err := r.releaser.GetNextVersion(gitCtx, bump)
	if err != nil {
		return errors.Wrap(ctx, err, "get next version")
	}

	if err := r.releaser.CommitAndRelease(gitCtx, title, bump); err != nil {
		return errors.Wrap(ctx, err, "commit and release")
	}

	log.Printf("dark-factory: committed and tagged %s", nextVersion)

	return nil
}

// getPromptsDir returns the prompts directory.
func (r *runner) getPromptsDir() string {
	// If relative path, make it absolute
	if !filepath.IsAbs(r.promptsDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return r.promptsDir
		}
		return filepath.Join(cwd, r.promptsDir)
	}
	return r.promptsDir
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
