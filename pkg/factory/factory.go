// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/runner"
	"github.com/bborbe/dark-factory/pkg/watcher"
)

// CreateRunner creates a Runner that coordinates watcher and processor.
func CreateRunner(promptsDir string) runner.Runner {
	releaser := git.NewReleaser()
	promptManager := prompt.NewManager(promptsDir, releaser)

	// Communication channel between watcher and processor
	ready := make(chan struct{}, 10)

	return runner.NewRunner(
		promptsDir,
		promptManager,
		CreateLocker(promptsDir),
		CreateWatcher(promptsDir, promptManager, ready),
		CreateProcessor(promptsDir, promptManager, releaser, ready),
	)
}

// CreateWatcher creates a Watcher that normalizes filenames on file events.
func CreateWatcher(
	promptsDir string,
	promptManager prompt.Manager,
	ready chan<- struct{},
) watcher.Watcher {
	return watcher.NewWatcher(promptsDir, promptManager, ready)
}

// CreateProcessor creates a Processor that executes queued prompts.
func CreateProcessor(
	promptsDir string,
	promptManager prompt.Manager,
	releaser git.Releaser,
	ready <-chan struct{},
) processor.Processor {
	return processor.NewProcessor(
		promptsDir,
		executor.NewDockerExecutor(),
		promptManager,
		releaser,
		ready,
	)
}

// CreateLocker creates a Locker for the specified directory.
func CreateLocker(dir string) lock.Locker {
	return lock.NewLocker(dir)
}
