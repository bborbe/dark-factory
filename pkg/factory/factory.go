// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	libhttp "github.com/bborbe/http"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/cmd"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/generator"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/notifier"
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

// providerDeps holds the provider-specific git operation implementations.
type providerDeps struct {
	prCreator           git.PRCreator
	prMerger            git.PRMerger
	reviewFetcher       git.ReviewFetcher
	collaboratorFetcher git.CollaboratorFetcher
	brancher            git.Brancher
}

// createProviderDeps returns the git provider implementations based on cfg.Provider.
// For github (or empty): uses gh CLI implementations (existing behavior).
// For bitbucket-server: uses Bitbucket Server REST API implementations.
func createProviderDeps(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) providerDeps {
	if cfg.Provider == config.ProviderBitbucketServer {
		return createBitbucketProviderDeps(cfg, currentDateTimeGetter)
	}
	return createGitHubProviderDeps(cfg, currentDateTimeGetter)
}

// createGitHubProviderDeps returns GitHub gh-CLI-backed implementations.
func createGitHubProviderDeps(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) providerDeps {
	ghToken := cfg.ResolvedGitHubToken()
	repoNameFetcher := git.NewGHRepoNameFetcher(ghToken)
	collaboratorLister := git.NewGHCollaboratorLister(ghToken)
	collaboratorFetcher := git.NewCollaboratorFetcher(
		repoNameFetcher,
		collaboratorLister,
		cfg.UseCollaborators,
		cfg.AllowedReviewers,
	)
	return providerDeps{
		prCreator:           git.NewPRCreator(ghToken),
		prMerger:            git.NewPRMerger(ghToken, currentDateTimeGetter),
		reviewFetcher:       git.NewReviewFetcher(ghToken),
		collaboratorFetcher: collaboratorFetcher,
		brancher:            git.NewBrancher(git.WithDefaultBranch(cfg.DefaultBranch)),
	}
}

// createBitbucketProviderDeps returns Bitbucket Server REST API-backed implementations.
// Parses project and repo from the current git remote URL.
// On error (e.g. unparseable remote URL), logs a warning and returns non-nil structs that
// will fail at operation time with a clear error — startup is not blocked.
func createBitbucketProviderDeps(
	cfg config.Config,
	_ libtime.CurrentDateTimeGetter,
) providerDeps {
	ctx := context.Background()
	token := cfg.ResolvedBitbucketToken()
	baseURL := cfg.Bitbucket.BaseURL

	coords, err := git.ParseBitbucketRemoteFromGit(ctx, "origin")
	if err != nil {
		slog.Warn(
			"bitbucket: failed to parse git remote URL; PR operations will fail",
			"error",
			err,
		)
		coords = &git.BitbucketRemoteCoords{Project: "", Repo: ""}
	}

	// Fetch current user (for excluding from reviewers) — best-effort
	currentUser := fetchBitbucketCurrentUser(ctx, baseURL, token)

	// Build collaborator fetcher (default reviewers plugin) with current user excluded
	collaboratorFetcher := git.NewBitbucketCollaboratorFetcher(
		baseURL, token, coords.Project, coords.Repo, cfg.DefaultBranch, currentUser,
	)

	// Fetch reviewers now; explicit config overrides plugin
	reviewers := collaboratorFetcher.Fetch(ctx)
	if len(cfg.AllowedReviewers) > 0 {
		reviewers = cfg.AllowedReviewers
	}

	return providerDeps{
		prCreator: git.NewBitbucketPRCreator(
			baseURL, token, coords.Project, coords.Repo, cfg.DefaultBranch, reviewers,
		),
		prMerger: git.NewBitbucketPRMerger(baseURL, token, coords.Project, coords.Repo),
		reviewFetcher: git.NewBitbucketReviewFetcher(
			baseURL,
			token,
			coords.Project,
			coords.Repo,
		),
		collaboratorFetcher: collaboratorFetcher,
		brancher:            git.NewBrancher(git.WithDefaultBranch(cfg.DefaultBranch)),
	}
}

// fetchBitbucketCurrentUser fetches the current Bitbucket Server username via the whoami endpoint.
// Returns empty string on error (graceful degradation — reviewer exclusion will not apply).
func fetchBitbucketCurrentUser(ctx context.Context, baseURL, token string) string {
	req, err := http.NewRequestWithContext(
		ctx, "GET",
		strings.TrimRight(baseURL, "/")+"/plugins/servlet/applinks/whoami",
		nil,
	)
	if err != nil {
		slog.Warn("bitbucket: failed to create whoami request", "error", err)
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("bitbucket: whoami request failed", "error", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Warn("bitbucket: whoami returned non-200", "status", resp.StatusCode)
		return ""
	}
	body, _ := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(body))
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
	ready := make(chan struct{}, 10)
	specGen := CreateSpecGenerator(cfg, cfg.ContainerImage, currentDateTimeGetter)
	deps := createProviderDeps(cfg, currentDateTimeGetter)

	n := CreateNotifier(cfg)

	var srv server.Server
	if cfg.ServerPort > 0 {
		srv = CreateServer(
			cfg.ServerPort,
			inboxDir,
			inProgressDir,
			completedDir,
			cfg.Prompts.LogDir,
			promptManager,
			currentDateTimeGetter,
		)
	}

	var poller review.ReviewPoller
	if cfg.AutoReview {
		poller = CreateReviewPoller(cfg, promptManager, projectName, n)
	}

	proc := CreateProcessor(
		inProgressDir, completedDir, cfg.Prompts.LogDir, projectName,
		promptManager, releaser, versionGetter, ready,
		cfg.ContainerImage, cfg.Model, cfg.NetrcFile, cfg.GitconfigFile,
		cfg.PR, cfg.Worktree,
		deps.brancher, deps.prCreator, deps.prMerger,
		cfg.AutoMerge, cfg.AutoRelease, cfg.AutoReview,
		cfg.ValidationCommand, cfg.ValidationPrompt,
		cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir,
		cfg.VerificationGate, cfg.Env, currentDateTimeGetter, n,
	)
	watcher := CreateWatcher(inProgressDir, inboxDir, promptManager, ready,
		time.Duration(cfg.DebounceMs)*time.Millisecond, currentDateTimeGetter)
	return runner.NewRunner(
		inboxDir, inProgressDir, completedDir, cfg.Prompts.LogDir,
		cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir, cfg.Specs.LogDir,
		promptManager, CreateLocker("."), watcher, proc, srv, poller,
		CreateSpecWatcher(cfg, specGen, currentDateTimeGetter), projectName,
		executor.NewDockerContainerChecker(), n,
	)
}

// CreateOneShotRunner creates an OneShotRunner that drains the queue and exits.
func CreateOneShotRunner(cfg config.Config, ver string, autoApprove bool) runner.OneShotRunner {
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
	deps := createProviderDeps(cfg, currentDateTimeGetter)

	// One-shot mode uses a nil ready channel — ProcessQueue never reads from it.
	ready := make(chan struct{}, 10)

	n := CreateNotifier(cfg)

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
			cfg.Worktree,
			deps.brancher,
			deps.prCreator,
			deps.prMerger,
			cfg.AutoMerge,
			cfg.AutoRelease,
			cfg.AutoReview,
			cfg.ValidationCommand,
			cfg.ValidationPrompt,
			cfg.Specs.InboxDir,
			cfg.Specs.InProgressDir,
			cfg.Specs.CompletedDir,
			cfg.VerificationGate,
			cfg.Env,
			currentDateTimeGetter,
			n,
		),
		CreateSpecGenerator(cfg, cfg.ContainerImage, currentDateTimeGetter),
		currentDateTimeGetter,
		executor.NewDockerContainerChecker(),
		autoApprove,
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
		executor.NewDockerContainerChecker(),
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
	worktree bool,
	brancher git.Brancher,
	prCreator git.PRCreator,
	prMerger git.PRMerger,
	autoMerge bool,
	autoRelease bool,
	autoReview bool,
	validationCommand string,
	validationPrompt string,
	specsInboxDir string,
	specsInProgressDir string,
	specsCompletedDir string,
	verificationGate bool,
	env map[string]string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	n notifier.Notifier,
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
		worktree,
		brancher,
		prCreator,
		git.NewCloner(),
		prMerger,
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
			projectName,
			n,
		),
		spec.NewLister(currentDateTimeGetter, specsInboxDir, specsInProgressDir, specsCompletedDir),
		validationCommand,
		validationPrompt,
		verificationGate,
		n,
	)
}

// CreateReviewPoller creates a ReviewPoller that watches in_review prompts and handles approvals/changes.
func CreateReviewPoller(
	cfg config.Config,
	promptManager prompt.Manager,
	projectName string,
	n notifier.Notifier,
) review.ReviewPoller {
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	deps := createProviderDeps(cfg, currentDateTimeGetter)

	return review.NewReviewPoller(
		cfg.Prompts.InProgressDir,
		cfg.Prompts.InboxDir,
		deps.collaboratorFetcher,
		cfg.MaxReviewRetries,
		time.Duration(cfg.PollIntervalSec)*time.Second,
		deps.reviewFetcher,
		deps.prMerger,
		promptManager,
		review.NewFixPromptGenerator(),
		projectName,
		n,
	)
}

// CreateNotifier creates a Notifier from config, or a no-op if no channels are configured.
func CreateNotifier(cfg config.Config) notifier.Notifier {
	var notifiers []notifier.Notifier
	if token := cfg.ResolvedTelegramBotToken(); token != "" {
		chatID := cfg.ResolvedTelegramChatID()
		if chatID != "" {
			notifiers = append(notifiers, notifier.NewTelegramNotifier(token, chatID))
		}
	}
	if webhook := cfg.ResolvedDiscordWebhook(); webhook != "" {
		notifiers = append(notifiers, notifier.NewDiscordNotifier(webhook))
	}
	return notifier.NewMultiNotifier(notifiers...)
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
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) server.Server {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	statusChecker := status.NewChecker(
		inProgressDir,
		completedDir,
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
	// Both routes share a single handler instance. The handler inspects the URL path
	// suffix to distinguish single-file (/api/v1/queue/action) from all-files (/api/v1/queue/action/all) operations.
	queueActionHandler := libhttp.NewErrorHandler(
		server.NewQueueActionHandler(inboxDir, inProgressDir, promptManager, currentDateTimeGetter),
	)
	mux.Handle("/api/v1/queue/action", queueActionHandler)
	mux.Handle("/api/v1/queue/action/all", queueActionHandler)
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

// CreateCancelCommand creates a CancelCommand.
func CreateCancelCommand(cfg config.Config) cmd.CancelCommand {
	return cmd.NewCancelCommand(cfg.Prompts.InProgressDir, libtime.NewCurrentDateTime())
}

// CreatePromptCompleteCommand creates a PromptCompleteCommand.
func CreatePromptCompleteCommand(cfg config.Config) cmd.PromptCompleteCommand {
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	promptManager, releaser := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		currentDateTimeGetter,
	)
	deps := createProviderDeps(cfg, currentDateTimeGetter)
	return cmd.NewPromptCompleteCommand(
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		promptManager,
		releaser,
		cfg.PR,
		deps.brancher,
		deps.prCreator,
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
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	counter := prompt.NewCounter(
		currentDateTimeGetter,
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)
	return cmd.NewSpecListCommand(
		spec.NewLister(
			currentDateTimeGetter,
			cfg.Specs.InboxDir,
			cfg.Specs.InProgressDir,
			cfg.Specs.CompletedDir,
		),
		counter,
	)
}

// CreateSpecStatusCommand creates a SpecStatusCommand.
func CreateSpecStatusCommand(cfg config.Config) cmd.SpecStatusCommand {
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	counter := prompt.NewCounter(
		currentDateTimeGetter,
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)
	return cmd.NewSpecStatusCommand(
		spec.NewLister(
			currentDateTimeGetter,
			cfg.Specs.InboxDir,
			cfg.Specs.InProgressDir,
			cfg.Specs.CompletedDir,
		),
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
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		currentDateTimeGetter,
	)

	statusChecker := status.NewChecker(
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.LogDir,
		lock.FilePath("."),
		cfg.ServerPort,
		promptManager,
	)
	formatter := status.NewFormatter()
	counter := prompt.NewCounter(
		currentDateTimeGetter,
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)

	return cmd.NewCombinedStatusCommand(
		statusChecker,
		formatter,
		spec.NewLister(
			currentDateTimeGetter,
			cfg.Specs.InboxDir,
			cfg.Specs.InProgressDir,
			cfg.Specs.CompletedDir,
		),
		counter,
	)
}

// CreateSpecShowCommand creates a SpecShowCommand.
func CreateSpecShowCommand(cfg config.Config) cmd.SpecShowCommand {
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	counter := prompt.NewCounter(
		currentDateTimeGetter,
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)
	return cmd.NewSpecShowCommand(
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.CompletedDir,
		counter,
		currentDateTimeGetter,
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
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	counter := prompt.NewCounter(
		currentDateTimeGetter,
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
	)
	return cmd.NewCombinedListCommand(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		spec.NewLister(
			currentDateTimeGetter,
			cfg.Specs.InboxDir,
			cfg.Specs.InProgressDir,
			cfg.Specs.CompletedDir,
		),
		counter,
		currentDateTimeGetter,
	)
}
