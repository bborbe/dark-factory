// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package specwatcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bborbe/errors"
	"github.com/fsnotify/fsnotify"

	"github.com/bborbe/dark-factory/pkg/generator"
	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/spec-watcher.go --fake-name SpecWatcher . SpecWatcher

// SpecWatcher watches the specs in-progress directory and triggers generation when a spec appears there.
type SpecWatcher interface {
	Watch(ctx context.Context) error
}

// specWatcher implements SpecWatcher.
type specWatcher struct {
	inProgressDir string
	generator     generator.SpecGenerator
	debounce      time.Duration
	mu            sync.Mutex
}

// NewSpecWatcher creates a new SpecWatcher.
func NewSpecWatcher(
	inProgressDir string,
	generator generator.SpecGenerator,
	debounce time.Duration,
) SpecWatcher {
	return &specWatcher{
		inProgressDir: inProgressDir,
		generator:     generator,
		debounce:      debounce,
	}
}

// Watch starts watching the in-progress directory for new spec files.
func (w *specWatcher) Watch(ctx context.Context) error {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(ctx, err, "create watcher")
	}
	defer fsWatcher.Close()

	absInProgressDir := w.getInProgressDir()

	if err := fsWatcher.Add(absInProgressDir); err != nil {
		return errors.Wrap(ctx, err, "add watch path")
	}

	slog.Info("spec watcher started", "dir", absInProgressDir)

	// Scan for specs already present in inProgressDir before entering the event loop.
	w.scanExistingInProgress(ctx, absInProgressDir)

	var debounceMu sync.Mutex
	debounceTimers := make(map[string]*time.Timer)

	for {
		select {
		case <-ctx.Done():
			slog.Info("spec watcher shutting down")
			return nil

		case err, ok := <-fsWatcher.Errors:
			if !ok {
				return errors.Errorf(ctx, "watcher error channel closed")
			}
			slog.Info("spec watcher error", "error", err)
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
func (w *specWatcher) handleWatchEvent(
	ctx context.Context,
	event fsnotify.Event,
	debounceMu *sync.Mutex,
	debounceTimers map[string]*time.Timer,
) {
	if !strings.HasSuffix(event.Name, ".md") {
		return
	}
	if event.Op&fsnotify.Create == 0 {
		return
	}

	slog.Debug("spec file event received", "operation", event.Op.String(), "path", event.Name)

	debounceMu.Lock()
	if timer, exists := debounceTimers[event.Name]; exists {
		timer.Stop()
	}

	eventName := event.Name
	debounceTimers[eventName] = time.AfterFunc(w.debounce, func() {
		debounceMu.Lock()
		delete(debounceTimers, eventName)
		debounceMu.Unlock()
		slog.Debug("spec debounce timer fired", "path", eventName)
		w.handleFileEvent(ctx, eventName)
	})
	debounceMu.Unlock()
}

// handleFileEvent triggers generation for a spec file in inProgressDir.
func (w *specWatcher) handleFileEvent(ctx context.Context, specPath string) {
	sf, err := spec.Load(ctx, specPath)
	if err != nil {
		slog.Warn("failed to load spec", "path", specPath, "error", err)
		return
	}
	if sf.Frontmatter.Status != string(spec.StatusApproved) {
		slog.Debug(
			"skipping spec — not approved",
			"path",
			specPath,
			"status",
			sf.Frontmatter.Status,
		)
		return
	}

	slog.Info("spec file created in in-progress, triggering generation", "path", specPath)

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.generator.Generate(ctx, specPath); err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			slog.Info("spec generation cancelled", "path", specPath)
		} else {
			slog.Info("spec generation failed", "path", specPath, "error", err)
		}
	}
}

// scanExistingInProgress scans inProgressDir for .md files on startup and triggers
// generation for each. This handles specs that were moved here before the daemon started.
func (w *specWatcher) scanExistingInProgress(ctx context.Context, inProgressDir string) {
	entries, err := os.ReadDir(inProgressDir)
	if err != nil {
		slog.Info(
			"failed to read spec in-progress dir on startup",
			"dir",
			inProgressDir,
			"error",
			err,
		)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		w.handleFileEvent(ctx, filepath.Join(inProgressDir, entry.Name()))
	}
}

// getInProgressDir returns the absolute in-progress directory path.
func (w *specWatcher) getInProgressDir() string {
	if !filepath.IsAbs(w.inProgressDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return w.inProgressDir
		}
		return filepath.Join(cwd, w.inProgressDir)
	}
	return w.inProgressDir
}
