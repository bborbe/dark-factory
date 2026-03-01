// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"fmt"
	"time"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/runner"
	"github.com/bborbe/dark-factory/pkg/server"
	"github.com/bborbe/dark-factory/pkg/status"
	"github.com/bborbe/dark-factory/pkg/version"
	"github.com/bborbe/dark-factory/pkg/watcher"
)

// CreateRunner creates a Runner that coordinates watcher and processor using the provided config.
func CreateRunner(cfg config.Config, ver string) runner.Runner {
	inboxDir := cfg.InboxDir
	queueDir := cfg.QueueDir
	completedDir := cfg.CompletedDir
	releaser := git.NewReleaser()
	promptManager := prompt.NewManager(queueDir, completedDir, releaser)
	versionGetter := version.NewGetter(ver)

	// Communication channel between watcher and processor
	ready := make(chan struct{}, 10)

	return runner.NewRunner(
		inboxDir,
		queueDir,
		completedDir,
		promptManager,
		CreateLocker("."),
		CreateWatcher(
			inboxDir,
			queueDir,
			promptManager,
			ready,
			time.Duration(cfg.DebounceMs)*time.Millisecond,
		),
		CreateProcessor(
			queueDir,
			completedDir,
			promptManager,
			releaser,
			versionGetter,
			ready,
			cfg.ContainerImage,
		),
		CreateServer(
			cfg.ServerPort,
			queueDir,
			completedDir,
			promptManager,
		),
	)
}

// CreateWatcher creates a Watcher that normalizes filenames on file events.
func CreateWatcher(
	inboxDir string,
	queueDir string,
	promptManager prompt.Manager,
	ready chan<- struct{},
	debounce time.Duration,
) watcher.Watcher {
	return watcher.NewWatcher(inboxDir, queueDir, promptManager, ready, debounce)
}

// CreateProcessor creates a Processor that executes queued prompts.
func CreateProcessor(
	queueDir string,
	completedDir string,
	promptManager prompt.Manager,
	releaser git.Releaser,
	versionGetter version.Getter,
	ready <-chan struct{},
	containerImage string,
) processor.Processor {
	return processor.NewProcessor(
		queueDir,
		completedDir,
		executor.NewDockerExecutor(containerImage),
		promptManager,
		releaser,
		versionGetter,
		ready,
	)
}

// CreateLocker creates a Locker for the specified directory.
func CreateLocker(dir string) lock.Locker {
	return lock.NewLocker(dir)
}

// CreateServer creates a Server that provides HTTP endpoints for monitoring.
func CreateServer(
	port int,
	queueDir string,
	completedDir string,
	promptManager prompt.Manager,
) server.Server {
	addr := fmt.Sprintf(":%d", port)
	ideasDir := "prompts/ideas"
	statusChecker := status.NewChecker(queueDir, completedDir, ideasDir, promptManager)
	return server.NewServer(addr, statusChecker)
}
