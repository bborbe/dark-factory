// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	libhttp "github.com/bborbe/http"

	"github.com/bborbe/dark-factory/pkg/cmd"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/generator"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/project"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/review"
	"github.com/bborbe/dark-factory/pkg/runner"
	"github.com/bborbe/dark-factory/pkg/server"
	"github.com/bborbe/dark-factory/pkg/spec"
	"github.com/bborbe/dark-factory/pkg/specwatcher"
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

	var reviewPoller review.ReviewPoller
	if cfg.AutoReview {
		reviewPoller = CreateReviewPoller(cfg, promptManager)
	}

	specGen := CreateSpecGenerator(cfg, cfg.ContainerImage)
	specWatcher := CreateSpecWatcher(cfg, specGen)

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
			cfg.AutoReview,
			"specs",
		),
		srv,
		reviewPoller,
		specWatcher,
	)
}

// CreateSpecGenerator creates a SpecGenerator using the Docker executor.
func CreateSpecGenerator(cfg config.Config, containerImage string) generator.SpecGenerator {
	return generator.NewSpecGenerator(
		executor.NewDockerExecutor(containerImage, project.Name(cfg.ProjectName), cfg.Model),
		cfg.InboxDir,
		cfg.CompletedDir,
		cfg.SpecDir,
		cfg.LogDir,
	)
}

// CreateSpecWatcher creates a SpecWatcher that triggers generation on approved specs.
func CreateSpecWatcher(cfg config.Config, gen generator.SpecGenerator) specwatcher.SpecWatcher {
	return specwatcher.NewSpecWatcher(
		cfg.SpecDir,
		gen,
		time.Duration(cfg.DebounceMs)*time.Millisecond,
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
	autoReview bool,
	specsDir string,
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
		autoReview,
		spec.NewAutoCompleter(queueDir, completedDir, specsDir),
	)
}

// CreateReviewPoller creates a ReviewPoller that watches in_review prompts and handles approvals/changes.
func CreateReviewPoller(cfg config.Config, promptManager prompt.Manager) review.ReviewPoller {
	ghToken := cfg.ResolvedGitHubToken()

	allowedReviewers := cfg.AllowedReviewers
	if len(allowedReviewers) == 0 && cfg.UseCollaborators {
		allowedReviewers = fetchCollaborators(ghToken)
	}

	return review.NewReviewPoller(
		cfg.QueueDir,
		cfg.InboxDir,
		allowedReviewers,
		cfg.MaxReviewRetries,
		time.Duration(cfg.PollIntervalSec)*time.Second,
		git.NewReviewFetcher(ghToken),
		git.NewPRMerger(ghToken),
		promptManager,
		review.NewFixPromptGenerator(),
	)
}

// fetchCollaborators retrieves repository collaborator logins via the gh CLI.
// Returns nil on any error (non-fatal — caller falls back to no allowed reviewers).
func fetchCollaborators(ghToken string) []string {
	// Get the repo name with owner
	nameCmd := exec.Command(
		"gh",
		"repo",
		"view",
		"--json",
		"nameWithOwner",
		"--jq",
		".nameWithOwner",
	) // #nosec G204 -- fixed args, no user input
	if ghToken != "" {
		nameCmd.Env = append(os.Environ(), "GH_TOKEN="+ghToken)
	}
	nameOut, err := nameCmd.Output()
	if err != nil {
		slog.Warn("failed to get repo name for collaborators", "error", err)
		return nil
	}
	repoName := strings.TrimSpace(string(nameOut))

	// Fetch collaborator logins
	collabCmd := exec.Command(
		"gh",
		"api",
		"repos/"+repoName+"/collaborators",
		"--jq",
		".[].login",
	) // #nosec G204 -- repoName from gh CLI, not user input
	if ghToken != "" {
		collabCmd.Env = append(os.Environ(), "GH_TOKEN="+ghToken)
	}
	collabOut, err := collabCmd.Output()
	if err != nil {
		slog.Warn("failed to fetch collaborators", "error", err)
		return nil
	}

	var result []string
	for _, line := range strings.Split(strings.TrimSpace(string(collabOut)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result
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
		lock.FilePath("."),
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
		lock.FilePath("."),
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

// CreateListCommand creates a ListCommand.
func CreateListCommand(cfg config.Config) cmd.ListCommand {
	return cmd.NewListCommand(cfg.InboxDir, cfg.QueueDir, cfg.CompletedDir)
}

// CreateRequeueCommand creates a RequeueCommand.
func CreateRequeueCommand(cfg config.Config) cmd.RequeueCommand {
	return cmd.NewRequeueCommand(cfg.QueueDir)
}

// CreateApproveCommand creates an ApproveCommand.
func CreateApproveCommand(cfg config.Config) cmd.ApproveCommand {
	promptManager, _ := createPromptManager(cfg.QueueDir, cfg.CompletedDir)

	return cmd.NewApproveCommand(cfg.InboxDir, cfg.QueueDir, promptManager)
}

// CreateSpecListCommand creates a SpecListCommand.
func CreateSpecListCommand(cfg config.Config) cmd.SpecListCommand {
	counter := prompt.NewCounter(cfg.InboxDir, cfg.QueueDir, cfg.CompletedDir)
	return cmd.NewSpecListCommand(spec.NewLister(cfg.SpecDir), counter)
}

// CreateSpecStatusCommand creates a SpecStatusCommand.
func CreateSpecStatusCommand(cfg config.Config) cmd.SpecStatusCommand {
	counter := prompt.NewCounter(cfg.InboxDir, cfg.QueueDir, cfg.CompletedDir)
	return cmd.NewSpecStatusCommand(spec.NewLister(cfg.SpecDir), counter)
}

// CreateSpecApproveCommand creates a SpecApproveCommand.
func CreateSpecApproveCommand(cfg config.Config) cmd.SpecApproveCommand {
	return cmd.NewSpecApproveCommand(cfg.SpecDir)
}

// CreateSpecVerifyCommand creates a SpecVerifyCommand.
func CreateSpecVerifyCommand(cfg config.Config) cmd.SpecVerifyCommand {
	return cmd.NewSpecVerifyCommand(cfg.SpecDir)
}

// CreateCombinedStatusCommand creates a CombinedStatusCommand.
func CreateCombinedStatusCommand(cfg config.Config) cmd.CombinedStatusCommand {
	promptManager, _ := createPromptManager(cfg.QueueDir, cfg.CompletedDir)

	statusChecker := status.NewChecker(
		cfg.QueueDir,
		cfg.CompletedDir,
		defaultIdeasDir,
		cfg.LogDir,
		lock.FilePath("."),
		cfg.ServerPort,
		promptManager,
	)
	formatter := status.NewFormatter()
	counter := prompt.NewCounter(cfg.InboxDir, cfg.QueueDir, cfg.CompletedDir)

	return cmd.NewCombinedStatusCommand(
		statusChecker,
		formatter,
		spec.NewLister(cfg.SpecDir),
		counter,
	)
}

// CreateCombinedListCommand creates a CombinedListCommand.
func CreateCombinedListCommand(cfg config.Config) cmd.CombinedListCommand {
	counter := prompt.NewCounter(cfg.InboxDir, cfg.QueueDir, cfg.CompletedDir)
	return cmd.NewCombinedListCommand(
		cfg.InboxDir,
		cfg.QueueDir,
		cfg.CompletedDir,
		spec.NewLister(cfg.SpecDir),
		counter,
	)
}
