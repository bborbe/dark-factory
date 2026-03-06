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

// SpecWatcher watches the specs directory and triggers generation when a spec is approved.
//
//counterfeiter:generate -o ../../mocks/spec-watcher.go --fake-name SpecWatcher . SpecWatcher
type SpecWatcher interface {
	Watch(ctx context.Context) error
}

// specWatcher implements SpecWatcher.
type specWatcher struct {
	specsDir  string
	generator generator.SpecGenerator
	debounce  time.Duration
	mu        sync.Mutex
}

// NewSpecWatcher creates a new SpecWatcher.
func NewSpecWatcher(
	specsDir string,
	generator generator.SpecGenerator,
	debounce time.Duration,
) SpecWatcher {
	return &specWatcher{
		specsDir:  specsDir,
		generator: generator,
		debounce:  debounce,
	}
}

// Watch starts watching the specs directory for file changes.
func (w *specWatcher) Watch(ctx context.Context) error {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(ctx, err, "create watcher")
	}
	defer fsWatcher.Close()

	absSpecsDir := w.getSpecsDir()

	if err := fsWatcher.Add(absSpecsDir); err != nil {
		return errors.Wrap(ctx, err, "add watch path")
	}

	slog.Info("spec watcher started", "dir", absSpecsDir)

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
	if event.Op&fsnotify.Write == 0 && event.Op&fsnotify.Create == 0 &&
		event.Op&fsnotify.Chmod == 0 {
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

// handleFileEvent checks spec status and triggers generation if approved.
func (w *specWatcher) handleFileEvent(ctx context.Context, specPath string) {
	sf, err := spec.Load(ctx, specPath)
	if err != nil {
		slog.Info("failed to load spec file", "path", specPath, "error", err)
		return
	}

	if sf.Frontmatter.Status != string(spec.StatusApproved) {
		slog.Debug("spec not approved, skipping", "path", specPath, "status", sf.Frontmatter.Status)
		return
	}

	slog.Info("spec approved, triggering generation", "path", specPath)

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.generator.Generate(ctx, specPath); err != nil {
		slog.Info("spec generation failed", "path", specPath, "error", err)
	}
}

// getSpecsDir returns the absolute specs directory path.
func (w *specWatcher) getSpecsDir() string {
	if !filepath.IsAbs(w.specsDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return w.specsDir
		}
		return filepath.Join(cwd, w.specsDir)
	}
	return w.specsDir
}
