// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package watcher

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bborbe/errors"
	"github.com/fsnotify/fsnotify"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

// Watcher watches the prompts directory and normalizes filenames.
//
//counterfeiter:generate -o ../../mocks/watcher.go --fake-name Watcher . Watcher
type Watcher interface {
	Watch(ctx context.Context) error
}

// watcher implements Watcher.
type watcher struct {
	inboxDir      string
	queueDir      string
	promptManager prompt.Manager
	ready         chan<- struct{}
	debounce      time.Duration
}

// NewWatcher creates a new Watcher with the specified debounce duration.
func NewWatcher(
	inboxDir string,
	queueDir string,
	promptManager prompt.Manager,
	ready chan<- struct{},
	debounce time.Duration,
) Watcher {
	return &watcher{
		inboxDir:      inboxDir,
		queueDir:      queueDir,
		promptManager: promptManager,
		ready:         ready,
		debounce:      debounce,
	}
}

// Watch starts watching the prompts directory for file changes.
// It normalizes filenames on every event and signals the processor when done.
func (w *watcher) Watch(ctx context.Context) error {
	// Set up file watcher
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(ctx, err, "create watcher")
	}
	defer fsWatcher.Close()

	// Get absolute path for inbox directory
	absInboxDir := w.getInboxDir()

	// Watch the inbox directory
	if err := fsWatcher.Add(absInboxDir); err != nil {
		return errors.Wrap(ctx, err, "add watch path")
	}

	log.Printf("dark-factory: watcher started on %s", absInboxDir)

	// Debounce map: file path -> timer (protected by mutex)
	var debounceMu sync.Mutex
	debounceTimers := make(map[string]*time.Timer)

	for {
		select {
		case <-ctx.Done():
			log.Printf("dark-factory: watcher shutting down")
			return nil

		case err, ok := <-fsWatcher.Errors:
			if !ok {
				return errors.Errorf(ctx, "watcher error channel closed")
			}
			log.Printf("dark-factory: watcher error: %v", err)
			return errors.Wrap(ctx, err, "watcher error")

		case event, ok := <-fsWatcher.Events:
			if !ok {
				return errors.Errorf(ctx, "watcher events channel closed")
			}

			w.handleWatchEvent(ctx, event, &debounceMu, debounceTimers)
		}
	}
}

// handleWatchEvent processes a file system event with debouncing.
func (w *watcher) handleWatchEvent(
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
	debounceTimers[eventName] = time.AfterFunc(w.debounce, func() {
		debounceMu.Lock()
		delete(debounceTimers, eventName)
		debounceMu.Unlock()
		w.handleFileEvent(ctx)
	})
	debounceMu.Unlock()
}

// handleFileEvent normalizes filenames and moves files from inbox to queue when dirs differ.
func (w *watcher) handleFileEvent(ctx context.Context) {
	// Normalize filenames in inbox directory
	renames, err := w.promptManager.NormalizeFilenames(ctx, w.inboxDir)
	if err != nil {
		log.Printf("dark-factory: failed to normalize filenames: %v", err)
		return
	}

	// Log renames
	for _, rename := range renames {
		log.Printf("dark-factory: renamed %s -> %s",
			filepath.Base(rename.OldPath), filepath.Base(rename.NewPath))
	}

	// If inbox != queue, move files from inbox to queue
	if w.inboxDir != w.queueDir {
		if err := w.moveInboxToQueue(ctx); err != nil {
			log.Printf("dark-factory: failed to move files from inbox to queue: %v", err)
			return
		}
	}

	// Signal processor that files are ready
	select {
	case w.ready <- struct{}{}:
	default:
		// Non-blocking send - processor may already be working
	}
}

// getInboxDir returns the inbox directory.
func (w *watcher) getInboxDir() string {
	// If relative path, make it absolute
	if !filepath.IsAbs(w.inboxDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return w.inboxDir
		}
		return filepath.Join(cwd, w.inboxDir)
	}
	return w.inboxDir
}

// moveInboxToQueue moves all .md files from inbox to queue directory.
func (w *watcher) moveInboxToQueue(ctx context.Context) error {
	entries, err := os.ReadDir(w.inboxDir)
	if err != nil {
		return errors.Wrap(ctx, err, "read inbox directory")
	}

	// Ensure queue directory exists
	if err := os.MkdirAll(w.queueDir, 0750); err != nil {
		return errors.Wrap(ctx, err, "create queue directory")
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		oldPath := filepath.Join(w.inboxDir, entry.Name())
		newPath := filepath.Join(w.queueDir, entry.Name())

		if err := os.Rename(oldPath, newPath); err != nil {
			return errors.Wrapf(ctx, err, "move %s to queue", entry.Name())
		}

		log.Printf("dark-factory: moved %s from inbox to queue", entry.Name())
	}

	return nil
}
