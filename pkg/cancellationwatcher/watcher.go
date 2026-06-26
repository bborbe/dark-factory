// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cancellationwatcher

import (
	"context"
	"path/filepath"

	"github.com/fsnotify/fsnotify"

	"github.com/bborbe/dark-factory/pkg/executor"
	log "github.com/bborbe/dark-factory/pkg/log"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/promptstate"
)

//counterfeiter:generate -o ../../mocks/cancellation-watcher.go --fake-name CancellationWatcher . Watcher

// Watcher monitors a prompt file for cancellation and stops its container when triggered.
type Watcher interface {
	// Watch starts a goroutine that watches promptPath for status==cancelled.
	// When detected, it stops/removes the container and closes the returned channel.
	// The goroutine exits when ctx is cancelled or cancellation is detected.
	//
	// containerName is passed as a string to avoid an import cycle with pkg/processor.
	// PromptLoader is a minimal local interface for the same reason.
	Watch(ctx context.Context, promptPath string, containerName string) <-chan struct{}
}

// PromptLoader is the minimal subset of processor.PromptManager that this package needs.
// Defined locally to avoid an import cycle (pkg/processor imports cancellationwatcher).
type PromptLoader interface {
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
}

type watcher struct {
	executor     executor.Executor
	promptLoader PromptLoader
}

// NewWatcher creates a Watcher that uses fsnotify to detect cancellation.
func NewWatcher(exec executor.Executor, promptLoader PromptLoader) Watcher {
	return &watcher{
		executor:     exec,
		promptLoader: promptLoader,
	}
}

func (w *watcher) Watch(
	ctx context.Context,
	promptPath string,
	containerName string,
) <-chan struct{} {
	ch := make(chan struct{})
	go w.watch(ctx, promptPath, containerName, ch)
	return ch
}

func (w *watcher) watch(
	ctx context.Context,
	promptPath string,
	containerName string,
	ch chan<- struct{},
) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.From(ctx).Warn("failed to create cancel watcher", "error", err)
		return
	}
	defer fsWatcher.Close()

	if err := fsWatcher.Add(promptPath); err != nil {
		log.From(ctx).
			Warn("failed to watch prompt file", "prompt_id", filepath.Base(promptPath), "error", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-fsWatcher.Errors:
			if !ok {
				return
			}
			log.From(ctx).Debug("cancel watcher error", "error", err)
		case event, ok := <-fsWatcher.Events:
			if !ok {
				return
			}
			if !event.Has(fsnotify.Write) {
				continue
			}
			pf, err := w.promptLoader.Load(ctx, promptPath)
			if err != nil {
				continue
			}
			if promptstate.InterpretRawTuple(
				promptstate.LocationInProgress,
				pf.Frontmatter.Status,
				pf.Frontmatter.Container,
				promptstate.DockerStateUnavailable,
			) == promptstate.StateCancelled {
				log.From(ctx).Info("prompt cancelled, stopping container",
					"container", containerName,
					"workflow_step", "cancel",
				)
				close(ch)
				w.executor.StopAndRemoveContainer(ctx, containerName)
				return
			}
		}
	}
}
