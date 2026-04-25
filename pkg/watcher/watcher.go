// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
	"github.com/fsnotify/fsnotify"
)

//counterfeiter:generate -o ../../mocks/watcher.go --fake-name Watcher . Watcher

// Watcher watches the prompts directory and normalizes filenames.
type Watcher interface {
	Watch(ctx context.Context) error
}

// NewWatcher creates a new Watcher with the specified debounce duration.
func NewWatcher(
	inProgressDir string,
	inboxDir string,
	promptManager PromptManager,
	ready chan<- struct{},
	debounce time.Duration,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) Watcher {
	return &watcher{
		inProgressDir:         inProgressDir,
		inboxDir:              inboxDir,
		promptManager:         promptManager,
		ready:                 ready,
		debounce:              debounce,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// watcher implements Watcher.
type watcher struct {
	inProgressDir         string
	inboxDir              string
	promptManager         PromptManager
	ready                 chan<- struct{}
	debounce              time.Duration
	currentDateTimeGetter libtime.CurrentDateTimeGetter
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

	// Get absolute path for in-progress directory
	absInProgressDir := w.getInProgressDir()

	// Watch the in-progress directory
	if err := fsWatcher.Add(absInProgressDir); err != nil {
		return errors.Wrap(ctx, err, "add watch path")
	}

	slog.Info("watcher started", "dir", absInProgressDir)

	// Debounce map: file path -> timer (protected by mutex)
	var debounceMu sync.Mutex
	debounceTimers := make(map[string]*time.Timer)

	for {
		select {
		case <-ctx.Done():
			slog.Info("watcher shutting down")
			return nil

		case err, ok := <-fsWatcher.Errors:
			if !ok {
				return errors.Errorf(ctx, "watcher error channel closed")
			}
			slog.Warn("watcher error", "error", err)
			continue

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
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) &&
		!event.Has(fsnotify.Chmod) {
		return
	}

	slog.Debug("file event received", "operation", event.Op.String(), "path", event.Name)

	// Debounce: cancel existing timer for this file
	debounceMu.Lock()
	if timer, exists := debounceTimers[event.Name]; exists {
		timer.Stop()
		slog.Debug("debounce timer reset", "path", event.Name)
	}

	// Set new timer
	eventName := event.Name // Capture for closure
	debounceTimers[eventName] = time.AfterFunc(w.debounce, func() {
		debounceMu.Lock()
		delete(debounceTimers, eventName)
		debounceMu.Unlock()
		slog.Debug("debounce timer fired", "path", eventName)
		w.handleFileEvent(ctx)
	})
	debounceMu.Unlock()
}

// handleFileEvent normalizes filenames in in-progress directory.
func (w *watcher) handleFileEvent(ctx context.Context) {
	slog.Debug("normalizing filenames", "dir", w.inProgressDir)

	// Normalize filenames in in-progress directory
	renames, err := w.promptManager.NormalizeFilenames(ctx, w.inProgressDir)
	if err != nil {
		slog.Info("failed to normalize filenames", "error", err)
		return
	}

	// Log renames
	for _, rename := range renames {
		slog.Debug("renamed file",
			"from", filepath.Base(rename.OldPath),
			"to", filepath.Base(rename.NewPath))
	}

	// Stamp created timestamps on inbox files that don't have one yet
	w.stampCreatedTimestamps(ctx)

	// Signal processor that files are ready
	select {
	case w.ready <- struct{}{}:
		slog.Debug("signaled processor ready")
	default:
		// Non-blocking send - processor may already be working
		slog.Debug("processor already working, signal skipped")
	}
}

// stampCreatedTimestamps scans inboxDir and sets Created on any prompt that lacks it.
func (w *watcher) stampCreatedTimestamps(ctx context.Context) {
	entries, err := os.ReadDir(w.inboxDir)
	if err != nil {
		slog.Debug("inbox scan failed for created stamping", "error", err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(w.inboxDir, entry.Name())
		pf, err := w.promptManager.Load(ctx, path)
		if err != nil || pf == nil {
			continue
		}
		if pf.Frontmatter.Created != "" {
			continue
		}
		pf.Frontmatter.Created = time.Time(w.currentDateTimeGetter.Now()).UTC().Format(time.RFC3339)
		if err := pf.Save(ctx); err != nil {
			slog.Debug("failed to stamp created timestamp", "path", path, "error", err)
			continue
		}
		slog.Info("stamped created timestamp", "file", entry.Name())
	}
}

// getInProgressDir returns the in-progress directory.
func (w *watcher) getInProgressDir() string {
	// If relative path, make it absolute
	if !filepath.IsAbs(w.inProgressDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return w.inProgressDir
		}
		return filepath.Join(cwd, w.inProgressDir)
	}
	return w.inProgressDir
}
