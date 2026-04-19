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
	"os"
	"time"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/cmd"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/containerlock"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/formatter"
	"github.com/bborbe/dark-factory/pkg/generator"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/globalconfig"
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/preflight"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/project"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/review"
	"github.com/bborbe/dark-factory/pkg/runner"
	"github.com/bborbe/dark-factory/pkg/server"
	"github.com/bborbe/dark-factory/pkg/slugmigrator"
	"github.com/bborbe/dark-factory/pkg/spec"
	"github.com/bborbe/dark-factory/pkg/specwatcher"
	"github.com/bborbe/dark-factory/pkg/status"
	"github.com/bborbe/dark-factory/pkg/subproc"
	"github.com/bborbe/dark-factory/pkg/version"
	"github.com/bborbe/dark-factory/pkg/watcher"
)

// EffectiveMaxContainers returns the per-project limit when set (> 0),
// otherwise falls back to the global limit.
// A per-project value MAY exceed the global limit. This is intentional:
// e.g. dark-factory itself uses maxContainers: 5 so it can self-update
// even when 3 other containers are already running.
func EffectiveMaxContainers(projectMax, globalMax int) int {
	if projectMax > 0 {
		return projectMax
	}
	return globalMax
}

// LogEffectiveConfig emits a single slog.Info "effective config" line describing
// the resolved settings that drive daemon/run behavior. This is purely diagnostic;
// no value is mutated.
//
// maxContainersSource is one of:
//   - "project" when cfg.MaxContainers > 0 (project file override wins)
//   - "global"  when cfg.MaxContainers <= 0 AND globalFilePresent is true
//   - "default" when cfg.MaxContainers <= 0 AND globalFilePresent is false
func LogEffectiveConfig(
	cfg config.Config,
	globalCfg globalconfig.GlobalConfig,
	globalFilePresent bool,
) {
	effective := EffectiveMaxContainers(cfg.MaxContainers, globalCfg.MaxContainers)
	source := "default"
	if cfg.MaxContainers > 0 {
		source = "project"
	} else if globalFilePresent {
		source = "global"
	}
	slog.Info("effective config",
		"maxContainers", effective,
		"maxContainersSource", source,
		"containerImage", cfg.ContainerImage,
		"model", cfg.Model,
		"workflow", cfg.Workflow,
		"pr", cfg.PR,
		"autoRelease", cfg.AutoRelease,
		"autoMerge", cfg.AutoMerge,
		"verificationGate", cfg.VerificationGate,
		"validationCommand", cfg.ValidationCommand,
		"testCommand", cfg.TestCommand,
		"debounceMs", cfg.DebounceMs,
		"hideGit", cfg.HideGit,
		"promptsInboxDir", cfg.Prompts.InboxDir,
		"promptsInProgressDir", cfg.Prompts.InProgressDir,
		"promptsCompletedDir", cfg.Prompts.CompletedDir,
		"promptsLogDir", cfg.Prompts.LogDir,
		"preflightCommand", cfg.PreflightCommand,
		"preflightInterval", cfg.PreflightInterval,
	)
}

// createStartupLogger builds the closure passed to NewRunner / NewOneShotRunner that
// emits the effective-config log line immediately after the daemon lock is acquired.
// Errors from FileExists are swallowed so logging never blocks startup.
func createStartupLogger(
	ctx context.Context,
	cfg config.Config,
	globalCfg globalconfig.GlobalConfig,
) func() {
	present, _ := globalconfig.FileExists(ctx)
	return func() { LogEffectiveConfig(cfg, globalCfg, present) }
}

// errRunner is a Runner that immediately returns an error when Run is called.
type errRunner struct{ err error }

func (e *errRunner) Run(_ context.Context) error { return e.err }

// errOneShotRunner is an OneShotRunner that immediately returns an error when Run is called.
type errOneShotRunner struct{ err error }

func (e *errOneShotRunner) Run(_ context.Context) error { return e.err }

// createPromptManager creates shared *prompt.Manager and git.Releaser dependencies.
func createPromptManager(
	inboxDir string,
	inProgressDir string,
	completedDir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (*prompt.Manager, git.Releaser) {
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
	ctx context.Context,
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) providerDeps {
	if cfg.Provider == config.ProviderBitbucketServer {
		return createBitbucketProviderDeps(ctx, cfg, currentDateTimeGetter)
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
	ctx context.Context,
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) providerDeps {
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

	// Create lazy fetchers — no HTTP calls at construction time
	userFetcher := git.NewBitbucketCurrentUserFetcher(baseURL, token)

	// Build collaborator fetcher (default reviewers plugin) with current user excluded lazily
	collaboratorFetcher := git.NewBitbucketCollaboratorFetcher(
		baseURL,
		token,
		coords.Project,
		coords.Repo,
		cfg.DefaultBranch,
		userFetcher,
		cfg.AllowedReviewers,
	)

	return providerDeps{
		prCreator: git.NewBitbucketPRCreator(
			baseURL, token, coords.Project, coords.Repo, cfg.DefaultBranch, collaboratorFetcher,
		),
		prMerger: git.NewBitbucketPRMerger(
			baseURL,
			token,
			coords.Project,
			coords.Repo,
			currentDateTimeGetter,
		),
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

// createSpecSlugMigrator creates a Migrator that resolves bare spec number refs to full slugs.
func createSpecSlugMigrator(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) slugmigrator.Migrator {
	return slugmigrator.NewMigrator(
		[]string{cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir},
		currentDateTimeGetter,
	)
}

// CreateRunner creates a Runner that coordinates watcher and processor using the provided config.
//
//nolint:funlen // composition root: wires N subsystems; splitting into sub-helpers hides initialization order
func CreateRunner(ctx context.Context, cfg config.Config, ver string) runner.Runner {
	globalCfg, err := globalconfig.NewLoader().Load(ctx)
	if err != nil {
		return &errRunner{err: errors.Wrap(ctx, err, "globalconfig")}
	}
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
	migrator := createSpecSlugMigrator(cfg, currentDateTimeGetter)
	specGen := CreateSpecGenerator(cfg, cfg.ContainerImage, currentDateTimeGetter, migrator)
	deps := createProviderDeps(ctx, cfg, currentDateTimeGetter)

	n := CreateNotifier(cfg)
	var srv server.Server
	if cfg.ServerPort > 0 {
		srv = CreateServer(
			ctx,
			cfg.ServerPort,
			inboxDir,
			inProgressDir,
			completedDir,
			cfg.Prompts.LogDir,
			promptManager,
			currentDateTimeGetter,
			cfg.MaxContainers,
		)
	}

	var poller review.ReviewPoller
	if cfg.AutoReview {
		poller = CreateReviewPoller(ctx, cfg, promptManager, projectName, n)
	}

	cl, containerChecker, clErr := createContainerDeps(ctx, currentDateTimeGetter)
	if clErr != nil {
		return &errRunner{err: errors.Wrap(ctx, clErr, "containerlock")}
	}

	var dirtyFileChecker processor.DirtyFileChecker
	var gitLockChecker processor.GitLockChecker
	if !cfg.HideGit {
		dirtyFileChecker = processor.NewDirtyFileChecker(".")
		gitLockChecker = processor.NewGitLockChecker(".")
	}

	// Create preflight checker from config. nil when preflightCommand is empty (disabled).
	var preflightChecker preflight.Checker
	if cfg.PreflightCommand != "" {
		projectRoot, rootErr := os.Getwd()
		if rootErr != nil {
			slog.Warn(
				"preflight: could not determine project root, preflight disabled",
				"error",
				rootErr,
			)
		} else {
			preflightChecker = preflight.NewChecker(
				cfg.PreflightCommand,
				cfg.ParsedPreflightInterval(),
				projectRoot,
				cfg.ContainerImage,
				cfg.ExtraMounts,
				n,
				projectName,
			)
		}
	}

	proc := CreateProcessor(
		inProgressDir, completedDir, cfg.Prompts.LogDir, projectName,
		promptManager, releaser, versionGetter, ready,
		cfg.ContainerImage, cfg.Model, cfg.NetrcFile, cfg.GitconfigFile,
		cfg.Workflow, cfg.PR,
		deps.brancher, deps.prCreator, deps.prMerger,
		cfg.AutoMerge, cfg.AutoRelease, cfg.AutoReview,
		cfg.ValidationCommand, cfg.ValidationPrompt, cfg.TestCommand,
		cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir,
		cfg.VerificationGate, cfg.Env, cfg.ExtraMounts, currentDateTimeGetter, n,
		cfg.ResolvedClaudeDir(),
		createContainerCounter(),
		EffectiveMaxContainers(cfg.MaxContainers, globalCfg.MaxContainers),
		cfg.AdditionalInstructions,
		cl,
		containerChecker,
		cfg.DirtyFileThreshold, dirtyFileChecker, gitLockChecker,
		cfg.ParsedMaxPromptDuration(), cfg.AutoRetryLimit,
		cfg.HideGit,
		preflightChecker,
	)
	watcher := CreateWatcher(inProgressDir, inboxDir, promptManager, ready,
		time.Duration(cfg.DebounceMs)*time.Millisecond, currentDateTimeGetter)
	specWatcher := CreateSpecWatcher(cfg, specGen, currentDateTimeGetter)
	var logWriter io.Writer
	if logFile, err := os.Create(".dark-factory.log"); err != nil {
		slog.Warn("failed to create daemon log file, continuing without", "error", err)
	} else {
		logWriter = logFile
	}
	return runner.NewRunner(
		inboxDir, inProgressDir, completedDir, cfg.Prompts.LogDir,
		cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir, cfg.Specs.LogDir,
		promptManager, CreateLocker("."), watcher, proc, srv, poller,
		specWatcher, projectName,
		containerChecker, n, migrator,
		currentDateTimeGetter,
		releaser,
		cfg.ParsedMaxPromptDuration(),
		executor.NewDockerContainerStopper(),
		createStartupLogger(ctx, cfg, globalCfg),
		cfg.HideGit,
		logWriter,
	)
}

// CreateOneShotRunner creates an OneShotRunner that drains the queue and exits.
//
//nolint:funlen // composition root: wires N subsystems; splitting into sub-helpers hides initialization order
func CreateOneShotRunner(
	ctx context.Context, cfg config.Config, ver string, autoApprove bool,
) runner.OneShotRunner {
	globalCfg, err := globalconfig.NewLoader().Load(ctx)
	if err != nil {
		return &errOneShotRunner{err: errors.Wrap(ctx, err, "globalconfig")}
	}
	inboxDir := cfg.Prompts.InboxDir
	inProgressDir := cfg.Prompts.InProgressDir
	completedDir := cfg.Prompts.CompletedDir
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	promptManager, releaser := createPromptManager(
		inboxDir, inProgressDir, completedDir, currentDateTimeGetter)
	versionGetter, n := version.NewGetter(ver), CreateNotifier(cfg)
	projectName := project.Name(cfg.ProjectName)
	deps := createProviderDeps(ctx, cfg, currentDateTimeGetter)
	migrator := createSpecSlugMigrator(cfg, currentDateTimeGetter)
	cl, containerChecker, clErr := createContainerDeps(ctx, currentDateTimeGetter)
	if clErr != nil {
		return &errOneShotRunner{err: errors.Wrap(ctx, clErr, "containerlock")}
	}
	var osDirtyFileChecker processor.DirtyFileChecker
	var osGitLockChecker processor.GitLockChecker
	if !cfg.HideGit {
		osDirtyFileChecker = processor.NewDirtyFileChecker(".")
		osGitLockChecker = processor.NewGitLockChecker(".")
	}

	// Create preflight checker from config. nil when preflightCommand is empty (disabled).
	var osPreflightChecker preflight.Checker
	if cfg.PreflightCommand != "" {
		projectRoot, rootErr := os.Getwd()
		if rootErr != nil {
			slog.Warn(
				"preflight: could not determine project root, preflight disabled",
				"error",
				rootErr,
			)
		} else {
			osPreflightChecker = preflight.NewChecker(
				cfg.PreflightCommand,
				cfg.ParsedPreflightInterval(),
				projectRoot,
				cfg.ContainerImage,
				cfg.ExtraMounts,
				n,
				projectName,
			)
		}
	}

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
			make(chan struct{}, 10), // ProcessQueue never reads from it in one-shot mode
			cfg.ContainerImage,
			cfg.Model,
			cfg.NetrcFile,
			cfg.GitconfigFile,
			cfg.Workflow,
			cfg.PR,
			deps.brancher,
			deps.prCreator,
			deps.prMerger,
			cfg.AutoMerge,
			cfg.AutoRelease,
			cfg.AutoReview,
			cfg.ValidationCommand,
			cfg.ValidationPrompt,
			cfg.TestCommand,
			cfg.Specs.InboxDir,
			cfg.Specs.InProgressDir,
			cfg.Specs.CompletedDir,
			cfg.VerificationGate,
			cfg.Env,
			cfg.ExtraMounts,
			currentDateTimeGetter,
			n,
			cfg.ResolvedClaudeDir(),
			createContainerCounter(),
			EffectiveMaxContainers(cfg.MaxContainers, globalCfg.MaxContainers),
			cfg.AdditionalInstructions,
			cl,
			containerChecker,
			cfg.DirtyFileThreshold, osDirtyFileChecker,
			osGitLockChecker, cfg.ParsedMaxPromptDuration(), cfg.AutoRetryLimit,
			cfg.HideGit,
			osPreflightChecker,
		),
		CreateSpecGenerator(cfg, cfg.ContainerImage, currentDateTimeGetter, migrator),
		currentDateTimeGetter,
		containerChecker,
		autoApprove,
		migrator,
		releaser,
		createStartupLogger(ctx, cfg, globalCfg),
	)
}

// CreateSpecGenerator creates a SpecGenerator using the Docker executor.
func CreateSpecGenerator(
	cfg config.Config,
	containerImage string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	slugMigrator slugmigrator.Migrator,
) generator.SpecGenerator {
	return generator.NewSpecGenerator(
		executor.NewDockerExecutor(
			containerImage,
			project.Name(cfg.ProjectName),
			cfg.Model,
			cfg.NetrcFile,
			cfg.GitconfigFile,
			cfg.Env,
			cfg.ExtraMounts,
			cfg.ResolvedClaudeDir(),
			cfg.ParsedMaxPromptDuration(),
			currentDateTimeGetter,
			formatter.NewFormatter(),
			false, // hideGit — spec generators never need .git masking
		),
		executor.NewDockerContainerChecker(currentDateTimeGetter),
		cfg.Prompts.InboxDir,
		cfg.Prompts.CompletedDir,
		cfg.Specs.InboxDir,
		cfg.Specs.LogDir,
		currentDateTimeGetter,
		slugMigrator,
		cfg.GenerateCommand,
		cfg.AdditionalInstructions,
		cfg.ParsedMaxPromptDuration(),
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
	promptManager *prompt.Manager,
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

// createContainerCounter returns a ContainerCounter backed by docker ps.
func createContainerCounter() executor.ContainerCounter {
	return executor.NewDockerContainerCounter(subproc.NewRunner())
}

// createStatusChecker loads global config and constructs a status.Checker.
// projectMax is the project-level MaxContainers value (may be 0); effective max is resolved against global config.
func createStatusChecker(
	ctx context.Context,
	inProgressDir, completedDir, logDir string,
	serverPort int,
	promptManager *prompt.Manager,
	projectMax int,
	dirtyFileThreshold int,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) status.Checker {
	projectDir, _ := os.Getwd()
	globalCfg, err := globalconfig.NewLoader().Load(ctx)
	if err != nil {
		slog.Warn("globalconfig load failed for status checker, using default", "error", err)
		globalCfg = globalconfig.GlobalConfig{MaxContainers: globalconfig.DefaultMaxContainers}
	}
	return status.NewChecker(
		projectDir,
		inProgressDir,
		completedDir,
		logDir,
		lock.FilePath("."),
		serverPort,
		promptManager,
		createContainerCounter(),
		EffectiveMaxContainers(projectMax, globalCfg.MaxContainers),
		dirtyFileThreshold,
		currentDateTimeGetter,
		subproc.NewRunner(),
	)
}

// createContainerDeps creates the container lock and checker used for the count-and-start window.
func createContainerDeps(
	ctx context.Context,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (containerlock.ContainerLock, executor.ContainerChecker, error) {
	cl, err := containerlock.NewContainerLock(ctx)
	if err != nil {
		return nil, nil, err
	}
	return cl, executor.NewDockerContainerChecker(currentDateTimeGetter), nil
}

// createAutoCompleter creates a spec.AutoCompleter with the given parameters.
func createAutoCompleter(
	inProgressDir, completedDir string,
	specsInboxDir, specsInProgressDir, specsCompletedDir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	projectName string,
	n notifier.Notifier,
) spec.AutoCompleter {
	return spec.NewAutoCompleter(
		inProgressDir, completedDir,
		specsInboxDir, specsInProgressDir, specsCompletedDir,
		currentDateTimeGetter, projectName, n,
	)
}

// CreateWorkflowExecutor creates the appropriate WorkflowExecutor based on the config.
func CreateWorkflowExecutor(
	workflow config.Workflow,
	pr bool,
	brancher git.Brancher,
	prCreator git.PRCreator,
	prMerger git.PRMerger,
	autoMerge bool,
	autoRelease bool,
	autoReview bool,
	projectName string,
	promptManager *prompt.Manager,
	releaser git.Releaser,
	autoCompleter spec.AutoCompleter,
) processor.WorkflowExecutor {
	deps := processor.WorkflowDeps{
		ProjectName:   projectName,
		PromptManager: promptManager,
		AutoCompleter: autoCompleter,
		Releaser:      releaser,
		Brancher:      brancher,
		PRCreator:     prCreator,
		Cloner:        git.NewCloner(),
		Worktreer:     git.NewWorktreer(),
		PRMerger:      prMerger,
		PR:            pr,
		AutoMerge:     autoMerge,
		AutoReview:    autoReview,
		AutoRelease:   autoRelease,
	}
	switch workflow {
	case config.WorkflowClone:
		return processor.NewCloneWorkflowExecutor(deps)
	case config.WorkflowWorktree:
		return processor.NewWorktreeWorkflowExecutor(deps)
	case config.WorkflowBranch:
		return processor.NewBranchWorkflowExecutor(deps)
	default:
		return processor.NewDirectWorkflowExecutor(deps)
	}
}

// CreateProcessor creates a Processor that executes queued prompts.
//
//nolint:funlen // composition root: wires N subsystems; splitting into sub-helpers hides initialization order
func CreateProcessor(
	inProgressDir string,
	completedDir string,
	logDir string,
	projectName string,
	promptManager *prompt.Manager,
	releaser git.Releaser,
	versionGetter version.Getter,
	ready <-chan struct{},
	containerImage string,
	model string,
	netrcFile string,
	gitconfigFile string,
	workflow config.Workflow,
	pr bool,
	brancher git.Brancher,
	prCreator git.PRCreator,
	prMerger git.PRMerger,
	autoMerge bool,
	autoRelease bool,
	autoReview bool,
	validationCommand string,
	validationPrompt string,
	testCommand string,
	specsInboxDir, specsInProgressDir, specsCompletedDir string,
	verificationGate bool,
	env map[string]string,
	extraMounts []config.ExtraMount,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	n notifier.Notifier,
	claudeDir string,
	containerCounter executor.ContainerCounter,
	maxContainers int,
	additionalInstructions string,
	containerLock containerlock.ContainerLock,
	containerChecker executor.ContainerChecker,
	dirtyFileThreshold int, dirtyFileChecker processor.DirtyFileChecker,
	gitLockChecker processor.GitLockChecker, maxPromptDuration time.Duration, autoRetryLimit int,
	hideGit bool,
	preflightChecker preflight.Checker,
) processor.Processor {
	autoCompleter := createAutoCompleter(
		inProgressDir, completedDir,
		specsInboxDir, specsInProgressDir, specsCompletedDir,
		currentDateTimeGetter, projectName, n,
	)
	workflowExecutor := CreateWorkflowExecutor(
		workflow, pr, brancher, prCreator, prMerger,
		autoMerge, autoRelease, autoReview,
		projectName, promptManager, releaser, autoCompleter,
	)
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
			extraMounts,
			claudeDir,
			maxPromptDuration,
			currentDateTimeGetter,
			formatter.NewFormatter(),
			workflow == config.WorkflowWorktree || hideGit,
		),
		promptManager,
		releaser,
		versionGetter,
		ready,
		workflowExecutor,
		autoCompleter,
		spec.NewLister(currentDateTimeGetter, specsInboxDir, specsInProgressDir, specsCompletedDir),
		validationCommand,
		validationPrompt,
		testCommand,
		verificationGate,
		n,
		containerCounter, maxContainers, additionalInstructions,
		containerLock, containerChecker,
		dirtyFileThreshold, dirtyFileChecker, gitLockChecker,
		autoRetryLimit, maxPromptDuration,
		preflightChecker,
	)
}

// CreateReviewPoller creates a ReviewPoller that watches in_review prompts and handles approvals/changes.
func CreateReviewPoller(
	ctx context.Context,
	cfg config.Config,
	promptManager *prompt.Manager,
	projectName string,
	n notifier.Notifier,
) review.ReviewPoller {
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	deps := createProviderDeps(ctx, cfg, currentDateTimeGetter)

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

// CreateKillCommand creates a KillCommand that stops the running daemon.
func CreateKillCommand(_ config.Config) cmd.KillCommand {
	return cmd.NewKillCommand(lock.FilePath("."), nil, nil)
}

// CreateLocker creates a Locker for the specified directory.
func CreateLocker(dir string) lock.Locker {
	return lock.NewLocker(dir)
}

// CreateServer creates a Server that provides HTTP endpoints for monitoring.
func CreateServer(
	ctx context.Context,
	port int,
	inboxDir string,
	inProgressDir string,
	completedDir string,
	logDir string,
	promptManager *prompt.Manager,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	projectMaxContainers int,
) server.Server {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	statusChecker := createStatusChecker(
		ctx,
		inProgressDir,
		completedDir,
		logDir,
		port,
		promptManager,
		projectMaxContainers,
		0, // CreateServer has no dirty-file threshold — server is status-only
		currentDateTimeGetter,
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
func CreateStatusCommand(ctx context.Context, cfg config.Config) cmd.StatusCommand {
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		currentDateTimeGetter,
	)

	statusChecker := createStatusChecker(
		ctx,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.LogDir,
		cfg.ServerPort,
		promptManager,
		cfg.MaxContainers,
		cfg.DirtyFileThreshold,
		currentDateTimeGetter,
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
func CreatePromptCompleteCommand(ctx context.Context, cfg config.Config) cmd.PromptCompleteCommand {
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	promptManager, releaser := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		currentDateTimeGetter,
	)
	deps := createProviderDeps(ctx, cfg, currentDateTimeGetter)
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

// CreateUnapproveCommand creates an UnapproveCommand.
func CreateUnapproveCommand(cfg config.Config) cmd.UnapproveCommand {
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		currentDateTimeGetter,
	)

	return cmd.NewUnapproveCommand(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		promptManager,
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

// CreateSpecUnapproveCommand creates a SpecUnapproveCommand.
func CreateSpecUnapproveCommand(cfg config.Config) cmd.SpecUnapproveCommand {
	return cmd.NewSpecUnapproveCommand(
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
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
func CreateCombinedStatusCommand(ctx context.Context, cfg config.Config) cmd.CombinedStatusCommand {
	currentDateTimeGetter := libtime.NewCurrentDateTime()
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		currentDateTimeGetter,
	)

	statusChecker := createStatusChecker(
		ctx,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.LogDir,
		cfg.ServerPort,
		promptManager,
		cfg.MaxContainers,
		cfg.DirtyFileThreshold,
		currentDateTimeGetter,
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
