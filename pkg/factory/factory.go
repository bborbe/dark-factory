// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"fmt"
	"net/http"
	"time"

	libhttp "github.com/bborbe/http"

	"github.com/bborbe/dark-factory/pkg/cmd"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/project"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/runner"
	"github.com/bborbe/dark-factory/pkg/server"
	"github.com/bborbe/dark-factory/pkg/status"
	"github.com/bborbe/dark-factory/pkg/version"
	"github.com/bborbe/dark-factory/pkg/watcher"
)

const defaultIdeasDir = "prompts/ideas"

// createPromptManager creates shared prompt.Manager and git.Releaser dependencies.
func createPromptManager(queueDir string, completedDir string) (prompt.Manager, git.Releaser) {
	releaser := git.NewReleaser()
	promptManager := prompt.NewManager(queueDir, completedDir, releaser)
	return promptManager, releaser
}

// CreateRunner creates a Runner that coordinates watcher and processor using the provided config.
func CreateRunner(cfg config.Config, ver string) runner.Runner {
	inboxDir := cfg.InboxDir
	queueDir := cfg.QueueDir
	completedDir := cfg.CompletedDir
	promptManager, releaser := createPromptManager(queueDir, completedDir)
	versionGetter := version.NewGetter(ver)

	// Resolve project name
	projectName := project.Name(cfg.ProjectName)

	// Resolve GitHub token (warns internally if env var is empty)
	ghToken := cfg.ResolvedGitHubToken()

	// Communication channel between watcher and processor
	ready := make(chan struct{}, 10)

	var srv server.Server
	if cfg.ServerPort > 0 {
		srv = CreateServer(
			cfg.ServerPort,
			inboxDir,
			queueDir,
			completedDir,
			cfg.LogDir,
			promptManager,
		)
	}

	return runner.NewRunner(
		inboxDir,
		queueDir,
		completedDir,
		promptManager,
		CreateLocker("."),
		CreateWatcher(
			queueDir,
			promptManager,
			ready,
			time.Duration(cfg.DebounceMs)*time.Millisecond,
		),
		CreateProcessor(
			queueDir,
			completedDir,
			cfg.LogDir,
			projectName,
			promptManager,
			releaser,
			versionGetter,
			ready,
			cfg.ContainerImage,
			cfg.Model,
			cfg.Workflow,
			ghToken,
			cfg.AutoMerge,
			cfg.AutoRelease,
		),
		srv,
	)
}

// CreateWatcher creates a Watcher that normalizes filenames on file events.
func CreateWatcher(
	queueDir string,
	promptManager prompt.Manager,
	ready chan<- struct{},
	debounce time.Duration,
) watcher.Watcher {
	return watcher.NewWatcher(queueDir, promptManager, ready, debounce)
}

// CreateProcessor creates a Processor that executes queued prompts.
func CreateProcessor(
	queueDir string,
	completedDir string,
	logDir string,
	projectName string,
	promptManager prompt.Manager,
	releaser git.Releaser,
	versionGetter version.Getter,
	ready <-chan struct{},
	containerImage string,
	model string,
	workflow config.Workflow,
	ghToken string,
	autoMerge bool,
	autoRelease bool,
) processor.Processor {
	return processor.NewProcessor(
		queueDir,
		completedDir,
		logDir,
		projectName,
		executor.NewDockerExecutor(containerImage, projectName, model),
		promptManager,
		releaser,
		versionGetter,
		ready,
		workflow,
		git.NewBrancher(),
		git.NewPRCreator(ghToken),
		git.NewWorktree(),
		git.NewPRMerger(ghToken),
		autoMerge,
		autoRelease,
	)
}

// CreateLocker creates a Locker for the specified directory.
func CreateLocker(dir string) lock.Locker {
	return lock.NewLocker(dir)
}

// CreateServer creates a Server that provides HTTP endpoints for monitoring.
func CreateServer(
	port int,
	inboxDir string,
	queueDir string,
	completedDir string,
	logDir string,
	promptManager prompt.Manager,
) server.Server {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	statusChecker := status.NewChecker(
		queueDir,
		completedDir,
		defaultIdeasDir,
		logDir,
		port,
		promptManager,
	)

	// Build the mux with all routes
	mux := http.NewServeMux()
	mux.Handle("/health", libhttp.NewErrorHandler(server.NewHealthHandler()))
	mux.Handle("/api/v1/status", libhttp.NewErrorHandler(server.NewStatusHandler(statusChecker)))
	mux.Handle("/api/v1/queue", libhttp.NewErrorHandler(server.NewQueueHandler(statusChecker)))
	mux.Handle(
		"/api/v1/queue/action",
		libhttp.NewErrorHandler(server.NewQueueActionHandler(inboxDir, queueDir, promptManager)),
	)
	mux.Handle(
		"/api/v1/queue/action/all",
		libhttp.NewErrorHandler(server.NewQueueActionHandler(inboxDir, queueDir, promptManager)),
	)
	mux.Handle("/api/v1/inbox", libhttp.NewErrorHandler(server.NewInboxHandler(inboxDir)))
	mux.Handle(
		"/api/v1/completed",
		libhttp.NewErrorHandler(server.NewCompletedHandler(statusChecker)),
	)

	// Create server with libhttp (includes sane defaults for timeouts)
	runFunc := libhttp.NewServer(addr, mux)
	return server.NewServer(runFunc)
}

// CreateStatusCommand creates a StatusCommand.
func CreateStatusCommand(cfg config.Config) cmd.StatusCommand {
	promptManager, _ := createPromptManager(cfg.QueueDir, cfg.CompletedDir)

	statusChecker := status.NewChecker(
		cfg.QueueDir,
		cfg.CompletedDir,
		defaultIdeasDir,
		cfg.LogDir,
		cfg.ServerPort,
		promptManager,
	)
	formatter := status.NewFormatter()

	return cmd.NewStatusCommand(statusChecker, formatter)
}

// CreateQueueCommand creates a QueueCommand.
func CreateQueueCommand(cfg config.Config) cmd.QueueCommand {
	promptManager, _ := createPromptManager(cfg.QueueDir, cfg.CompletedDir)

	return cmd.NewQueueCommand(cfg.InboxDir, cfg.QueueDir, promptManager)
}
