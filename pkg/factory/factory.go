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
	libtime "github.com/bborbe/time"

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
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (prompt.Manager, git.Releaser) {
	releaser := git.NewReleaser()
	promptManager := prompt.NewManager(
		inboxDir,
		inProgressDir,
		completedDir,
		releaser,
		currentDateTimeGetter,
	)
	return promptManager, releaser
}

// createOptionalServer creates a Server when port > 0, or returns nil.
func createOptionalServer(
	cfg config.Config,
	inboxDir, inProgressDir, completedDir string,
	promptManager prompt.Manager,
) server.Server {
	if cfg.ServerPort > 0 {
		return CreateServer(
			cfg.ServerPort,
			inboxDir,
			inProgressDir,
			completedDir,
			cfg.Prompts.LogDir,
			promptManager,
		)
	}
	return nil
}

// createOptionalReviewPoller creates a ReviewPoller when AutoReview is enabled, or returns nil.
func createOptionalReviewPoller(
	cfg config.Config,
	promptManager prompt.Manager,
) review.ReviewPoller {
	if cfg.AutoReview {
		return CreateReviewPoller(cfg, promptManager)
	}
	return nil
}

// CreateRunner creates a Runner that coordinates watcher and processor using the provided config.
func CreateRunner(cfg config.Config, ver string) runner.Runner {
	inboxDir := cfg.Prompts.InboxDir
	inProgressDir := cfg.Prompts.InProgressDir
	completedDir := cfg.Prompts.CompletedDir
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	promptManager, releaser := createPromptManager(
		inboxDir,
		inProgressDir,
		completedDir,
		currentDateTimeGetter,
	)
	versionGetter := version.NewGetter(ver)
	projectName := project.Name(cfg.ProjectName)
	ghToken := cfg.ResolvedGitHubToken()
	ready := make(chan struct{}, 10)
	specGen := CreateSpecGenerator(cfg, cfg.ContainerImage, currentDateTimeGetter)

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
			currentDateTimeGetter,
		),
		CreateProcessor(
			inProgressDir, completedDir, cfg.Prompts.LogDir, projectName,
			promptManager, releaser, versionGetter, ready,
			cfg.ContainerImage, cfg.Model, cfg.NetrcFile, cfg.GitconfigFile,
			cfg.PR, ghToken, cfg.AutoMerge, cfg.AutoRelease, cfg.AutoReview,
			cfg.ValidationCommand, cfg.Specs.InboxDir, cfg.Specs.InProgressDir,
			cfg.Specs.CompletedDir, cfg.VerificationGate, cfg.DefaultBranch,
			cfg.Env, currentDateTimeGetter,
		),
		createOptionalServer(cfg, inboxDir, inProgressDir, completedDir, promptManager),
		createOptionalReviewPoller(cfg, promptManager),
		CreateSpecWatcher(cfg, specGen, currentDateTimeGetter),
	)
}

// CreateOneShotRunner creates an OneShotRunner that drains the queue and exits.
func CreateOneShotRunner(cfg config.Config, ver string) runner.OneShotRunner {
	inboxDir := cfg.Prompts.InboxDir
	inProgressDir := cfg.Prompts.InProgressDir
	completedDir := cfg.Prompts.CompletedDir
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	promptManager, releaser := createPromptManager(
		inboxDir,
		inProgressDir,
		completedDir,
		currentDateTimeGetter,
	)
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
			cfg.PR,
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
			cfg.Env,
			currentDateTimeGetter,
		),
		CreateSpecGenerator(cfg, cfg.ContainerImage, currentDateTimeGetter),
		currentDateTimeGetter,
	)
}

// CreateSpecGenerator creates a SpecGenerator using the Docker executor.
func CreateSpecGenerator(
	cfg config.Config,
	containerImage string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) generator.SpecGenerator {
	return generator.NewSpecGenerator(
		executor.NewDockerExecutor(
			containerImage,
			project.Name(cfg.ProjectName),
			cfg.Model,
			cfg.NetrcFile,
			cfg.GitconfigFile,
			cfg.Env,
		),
		cfg.Prompts.InboxDir,
		cfg.Prompts.CompletedDir,
		cfg.Specs.InboxDir,
		cfg.Specs.LogDir,
		currentDateTimeGetter,
	)
}

// CreateSpecWatcher creates a SpecWatcher that triggers generation when a spec appears in inProgressDir.
func CreateSpecWatcher(
	cfg config.Config,
	gen generator.SpecGenerator,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) specwatcher.SpecWatcher {
	return specwatcher.NewSpecWatcher(
		cfg.Specs.InProgressDir,
		gen,
		time.Duration(cfg.DebounceMs)*time.Millisecond,
		currentDateTimeGetter,
	)
}

// CreateWatcher creates a Watcher that normalizes filenames on file events.
func CreateWatcher(
	inProgressDir string,
	inboxDir string,
	promptManager prompt.Manager,
	ready chan<- struct{},
	debounce time.Duration,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) watcher.Watcher {
	return watcher.NewWatcher(
		inProgressDir,
		inboxDir,
		promptManager,
		ready,
		debounce,
		currentDateTimeGetter,
	)
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
	pr bool,
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
	env map[string]string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) processor.Processor {
	return processor.NewProcessor(
		inProgressDir,
		completedDir,
		logDir,
		projectName,
		executor.NewDockerExecutor(
			containerImage,
			projectName,
			model,
			netrcFile,
			gitconfigFile,
			env,
		),
		promptManager,
		releaser,
		versionGetter,
		ready,
		pr,
		git.NewBrancher(git.WithDefaultBranch(defaultBranch)),
		git.NewPRCreator(ghToken),
		git.NewCloner(),
		git.NewPRMerger(ghToken, currentDateTimeGetter),
		autoMerge,
		autoRelease,
		autoReview,
		spec.NewAutoCompleter(
			inProgressDir,
			completedDir,
			specsInboxDir,
			specsInProgressDir,
			specsCompletedDir,
			currentDateTimeGetter,
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
		git.NewPRMerger(ghToken, libtime.NewCurrentDateTime()),
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
		libtime.NewCurrentDateTime(),
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
		libtime.NewCurrentDateTime(),
	)
}

// CreateRequeueCommand creates a RequeueCommand.
func CreateRequeueCommand(cfg config.Config) cmd.RequeueCommand {
	return cmd.NewRequeueCommand(cfg.Prompts.InProgressDir, libtime.NewCurrentDateTime())
}

// CreatePromptVerifyCommand creates a PromptVerifyCommand.
func CreatePromptVerifyCommand(cfg config.Config) cmd.PromptVerifyCommand {
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	promptManager, releaser := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		currentDateTimeGetter,
	)
	ghToken := cfg.ResolvedGitHubToken()
	return cmd.NewPromptVerifyCommand(
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		promptManager,
		releaser,
		cfg.PR,
		git.NewBrancher(git.WithDefaultBranch(cfg.DefaultBranch)),
		git.NewPRCreator(ghToken),
		currentDateTimeGetter,
	)
}

// CreateApproveCommand creates an ApproveCommand.
func CreateApproveCommand(cfg config.Config) cmd.ApproveCommand {
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		currentDateTimeGetter,
	)

	return cmd.NewApproveCommand(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		promptManager,
		currentDateTimeGetter,
	)
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
		libtime.NewCurrentDateTime(),
	)
}

// CreateSpecCompleteCommand creates a SpecCompleteCommand.
func CreateSpecCompleteCommand(cfg config.Config) cmd.SpecCompleteCommand {
	return cmd.NewSpecCompleteCommand(
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.CompletedDir,
		libtime.NewCurrentDateTime(),
	)
}

// CreateCombinedStatusCommand creates a CombinedStatusCommand.
func CreateCombinedStatusCommand(cfg config.Config) cmd.CombinedStatusCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		libtime.NewCurrentDateTime(),
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
		libtime.NewCurrentDateTime(),
	)
}

// CreatePromptShowCommand creates a PromptShowCommand.
func CreatePromptShowCommand(cfg config.Config) cmd.PromptShowCommand {
	return cmd.NewPromptShowCommand(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.LogDir,
		libtime.NewCurrentDateTime(),
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
		libtime.NewCurrentDateTime(),
	)
}
