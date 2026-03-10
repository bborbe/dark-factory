// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"context"
	"fmt"
	"net/http"
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
func createPromptManager(
	inboxDir string,
	inProgressDir string,
	completedDir string,
) (prompt.Manager, git.Releaser) {
	releaser := git.NewReleaser()
	promptManager := prompt.NewManager(inboxDir, inProgressDir, completedDir, releaser)
	return promptManager, releaser
}

// CreateRunner creates a Runner that coordinates watcher and processor using the provided config.
func CreateRunner(cfg config.Config, ver string) runner.Runner {
	inboxDir := cfg.Prompts.InboxDir
	inProgressDir := cfg.Prompts.InProgressDir
	completedDir := cfg.Prompts.CompletedDir
	promptManager, releaser := createPromptManager(inboxDir, inProgressDir, completedDir)
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
			inProgressDir,
			completedDir,
			cfg.Prompts.LogDir,
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
		inProgressDir,
		completedDir,
		cfg.Prompts.LogDir,
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.CompletedDir,
		cfg.Specs.LogDir,
		promptManager,
		CreateLocker("."),
		CreateWatcher(
			inProgressDir,
			inboxDir,
			promptManager,
			ready,
			time.Duration(cfg.DebounceMs)*time.Millisecond,
		),
		CreateProcessor(
			inProgressDir,
			completedDir,
			cfg.Prompts.LogDir,
			projectName,
			promptManager,
			releaser,
			versionGetter,
			ready,
			cfg.ContainerImage,
			cfg.Model,
			cfg.NetrcFile,
			cfg.GitconfigFile,
			cfg.Workflow,
			ghToken,
			cfg.AutoMerge,
			cfg.AutoRelease,
			cfg.AutoReview,
			cfg.ValidationCommand,
			cfg.Specs.InboxDir,
			cfg.Specs.InProgressDir,
			cfg.Specs.CompletedDir,
			cfg.VerificationGate,
			cfg.DefaultBranch,
		),
		srv,
		reviewPoller,
		specWatcher,
	)
}

// CreateOneShotRunner creates an OneShotRunner that drains the queue and exits.
func CreateOneShotRunner(cfg config.Config, ver string) runner.OneShotRunner {
	inboxDir := cfg.Prompts.InboxDir
	inProgressDir := cfg.Prompts.InProgressDir
	completedDir := cfg.Prompts.CompletedDir
	promptManager, releaser := createPromptManager(inboxDir, inProgressDir, completedDir)
	versionGetter := version.NewGetter(ver)
	projectName := project.Name(cfg.ProjectName)
	ghToken := cfg.ResolvedGitHubToken()

	// One-shot mode uses a nil ready channel — ProcessQueue never reads from it.
	ready := make(chan struct{}, 10)

	return runner.NewOneShotRunner(
		inboxDir,
		inProgressDir,
		completedDir,
		cfg.Prompts.LogDir,
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.CompletedDir,
		cfg.Specs.LogDir,
		promptManager,
		CreateLocker("."),
		CreateProcessor(
			inProgressDir,
			completedDir,
			cfg.Prompts.LogDir,
			projectName,
			promptManager,
			releaser,
			versionGetter,
			ready,
			cfg.ContainerImage,
			cfg.Model,
			cfg.NetrcFile,
			cfg.GitconfigFile,
			cfg.Workflow,
			ghToken,
			cfg.AutoMerge,
			cfg.AutoRelease,
			cfg.AutoReview,
			cfg.ValidationCommand,
			cfg.Specs.InboxDir,
			cfg.Specs.InProgressDir,
			cfg.Specs.CompletedDir,
			cfg.VerificationGate,
			cfg.DefaultBranch,
		),
	)
}

// CreateSpecGenerator creates a SpecGenerator using the Docker executor.
func CreateSpecGenerator(cfg config.Config, containerImage string) generator.SpecGenerator {
	return generator.NewSpecGenerator(
		executor.NewDockerExecutor(
			containerImage,
			project.Name(cfg.ProjectName),
			cfg.Model,
			cfg.NetrcFile,
			cfg.GitconfigFile,
		),
		cfg.Prompts.InboxDir,
		cfg.Prompts.CompletedDir,
		cfg.Specs.InboxDir,
		cfg.Specs.LogDir,
	)
}

// CreateSpecWatcher creates a SpecWatcher that triggers generation when a spec appears in inProgressDir.
func CreateSpecWatcher(cfg config.Config, gen generator.SpecGenerator) specwatcher.SpecWatcher {
	return specwatcher.NewSpecWatcher(
		cfg.Specs.InProgressDir,
		gen,
		time.Duration(cfg.DebounceMs)*time.Millisecond,
	)
}

// CreateWatcher creates a Watcher that normalizes filenames on file events.
func CreateWatcher(
	inProgressDir string,
	inboxDir string,
	promptManager prompt.Manager,
	ready chan<- struct{},
	debounce time.Duration,
) watcher.Watcher {
	return watcher.NewWatcher(inProgressDir, inboxDir, promptManager, ready, debounce)
}

// CreateProcessor creates a Processor that executes queued prompts.
func CreateProcessor(
	inProgressDir string,
	completedDir string,
	logDir string,
	projectName string,
	promptManager prompt.Manager,
	releaser git.Releaser,
	versionGetter version.Getter,
	ready <-chan struct{},
	containerImage string,
	model string,
	netrcFile string,
	gitconfigFile string,
	workflow config.Workflow,
	ghToken string,
	autoMerge bool,
	autoRelease bool,
	autoReview bool,
	validationCommand string,
	specsInboxDir string,
	specsInProgressDir string,
	specsCompletedDir string,
	verificationGate bool,
	defaultBranch string,
) processor.Processor {
	return processor.NewProcessor(
		inProgressDir,
		completedDir,
		logDir,
		projectName,
		executor.NewDockerExecutor(containerImage, projectName, model, netrcFile, gitconfigFile),
		promptManager,
		releaser,
		versionGetter,
		ready,
		workflow,
		git.NewBrancher(git.WithDefaultBranch(defaultBranch)),
		git.NewPRCreator(ghToken),
		git.NewCloner(),
		git.NewPRMerger(ghToken),
		autoMerge,
		autoRelease,
		autoReview,
		spec.NewAutoCompleter(
			inProgressDir,
			completedDir,
			specsInboxDir,
			specsInProgressDir,
			specsCompletedDir,
		),
		spec.NewLister(specsInboxDir, specsInProgressDir, specsCompletedDir),
		validationCommand,
		verificationGate,
	)
}

// CreateReviewPoller creates a ReviewPoller that watches in_review prompts and handles approvals/changes.
func CreateReviewPoller(cfg config.Config, promptManager prompt.Manager) review.ReviewPoller {
	ghToken := cfg.ResolvedGitHubToken()

	repoNameFetcher := git.NewGHRepoNameFetcher(ghToken)
	collaboratorLister := git.NewGHCollaboratorLister(ghToken)
	fetcher := git.NewCollaboratorFetcher(
		repoNameFetcher,
		collaboratorLister,
		cfg.UseCollaborators,
		cfg.AllowedReviewers,
	)
	allowedReviewers := fetcher.Fetch(context.Background())

	return review.NewReviewPoller(
		cfg.Prompts.InProgressDir,
		cfg.Prompts.InboxDir,
		allowedReviewers,
		cfg.MaxReviewRetries,
		time.Duration(cfg.PollIntervalSec)*time.Second,
		git.NewReviewFetcher(ghToken),
		git.NewPRMerger(ghToken),
		promptManager,
		review.NewFixPromptGenerator(),
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
	inProgressDir string,
	completedDir string,
	logDir string,
	promptManager prompt.Manager,
) server.Server {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	statusChecker := status.NewChecker(
		inProgressDir,
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
		libhttp.NewErrorHandler(
			server.NewQueueActionHandler(inboxDir, inProgressDir, promptManager),
		),
	)
	mux.Handle(
		"/api/v1/queue/action/all",
		libhttp.NewErrorHandler(
			server.NewQueueActionHandler(inboxDir, inProgressDir, promptManager),
		),
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
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)

	statusChecker := status.NewChecker(
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		defaultIdeasDir,
		cfg.Prompts.LogDir,
		lock.FilePath("."),
		cfg.ServerPort,
		promptManager,
	)
	formatter := status.NewFormatter()

	return cmd.NewStatusCommand(statusChecker, formatter)
}

// CreateListCommand creates a ListCommand.
func CreateListCommand(cfg config.Config) cmd.ListCommand {
	return cmd.NewListCommand(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)
}

// CreateRequeueCommand creates a RequeueCommand.
func CreateRequeueCommand(cfg config.Config) cmd.RequeueCommand {
	return cmd.NewRequeueCommand(cfg.Prompts.InProgressDir)
}

// CreatePromptVerifyCommand creates a PromptVerifyCommand.
func CreatePromptVerifyCommand(cfg config.Config) cmd.PromptVerifyCommand {
	promptManager, releaser := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)
	ghToken := cfg.ResolvedGitHubToken()
	return cmd.NewPromptVerifyCommand(
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		promptManager,
		releaser,
		cfg.Workflow,
		git.NewBrancher(git.WithDefaultBranch(cfg.DefaultBranch)),
		git.NewPRCreator(ghToken),
	)
}

// CreateApproveCommand creates an ApproveCommand.
func CreateApproveCommand(cfg config.Config) cmd.ApproveCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)

	return cmd.NewApproveCommand(cfg.Prompts.InboxDir, cfg.Prompts.InProgressDir, promptManager)
}

// CreateSpecListCommand creates a SpecListCommand.
func CreateSpecListCommand(cfg config.Config) cmd.SpecListCommand {
	counter := prompt.NewCounter(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)
	return cmd.NewSpecListCommand(
		spec.NewLister(cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir),
		counter,
	)
}

// CreateSpecStatusCommand creates a SpecStatusCommand.
func CreateSpecStatusCommand(cfg config.Config) cmd.SpecStatusCommand {
	counter := prompt.NewCounter(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)
	return cmd.NewSpecStatusCommand(
		spec.NewLister(cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir),
		counter,
	)
}

// CreateSpecApproveCommand creates a SpecApproveCommand.
func CreateSpecApproveCommand(cfg config.Config) cmd.SpecApproveCommand {
	return cmd.NewSpecApproveCommand(
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.CompletedDir,
	)
}

// CreateSpecCompleteCommand creates a SpecCompleteCommand.
func CreateSpecCompleteCommand(cfg config.Config) cmd.SpecCompleteCommand {
	return cmd.NewSpecCompleteCommand(
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.CompletedDir,
	)
}

// CreateCombinedStatusCommand creates a CombinedStatusCommand.
func CreateCombinedStatusCommand(cfg config.Config) cmd.CombinedStatusCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)

	statusChecker := status.NewChecker(
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		defaultIdeasDir,
		cfg.Prompts.LogDir,
		lock.FilePath("."),
		cfg.ServerPort,
		promptManager,
	)
	formatter := status.NewFormatter()
	counter := prompt.NewCounter(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)

	return cmd.NewCombinedStatusCommand(
		statusChecker,
		formatter,
		spec.NewLister(cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir),
		counter,
	)
}

// CreateSpecShowCommand creates a SpecShowCommand.
func CreateSpecShowCommand(cfg config.Config) cmd.SpecShowCommand {
	counter := prompt.NewCounter(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)
	return cmd.NewSpecShowCommand(
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.CompletedDir,
		counter,
	)
}

// CreatePromptShowCommand creates a PromptShowCommand.
func CreatePromptShowCommand(cfg config.Config) cmd.PromptShowCommand {
	return cmd.NewPromptShowCommand(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.LogDir,
	)
}

// CreateCombinedListCommand creates a CombinedListCommand.
func CreateCombinedListCommand(cfg config.Config) cmd.CombinedListCommand {
	counter := prompt.NewCounter(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)
	return cmd.NewCombinedListCommand(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		spec.NewLister(cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir),
		counter,
	)
}
