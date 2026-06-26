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
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
	liblog "github.com/bborbe/log"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/cancellationwatcher"
	"github.com/bborbe/dark-factory/pkg/cmd"
	"github.com/bborbe/dark-factory/pkg/cmd/healthcheck"
	"github.com/bborbe/dark-factory/pkg/committingrecoverer"
	"github.com/bborbe/dark-factory/pkg/completionreport"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/containerlock"
	"github.com/bborbe/dark-factory/pkg/containerslot"
	"github.com/bborbe/dark-factory/pkg/doctor"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/failurehandler"
	"github.com/bborbe/dark-factory/pkg/formatter"
	"github.com/bborbe/dark-factory/pkg/generator"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/gitprovider/bitbucket"
	"github.com/bborbe/dark-factory/pkg/globalconfig"
	"github.com/bborbe/dark-factory/pkg/healthcheckgate"
	"github.com/bborbe/dark-factory/pkg/launchpolicy"
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/preflight"
	"github.com/bborbe/dark-factory/pkg/preflightconditions"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/project"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/promptenricher"
	"github.com/bborbe/dark-factory/pkg/promptresumer"
	"github.com/bborbe/dark-factory/pkg/queuescanner"
	"github.com/bborbe/dark-factory/pkg/runner"
	"github.com/bborbe/dark-factory/pkg/scenario"
	"github.com/bborbe/dark-factory/pkg/server"
	"github.com/bborbe/dark-factory/pkg/slugmigrator"
	"github.com/bborbe/dark-factory/pkg/spec"
	"github.com/bborbe/dark-factory/pkg/specsweeper"
	"github.com/bborbe/dark-factory/pkg/specwatcher"
	"github.com/bborbe/dark-factory/pkg/status"
	"github.com/bborbe/dark-factory/pkg/subproc"
	"github.com/bborbe/dark-factory/pkg/validationprompt"
	"github.com/bborbe/dark-factory/pkg/version"
	"github.com/bborbe/dark-factory/pkg/watcher"
)

// lazyPromptProcessor forwards ProcessPrompt to a back-reference set after the processor is constructed.
// This breaks the circular wiring: scanner → processor.ProcessPrompt → scanner (via queueScanner field).
// The forwarder lives in factory (where wiring concerns belong) and is invisible to processor's public API.
type lazyPromptProcessor struct {
	inner queuescanner.PromptProcessor
}

func (l *lazyPromptProcessor) ProcessPrompt(ctx context.Context, pr prompt.Prompt) error {
	return l.inner.ProcessPrompt(ctx, pr)
}

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
// maxContainersSource is resolved from sources.MaxContainers when set (e.g. "arg", "project");
// otherwise falls back to inline detection using cfg.MaxContainers and globalFilePresent:
//   - "project" when cfg.MaxContainers > 0
//   - "global"  when cfg.MaxContainers <= 0 AND globalFilePresent is true
//   - "default" when cfg.MaxContainers <= 0 AND globalFilePresent is false
func LogEffectiveConfig(
	cfg config.Config,
	globalCfg globalconfig.GlobalConfig,
	globalFilePresent bool,
	sources config.FieldSources,
	projectEnv map[string]string,
) {
	effective := EffectiveMaxContainers(cfg.MaxContainers, globalCfg.MaxContainers)
	source := sources.MaxContainers
	if source == "" {
		if cfg.MaxContainers > 0 {
			source = "project"
		} else if globalFilePresent {
			source = "global"
		} else {
			source = "default"
		}
	}

	// Compute env key groupings — values are never logged.
	var fromGlobal, projectOverrides, projectOnly []string
	for k := range globalCfg.Env {
		if _, overridden := projectEnv[k]; overridden {
			projectOverrides = append(projectOverrides, k)
		} else {
			fromGlobal = append(fromGlobal, k)
		}
	}
	for k := range projectEnv {
		if _, inGlobal := globalCfg.Env[k]; !inGlobal {
			projectOnly = append(projectOnly, k)
		}
	}
	sort.Strings(fromGlobal)
	sort.Strings(projectOverrides)
	sort.Strings(projectOnly)

	slog.Info("effective config",
		"maxContainers", effective,
		"maxContainersSource", source,
		"containerImage", cfg.ContainerImage,
		"model", cfg.Model,
		"modelSource", sources.Model,
		"workflow", cfg.Workflow,
		"workflowSource", sources.Workflow,
		"pr", cfg.PR,
		"prSource", sources.PR,
		"autoRelease", cfg.AutoRelease,
		"autoReleaseSource", sources.AutoRelease,
		"autoMerge", cfg.AutoMerge,
		"autoMergeSource", sources.AutoMerge,
		"verificationGate", cfg.VerificationGate,
		"validationCommand", cfg.ValidationCommand,
		"testCommand", cfg.TestCommand,
		"debounceMs", cfg.DebounceMs,
		"hideGit", cfg.HideGit,
		"hideGitSource", sources.HideGit,
		"autoApprovePrompts", cfg.AutoApprovePrompts,
		"autoApprovePromptsSource", sources.AutoApprovePrompts,
		"autoGeneratePrompts", cfg.AutoGeneratePrompts,
		"autoGeneratePromptsSource", sources.AutoGeneratePrompts,
		"dirtyFileThreshold", cfg.DirtyFileThreshold,
		"dirtyFileThresholdSource", sources.DirtyFileThreshold,
		"promptsInboxDir", cfg.Prompts.InboxDir,
		"promptsInProgressDir", cfg.Prompts.InProgressDir,
		"promptsCompletedDir", cfg.Prompts.CompletedDir,
		"promptsLogDir", cfg.Prompts.LogDir,
		"preflightCommand", cfg.PreflightCommand,
		"preflightInterval", cfg.PreflightInterval,
		"healthcheckEnabled", cfg.HealthcheckEnabledValue(),
		"healthcheckEnabledSource", sources.HealthcheckEnabled,
		"healthcheckInterval", cfg.HealthcheckInterval,
		"healthcheckIntervalSource", sources.HealthcheckInterval,
		"envFromGlobal", fromGlobal,
		"envProjectOverrides", projectOverrides,
		"envProjectOnly", projectOnly,
	)
}

// createStartupLogger builds the closure passed to NewRunner / NewOneShotRunner that
// emits the effective-config log line immediately after the daemon lock is acquired.
// Errors from FileExists are swallowed so logging never blocks startup.
func createStartupLogger(
	ctx context.Context,
	cfg config.Config,
	globalCfg globalconfig.GlobalConfig,
	sources config.FieldSources,
	projectEnv map[string]string,
) func() {
	present, _ := globalconfig.FileExists(ctx)
	return func() { LogEffectiveConfig(cfg, globalCfg, present, sources, projectEnv) }
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
	cancelledDir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (*prompt.Manager, git.Releaser) {
	releaser := git.NewReleaser()
	promptManager := prompt.NewManager(
		inboxDir,
		inProgressDir,
		completedDir,
		cancelledDir,
		releaser,
		currentDateTimeGetter,
	)
	return promptManager, releaser
}

// providerDeps holds the provider-specific git operation implementations.
type providerDeps struct {
	prCreator git.PRCreator
	prMerger  git.PRMerger
	brancher  git.Brancher
}

// createProviderDeps returns the git provider implementations for GitHub.
func createProviderDeps(
	_ context.Context,
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) providerDeps {
	return CreateGitHubProviderDeps(cfg, currentDateTimeGetter)
}

// CreateGitHubProviderDeps returns GitHub gh-CLI-backed implementations.
func CreateGitHubProviderDeps(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) providerDeps {
	ghToken := cfg.ResolvedGitHubToken()
	return providerDeps{
		prCreator: git.NewPRCreator(ghToken),
		prMerger:  git.NewPRMerger(ghToken, currentDateTimeGetter),
		brancher:  git.NewBrancher(git.WithDefaultBranch(cfg.DefaultBranch)),
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

	coords, err := bitbucket.ParseRemoteFromGit(ctx, "origin")
	if err != nil {
		slog.Warn(
			"bitbucket: failed to parse git remote URL; PR operations will fail",
			"error",
			err,
		)
		coords = &bitbucket.RemoteCoords{Project: "", Repo: ""}
	}

	// Create lazy fetchers — no HTTP calls at construction time
	userFetcher := bitbucket.NewCurrentUserFetcher(baseURL, token)

	// Build collaborator fetcher (default reviewers plugin) with current user excluded lazily
	collaboratorFetcher := bitbucket.NewCollaboratorFetcher(
		baseURL,
		token,
		coords.Project,
		coords.Repo,
		cfg.DefaultBranch,
		userFetcher,
		nil,
	)

	return providerDeps{
		prCreator: bitbucket.NewPRCreator(
			baseURL, token, coords.Project, coords.Repo, cfg.DefaultBranch, collaboratorFetcher,
		),
		prMerger: bitbucket.NewPRMerger(
			baseURL,
			token,
			coords.Project,
			coords.Repo,
			currentDateTimeGetter,
		),
		brancher: git.NewBrancher(git.WithDefaultBranch(cfg.DefaultBranch)),
	}
}

// CreateBitbucketServerProviderDeps returns Bitbucket Server REST API-backed implementations.
func CreateBitbucketServerProviderDeps(
	ctx context.Context,
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) providerDeps {
	return createBitbucketProviderDeps(ctx, cfg, currentDateTimeGetter)
}

// createSpecSlugMigrator creates a Migrator that resolves bare spec number refs to full slugs.
func createSpecSlugMigrator(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) slugmigrator.Migrator {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)
	return slugmigrator.NewMigrator(
		[]string{cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir},
		promptManager,
	)
}

// CreateRunner creates a Runner that coordinates watcher and processor using the provided config.
//
//nolint:funlen // composition root: wires N subsystems; splitting into sub-helpers hides initialization order
func CreateRunner(
	ctx context.Context,
	cfg config.Config,
	ver string,
	skipPreflight bool,
	skipHealthcheck bool,
	sources config.FieldSources,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) runner.Runner {
	globalCfg, err := globalconfig.NewLoader().Load(ctx)
	if err != nil {
		return &errRunner{err: errors.Wrap(ctx, err, "globalconfig")}
	}
	// Merge global env with project env (project wins on collision).
	projectEnv := cfg.Env
	cfg.Env = config.MergeEnv(globalCfg.Env, projectEnv)
	inboxDir := cfg.Prompts.InboxDir
	inProgressDir := cfg.Prompts.InProgressDir
	completedDir := cfg.Prompts.CompletedDir
	promptManager, releaser := createPromptManager(
		inboxDir,
		inProgressDir,
		completedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)
	versionGetter := version.NewGetter(ver)
	projectName := project.Resolve(cfg.ResolvedProjectOverride())
	wakeup := make(chan struct{}, 10)
	migrator := createSpecSlugMigrator(cfg, currentDateTimeGetter)
	specGen := CreateSpecGenerator(
		cfg,
		cfg.ContainerImage,
		currentDateTimeGetter,
		migrator,
		promptManager,
	)
	deps := createProviderDeps(ctx, cfg, currentDateTimeGetter)

	n := CreateNotifier(
		CreateTelegramNotifier(cfg.ResolvedTelegramBotToken(), cfg.ResolvedTelegramChatID()),
		CreateDiscordNotifier(cfg.ResolvedDiscordWebhook()),
	)
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
			projectName,
		)
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

	// Create preflight checker from config. nil when preflightCommand is empty (disabled) or skipped.
	var preflightChecker preflight.Checker
	if cfg.PreflightCommand != "" && !skipPreflight {
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
				libtime.Duration(cfg.ParsedPreflightInterval()),
				projectRoot,
				n,
				projectName.String(),
				currentDateTimeGetter,
			)
		}
	}

	// Healthcheck startup gate (daemon-only). Reuses the same probe sequence as the
	// `dark-factory healthcheck` CLI. Disabled gates and --skip-healthcheck are handled
	// inside the gate; the factory always constructs it (zero branching).
	healthcheckGate := CreateHealthcheckGate(
		ctx, cfg, skipHealthcheck, projectName.String(), n, currentDateTimeGetter,
	)

	proc := CreateProcessor(
		ctx,
		buildProcessorConfig(cfg, globalCfg, inProgressDir, completedDir),
		projectName,
		promptManager,
		releaser,
		versionGetter,
		wakeup,
		deps.brancher,
		deps.prCreator,
		deps.prMerger,
		currentDateTimeGetter,
		n,
		createContainerCounter(),
		cl,
		containerChecker,
		dirtyFileChecker,
		gitLockChecker,
		preflightChecker,
		buildIdleLogger(
			cfg.ParsedIdleLogInterval(),
			cfg.ParsedQueueInterval(),
			func() { slog.Info("nothing to do, waiting for changes") },
		),
	)
	watcher := CreateWatcher(inProgressDir, inboxDir, promptManager, wakeup,
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
		promptManager, CreateLocker("."), watcher, proc, srv,
		specWatcher, projectName,
		containerChecker, n, migrator,
		currentDateTimeGetter,
		cfg.ParsedMaxPromptDuration(),
		executor.NewDockerContainerStopper(),
		createStartupLogger(ctx, cfg, globalCfg, sources, projectEnv),
		cfg.HideGit,
		preflightChecker,
		logWriter,
		healthcheckGate,
	)
}

// CreateOneShotRunner creates an OneShotRunner that drains the queue and exits.
//
//nolint:funlen // composition root: wires N subsystems; splitting into sub-helpers hides initialization order
func CreateOneShotRunner(
	ctx context.Context,
	cfg config.Config,
	ver string,
	autoApprove bool,
	skipPreflight bool,
	sources config.FieldSources,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) runner.OneShotRunner {
	globalCfg, err := globalconfig.NewLoader().Load(ctx)
	if err != nil {
		return &errOneShotRunner{err: errors.Wrap(ctx, err, "globalconfig")}
	}
	// Merge global env with project env (project wins on collision).
	projectEnv := cfg.Env
	cfg.Env = config.MergeEnv(globalCfg.Env, projectEnv)
	inboxDir := cfg.Prompts.InboxDir
	inProgressDir := cfg.Prompts.InProgressDir
	completedDir := cfg.Prompts.CompletedDir
	promptManager, releaser := createPromptManager(
		inboxDir, inProgressDir, completedDir, cfg.Prompts.CancelledDir, currentDateTimeGetter)
	versionGetter, n := version.NewGetter(ver), CreateNotifier(
		CreateTelegramNotifier(cfg.ResolvedTelegramBotToken(), cfg.ResolvedTelegramChatID()),
		CreateDiscordNotifier(cfg.ResolvedDiscordWebhook()),
	)
	projectName := project.Resolve(cfg.ResolvedProjectOverride())
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

	// Create preflight checker from config. nil when preflightCommand is empty (disabled) or skipped.
	var osPreflightChecker preflight.Checker
	if cfg.PreflightCommand != "" && !skipPreflight {
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
				libtime.Duration(cfg.ParsedPreflightInterval()),
				projectRoot,
				n,
				projectName.String(),
				currentDateTimeGetter,
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
			ctx,
			buildProcessorConfig(cfg, globalCfg, inProgressDir, completedDir),
			projectName,
			promptManager,
			releaser,
			versionGetter,
			make(chan struct{}, 10),
			deps.brancher,
			deps.prCreator,
			deps.prMerger,
			currentDateTimeGetter,
			n,
			createContainerCounter(),
			cl,
			containerChecker,
			osDirtyFileChecker,
			osGitLockChecker,
			osPreflightChecker,
			func(_ context.Context, cancel context.CancelFunc) {
				slog.Info("queue idle, exiting one-shot mode")
				cancel()
			},
		),
		CreateSpecGenerator(
			cfg,
			cfg.ContainerImage,
			currentDateTimeGetter,
			migrator,
			promptManager,
		),
		currentDateTimeGetter,
		containerChecker,
		autoApprove,
		migrator,
		cfg.HideGit,
		createStartupLogger(ctx, cfg, globalCfg, sources, projectEnv),
	)
}

// resolveHideGit computes the effective hideGit value every dark-factory
// container receives — spec-generator, prompt-executor, and healthcheck probes.
// Worktree mode always masks .git (the worktree's .git is a pointer file the
// container can't follow); explicit hideGit also masks it. All three call
// sites must agree so the containers see the same workspace shape — the
// architectural invariant of spec 098.
func resolveHideGit(cfg config.Config) bool {
	return cfg.Workflow == config.WorkflowWorktree || cfg.HideGit
}

// CreateSpecGenerator creates a SpecGenerator using the Docker executor.
func CreateSpecGenerator(
	cfg config.Config,
	containerImage string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	slugMigrator slugmigrator.Migrator,
	promptManager *prompt.Manager,
) generator.SpecGenerator {
	projectRoot, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	specGenPolicy := launchpolicy.NewPolicy(
		containerImage,
		project.Resolve(cfg.ResolvedProjectOverride()).String(),
		projectRoot,
		cfg.ResolvedClaudeDir(),
		home,
		cfg.Env,
		cfg.ExtraMounts,
		cfg.NetrcFile,
		cfg.GitconfigFile,
		resolveHideGit(cfg),
	)
	return generator.NewSpecGenerator(
		executor.NewDockerExecutor(
			specGenPolicy,
			cfg.Model,
			cfg.ParsedMaxPromptDuration(),
			currentDateTimeGetter,
			formatter.NewFormatter(currentDateTimeGetter),
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
		promptManager,
		cfg.AutoApprovePrompts,
		cfg.Prompts.InProgressDir,
		project.Resolve(cfg.ResolvedProjectOverride()),
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
		cfg.AutoGeneratePrompts,
	)
}

// CreateWatcher creates a Watcher that normalizes filenames on file events.
func CreateWatcher(
	inProgressDir string,
	inboxDir string,
	promptManager *prompt.Manager,
	wakeup chan<- struct{},
	debounce time.Duration,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) watcher.Watcher {
	return watcher.NewWatcher(
		inProgressDir,
		inboxDir,
		promptManager,
		wakeup,
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
	projectName project.Name,
) status.Checker {
	projectDir, _ := os.Getwd()
	globalCfg, err := globalconfig.NewLoader().Load(ctx)
	if err != nil {
		slog.Warn("globalconfig load failed for status checker, using default", "error", err)
		globalCfg = globalconfig.GlobalConfig{MaxContainers: globalconfig.DefaultMaxContainers}
	}
	return status.NewChecker(
		projectName,
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
	projectName project.Name,
	n notifier.Notifier,
	pm *prompt.Manager,
) spec.AutoCompleter {
	return spec.NewAutoCompleter(
		inProgressDir, completedDir,
		specsInboxDir, specsInProgressDir, specsCompletedDir,
		currentDateTimeGetter, projectName.String(), n, pm,
	)
}

// CreateWorkflowExecutor creates a WorkflowExecutorProvider that dispenses the appropriate
// WorkflowExecutor for a given workflow.
func CreateWorkflowExecutor(
	pr bool,
	brancher git.Brancher,
	prCreator git.PRCreator,
	prMerger git.PRMerger,
	autoMerge bool,
	autoRelease bool,
	projectName project.Name,
	promptManager *prompt.Manager,
	releaser git.Releaser,
	autoCompleter spec.AutoCompleter,
	promptDirPrefixes []string,
	fileMover prompt.FileMover,
) processor.WorkflowExecutorProvider {
	deps := processor.WorkflowDeps{
		ProjectName:        projectName,
		PromptManager:      promptManager,
		AutoCompleter:      autoCompleter,
		Releaser:           releaser,
		FileMover:          fileMover,
		Brancher:           brancher,
		PRCreator:          prCreator,
		Cloner:             git.NewCloner(),
		Worktreer:          git.NewWorktreer(),
		PRMerger:           prMerger,
		PR:                 pr,
		AutoMerge:          autoMerge,
		AutoRelease:        autoRelease,
		IgnorePathPrefixes: promptDirPrefixes,
	}
	return processor.NewWorkflowExecutorProviderMap(map[config.Workflow]processor.WorkflowExecutor{
		config.WorkflowClone:    processor.NewCloneWorkflowExecutor(deps),
		config.WorkflowWorktree: processor.NewWorktreeWorkflowExecutor(deps),
		config.WorkflowBranch:   processor.NewBranchWorkflowExecutor(deps),
		config.WorkflowDirect:   processor.NewDirectWorkflowExecutor(deps),
	})
}

// buildProcessorConfig assembles a ProcessorConfig from the dark-factory cfg,
// the global config, and the resolved in-progress / completed dirs. Both
// CreateRunner and CreateOneShotRunner call this helper so the two paths
// stay positionally identical — silent drift was the bug class this refactor
// closed.
//
//nolint:funlen // single struct literal naming every config-derived field
func buildProcessorConfig(
	cfg config.Config,
	globalCfg globalconfig.GlobalConfig,
	inProgressDir, completedDir string,
) ProcessorConfig {
	return ProcessorConfig{
		InProgressDir:      inProgressDir,
		CompletedDir:       completedDir,
		LogDir:             cfg.Prompts.LogDir,
		SpecsInboxDir:      cfg.Specs.InboxDir,
		SpecsInProgressDir: cfg.Specs.InProgressDir,
		SpecsCompletedDir:  cfg.Specs.CompletedDir,
		SpecsRejectedDir:   cfg.Specs.RejectedDir,
		PromptDirPrefixes: []string{
			cfg.Prompts.InboxDir,
			cfg.Prompts.InProgressDir,
			cfg.Prompts.CompletedDir,
			cfg.Prompts.LogDir,
		},
		ContainerImage:         cfg.ContainerImage,
		Model:                  cfg.Model,
		ClaudeDir:              cfg.ResolvedClaudeDir(),
		Env:                    cfg.Env,
		ExtraMounts:            cfg.ExtraMounts,
		HideGit:                cfg.HideGit,
		NetrcFile:              cfg.NetrcFile,
		GitconfigFile:          cfg.GitconfigFile,
		Workflow:               cfg.Workflow,
		PR:                     cfg.PR,
		AutoMerge:              cfg.AutoMerge,
		AutoRelease:            cfg.AutoRelease,
		VerificationGate:       cfg.VerificationGate,
		ValidationCommand:      cfg.ValidationCommand,
		ValidationPrompt:       cfg.ValidationPrompt,
		TestCommand:            cfg.TestCommand,
		AdditionalInstructions: cfg.AdditionalInstructions,
		MaxContainers:          EffectiveMaxContainers(cfg.MaxContainers, globalCfg.MaxContainers),
		MaxPromptDuration:      cfg.ParsedMaxPromptDuration(),
		DirtyFileThreshold:     cfg.DirtyFileThreshold,
		AutoRetryLimit:         cfg.AutoRetryLimit,
		QueueInterval:          cfg.ParsedQueueInterval(),
		SweepInterval:          cfg.ParsedSweepInterval(),
	}
}

// ProcessorConfig groups the config-derived inputs to CreateProcessor.
// Previously these 25 fields were positional params of CreateProcessor,
// duplicated across CreateRunner and CreateOneShotRunner — silent drift
// between the two call sites was a real bug class. Grouping them into a
// struct makes the call sites identical and adding a new field a one-line
// change. Infra deps (PromptManager, Releaser, Brancher, etc.) remain
// separate args so wiring stays visible at the call site.
type ProcessorConfig struct {
	// Directories
	InProgressDir      string
	CompletedDir       string
	LogDir             string
	SpecsInboxDir      string
	SpecsInProgressDir string
	SpecsCompletedDir  string
	SpecsRejectedDir   string
	PromptDirPrefixes  []string

	// Container
	ContainerImage string
	Model          string
	ClaudeDir      string
	Env            map[string]string
	ExtraMounts    []config.ExtraMount
	HideGit        bool

	// Git / VCS
	NetrcFile     string
	GitconfigFile string

	// Workflow
	Workflow         config.Workflow
	PR               bool
	AutoMerge        bool
	AutoRelease      bool
	VerificationGate bool

	// Validation
	ValidationCommand      string
	ValidationPrompt       string
	TestCommand            string
	AdditionalInstructions string

	// Resource limits
	MaxContainers      int
	MaxPromptDuration  time.Duration
	DirtyFileThreshold int
	AutoRetryLimit     int

	// Timing
	QueueInterval time.Duration
	SweepInterval time.Duration
}

// CreateProcessor creates a Processor that executes queued prompts.
//
//nolint:funlen // composition root: wires N subsystems; splitting into sub-helpers hides initialization order
func CreateProcessor(
	ctx context.Context,
	cfg ProcessorConfig,
	projectName project.Name,
	promptManager *prompt.Manager,
	releaser git.Releaser,
	versionGetter version.Getter,
	wakeup <-chan struct{},
	brancher git.Brancher,
	prCreator git.PRCreator,
	prMerger git.PRMerger,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	n notifier.Notifier,
	containerCounter executor.ContainerCounter,
	containerLock containerlock.ContainerLock,
	containerChecker executor.ContainerChecker,
	dirtyFileChecker processor.DirtyFileChecker,
	gitLockChecker processor.GitLockChecker,
	preflightChecker preflight.Checker,
	onIdle processor.NothingToDoCallback,
) processor.Processor {
	dirs := processor.Dirs{
		Queue:     cfg.InProgressDir,
		Completed: cfg.CompletedDir,
		Log:       cfg.LogDir,
	}
	autoCompleter := createAutoCompleter(
		cfg.InProgressDir, cfg.CompletedDir,
		cfg.SpecsInboxDir, cfg.SpecsInProgressDir, cfg.SpecsCompletedDir,
		currentDateTimeGetter, projectName, n, promptManager,
	)
	workflowExecutorProvider := CreateWorkflowExecutor(
		cfg.PR, brancher, prCreator, prMerger,
		cfg.AutoMerge, cfg.AutoRelease,
		projectName, promptManager, releaser, autoCompleter,
		cfg.PromptDirPrefixes, releaser,
	)
	workflowExecutor := workflowExecutorProvider.Get(ctx, cfg.Workflow)
	projectRoot, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	processorPolicy := launchpolicy.NewPolicy(
		cfg.ContainerImage,
		projectName.String(),
		projectRoot,
		cfg.ClaudeDir,
		home,
		cfg.Env,
		cfg.ExtraMounts,
		cfg.NetrcFile,
		cfg.GitconfigFile,
		cfg.Workflow == config.WorkflowWorktree || cfg.HideGit,
	)
	exec := executor.NewDockerExecutor(
		processorPolicy,
		cfg.Model,
		cfg.MaxPromptDuration,
		currentDateTimeGetter,
		formatter.NewFormatter(currentDateTimeGetter),
	)
	fh := failurehandler.NewHandler(
		promptManager,
		n,
		dirs.Completed,
		projectName,
		int(cfg.AutoRetryLimit),
	)
	resumer := promptresumer.NewResumer(
		promptManager,
		exec,
		workflowExecutor,
		completionreport.NewValidator(),
		fh,
		dirs.Queue,
		dirs.Completed,
		dirs.Log,
		projectName,
		cfg.MaxPromptDuration,
	)
	// Two-phase wiring: scanner → proc.ProcessPrompt → scanner.
	// The lazyPromptProcessor closes the loop inside factory where wiring belongs.
	ppForwarder := &lazyPromptProcessor{}
	scanner := queuescanner.NewScanner(
		promptManager,
		ppForwarder,
		fh,
		dirs.Queue,
		lock.NewDirLock,
		0,
	)
	proc := processor.NewProcessor(
		exec,
		promptManager,
		releaser,
		versionGetter,
		workflowExecutor,
		autoCompleter,
		specsweeper.NewSweeper(
			spec.NewLister(
				currentDateTimeGetter,
				cfg.SpecsInboxDir,
				cfg.SpecsInProgressDir,
				cfg.SpecsCompletedDir,
				cfg.SpecsRejectedDir,
			),
			autoCompleter,
		),
		preflightconditions.NewConditions(
			preflightChecker,
			gitLockChecker,
			dirtyFileChecker,
			cfg.DirtyFileThreshold,
		),
		containerslot.NewManager(
			containerLock,
			containerCounter,
			containerChecker,
			cfg.MaxContainers,
			10*time.Second,
		),
		cancellationwatcher.NewWatcher(exec, promptManager),
		wakeup,
		dirs,
		projectName,
		fh,
		resumer,
		cfg.Workflow,
		cfg.VerificationGate,
		completionreport.NewValidator(),
		promptenricher.NewEnricher(
			releaser,
			cfg.AdditionalInstructions,
			cfg.TestCommand,
			cfg.ValidationCommand,
			cfg.ValidationPrompt,
			validationprompt.NewResolver(),
			cfg.Workflow == config.WorkflowWorktree || cfg.HideGit,
		),
		committingrecoverer.NewRecoverer(
			promptManager,
			releaser,
			autoCompleter,
			dirs.Completed,
			cfg.AutoRelease,
		),
		scanner,
		cfg.QueueInterval,
		cfg.SweepInterval,
		onIdle,
	)
	ppForwarder.inner = proc
	return proc
}

// CreateNotifier creates a Notifier from config, or a no-op if no channels are configured.
// CreateTelegramNotifier creates a Telegram notifier with the given token and chatID.
func CreateTelegramNotifier(token, chatID string) notifier.Notifier {
	if token == "" || chatID == "" {
		return notifier.NewMultiNotifier()
	}
	return notifier.NewTelegramNotifier(token, chatID)
}

// CreateDiscordNotifier creates a Discord notifier with the given webhook.
func CreateDiscordNotifier(webhook string) notifier.Notifier {
	if webhook == "" {
		return notifier.NewMultiNotifier()
	}
	return notifier.NewDiscordNotifier(webhook)
}

// CreateNotifier creates a Notifier based on the provided Telegram and Discord notifiers.
func CreateNotifier(telegram, discord notifier.Notifier) notifier.Notifier {
	return notifier.NewMultiNotifier(telegram, discord)
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
	projectName project.Name,
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
		projectName,
	)

	// Build the mux with all routes
	mux := http.NewServeMux()
	mux.Handle("/health", libhttp.NewErrorHandler(server.NewHealthHandler()))
	mux.Handle("/api/v1/status", libhttp.NewErrorHandler(server.NewStatusHandler(statusChecker)))
	mux.Handle("/api/v1/queue", libhttp.NewErrorHandler(server.NewQueueHandler(statusChecker)))
	// Both routes share a single handler instance. The handler inspects the URL path
	// suffix to distinguish single-file (/api/v1/queue/action) from all-files (/api/v1/queue/action/all) operations.
	queueActionHandler := libhttp.NewErrorHandler(
		server.NewQueueActionHandler(inboxDir, inProgressDir, promptManager),
	)
	mux.Handle("/api/v1/queue/action", queueActionHandler)
	mux.Handle("/api/v1/queue/action/all", queueActionHandler)
	mux.Handle("/api/v1/inbox", libhttp.NewErrorHandler(server.NewInboxHandler(inboxDir)))
	mux.Handle(
		"/api/v1/completed",
		libhttp.NewErrorHandler(server.NewCompletedHandler(statusChecker)),
	)

	// Create server with libhttp (sane defaults: ReadHeaderTimeout 10s, ReadTimeout 30s,
	// WriteTimeout 30s, IdleTimeout 60s, MaxHeaderBytes 1MB — sufficient for dark-factory threat model)
	runFunc := libhttp.NewServer(addr, mux)
	return server.NewServer(runFunc)
}

// CreateStatusCommand creates a StatusCommand.
func CreateStatusCommand(
	ctx context.Context,
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.StatusCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
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
		project.Resolve(cfg.ResolvedProjectOverride()),
	)
	formatter := status.NewFormatter()

	return cmd.NewStatusCommand(statusChecker, formatter)
}

// CreateDoctorCommand creates a DoctorCommand with all required dependencies.
func CreateDoctorCommand(
	ctx context.Context,
	cfg config.Config,
	verifyingStaleHours int,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.DoctorCommand {
	promptManager, releaser := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)

	specLister := spec.NewLister(
		currentDateTimeGetter,
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.CompletedDir,
		cfg.Specs.RejectedDir,
	)

	autoCompleter := spec.NewAutoCompleter(
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.CompletedDir,
		currentDateTimeGetter,
		cfg.ProjectName,
		notifier.NewMultiNotifier(),
		promptManager,
	)

	deps := doctor.Deps{
		SpecsInboxDir:         cfg.Specs.InboxDir,
		SpecsInProgressDir:    cfg.Specs.InProgressDir,
		SpecsCompletedDir:     cfg.Specs.CompletedDir,
		SpecsRejectedDir:      cfg.Specs.RejectedDir,
		PromptsInboxDir:       cfg.Prompts.InboxDir,
		PromptsInProgressDir:  cfg.Prompts.InProgressDir,
		PromptsCompletedDir:   cfg.Prompts.CompletedDir,
		PromptsCancelledDir:   cfg.Prompts.CancelledDir,
		SpecLister:            specLister,
		PromptManager:         promptManager,
		CurrentDateTimeGetter: currentDateTimeGetter,
		VerifyingStaleHours:   verifyingStaleHours,
	}

	checker := doctor.NewChecker(deps)

	fixer := doctor.NewFixer(doctor.FixerDeps{
		Deps:            deps,
		AutoCompleter:   autoCompleter,
		Mover:           releaser,
		FileLockFactory: lock.NewDirLock,
	})

	return cmd.NewDoctorCommand(checker, fixer)
}

// CreateHealthcheckCommand creates a HealthcheckCommand with the seven
// probes wired in fixed order: docker, image, boot, claude, mount, gh,
// notifications. The gh probe is appended only when cfg.PR is true; the
// notifications probe is appended only when at least one notification
// channel is configured. The factory is construction-only — instantiate
// concrete deps, pass them in, no branches.
// CreateHealthcheckGate builds the daemon-startup healthcheck gate. The gate's
// disabled/skip/cache logic lives in healthcheckgate.gate.Check; this factory only
// constructs collaborators (the underlying HealthcheckCommand, the file cache, the
// cache key, the notifier) and passes them in.
//
// os.UserHomeDir error: failure is logged, then the gate falls back to a CWD-relative
// cache path. The cache is non-secret, non-critical, and write failures are tolerated
// by design; surfacing the error here is enough — refusing to start the daemon over a
// cache-dir resolution miss would be worse than the silent fallback.
func CreateHealthcheckGate(
	ctx context.Context,
	cfg config.Config,
	skipHealthcheck bool,
	projectName string,
	n notifier.Notifier,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) healthcheckgate.Gate {
	cacheKey := healthcheckgate.CacheKey(
		cfg.ContainerImage,
		projectName,
		cfg.ParsedHealthcheckInterval(),
	)
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn(
			"healthcheck cache: os.UserHomeDir failed; cache root will be CWD-relative",
			"error",
			err,
		)
	}
	cacheRoot := filepath.Join(home, ".dark-factory", "healthcheck-cache")
	return healthcheckgate.NewGate(
		cfg.HealthcheckEnabledValue(),
		skipHealthcheck,
		cfg.ParsedHealthcheckInterval(),
		CreateHealthcheckCommand(ctx, cfg, currentDateTimeGetter),
		cacheKey,
		healthcheckgate.NewFileCache(cacheRoot),
		n,
		projectName,
		currentDateTimeGetter,
	)
}

func CreateHealthcheckCommand(
	ctx context.Context,
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.HealthcheckCommand {
	subprocRunner := subproc.NewRunner()
	claudeRunner := subproc.NewRunnerWithThresholds(
		healthcheck.ClaudeWarnAfterForFactory(),
		healthcheck.ClaudeTimeoutForFactory(),
	)
	ghRunner := subproc.NewRunnerWithThresholds(
		healthcheck.GhWarnAfterForFactory(),
		healthcheck.GhTimeoutForFactory(),
	)
	projectName := project.Resolve(cfg.ResolvedProjectOverride()).String()
	projectRoot, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	env := map[string]string{}
	for k, v := range cfg.Env {
		env[k] = v
	}
	if cfg.ContainerImage != "" {
		// ANTHROPIC_MODEL is set by the production executor; mirror it here so
		// the claude probe sees the same env as a real prompt container.
		if cfg.Model != "" {
			env["ANTHROPIC_MODEL"] = cfg.Model
		}
	}
	policy := launchpolicy.NewPolicy(
		cfg.ContainerImage,
		projectName,
		projectRoot,
		cfg.ResolvedClaudeDir(),
		home,
		env,
		cfg.ExtraMounts,
		cfg.NetrcFile,
		cfg.GitconfigFile,
		resolveHideGit(cfg),
	)
	probes := cmd.Probes{
		healthcheck.NewDockerProbe(subprocRunner),
		healthcheck.NewImageProbe(cfg.ContainerImage, subprocRunner),
		healthcheck.NewBootProbe(policy, subprocRunner),
		healthcheck.NewClaudeProbe(policy, claudeRunner),
		healthcheck.NewMountProbe(policy, subprocRunner),
	}
	if cfg.PR {
		probes = append(probes, healthcheck.NewGhProbe(ghRunner))
	}
	if healthcheck.NotificationsConfigured(cfg) {
		probes = append(
			probes,
			healthcheck.NewNotificationsProbe(cfg, &http.Client{Timeout: 5 * time.Second}),
		)
	}
	return cmd.NewHealthcheckCommand(probes)
}

// CreateListCommand creates a ListCommand.
func CreateListCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.ListCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)
	return cmd.NewListCommand(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.RejectedDir,
		promptManager,
	)
}

// CreateRequeueCommand creates a RequeueCommand.
func CreateRequeueCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.RequeueCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)
	return cmd.NewRequeueCommand(cfg.Prompts.InProgressDir, promptManager)
}

// CreateCancelCommand creates a CancelCommand.
func CreateCancelCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.CancelCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)
	return cmd.NewCancelCommand(cfg.Prompts.InProgressDir, cfg.Prompts.CancelledDir, promptManager)
}

// CreatePromptCompleteCommand creates a PromptCompleteCommand.
func CreatePromptCompleteCommand(
	ctx context.Context,
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	forceRelease bool,
) cmd.PromptCompleteCommand {
	promptManager, releaser := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
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
		cfg.AutoRelease,
		forceRelease,
	)
}

// CreateUnapproveCommand creates an UnapproveCommand.
func CreateUnapproveCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.UnapproveCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)

	return cmd.NewUnapproveCommand(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		promptManager,
	)
}

// CreateApproveCommand creates an ApproveCommand.
func CreateApproveCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.ApproveCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)

	return cmd.NewApproveCommand(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		promptManager,
	)
}

// CreateSpecListCommand creates a SpecListCommand.
func CreateSpecListCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.SpecListCommand {
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
			cfg.Specs.RejectedDir,
		),
		counter,
	)
}

// CreateSpecStatusCommand creates a SpecStatusCommand.
func CreateSpecStatusCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.SpecStatusCommand {
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
			cfg.Specs.RejectedDir,
		),
		counter,
	)
}

// CreateSpecApproveCommand creates a SpecApproveCommand.
func CreateSpecApproveCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.SpecApproveCommand {
	return cmd.NewSpecApproveCommand(
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.CompletedDir,
		currentDateTimeGetter,
		lock.NewDirLock,
		0,
	)
}

// CreateSpecUnapproveCommand creates a SpecUnapproveCommand.
func CreateSpecUnapproveCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.SpecUnapproveCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)
	return cmd.NewSpecUnapproveCommand(
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		promptManager,
		currentDateTimeGetter,
		lock.NewDirLock,
		0,
	)
}

// CreateRejectCommand creates a RejectCommand.
func CreateRejectCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.RejectCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)
	return cmd.NewRejectCommand(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.RejectedDir,
		promptManager,
		lock.NewDirLock,
		0,
	)
}

// CreateSpecRejectCommand creates a SpecRejectCommand.
func CreateSpecRejectCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.SpecRejectCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)
	return cmd.NewSpecRejectCommand(
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.RejectedDir,
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.RejectedDir,
		promptManager,
		currentDateTimeGetter,
		lock.NewDirLock,
		0,
	)
}

// CreateSpecCompleteCommand creates a SpecCompleteCommand.
func CreateSpecCompleteCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.SpecCompleteCommand {
	return cmd.NewSpecCompleteCommand(
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.CompletedDir,
		currentDateTimeGetter,
		lock.NewDirLock,
		0,
	)
}

// CreateSpecMarkPromptedCommand creates a SpecMarkPromptedCommand.
func CreateSpecMarkPromptedCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.SpecMarkPromptedCommand {
	return cmd.NewSpecMarkPromptedCommand(
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.CompletedDir,
		currentDateTimeGetter,
		lock.NewDirLock,
		0,
	)
}

// CreateCombinedStatusCommand creates a CombinedStatusCommand.
func CreateCombinedStatusCommand(
	ctx context.Context,
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.CombinedStatusCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
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
		project.Resolve(cfg.ResolvedProjectOverride()),
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
			cfg.Specs.RejectedDir,
		),
		counter,
	)
}

// CreateSpecShowCommand creates a SpecShowCommand.
func CreateSpecShowCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.SpecShowCommand {
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

const scenariosDir = "scenarios"

// CreateScenarioListCommand creates a ScenarioListCommand.
func CreateScenarioListCommand(_ config.Config) cmd.ScenarioListCommand {
	return cmd.NewScenarioListCommand(scenario.NewLister(scenariosDir))
}

// CreateScenarioShowCommand creates a ScenarioShowCommand.
func CreateScenarioShowCommand(_ config.Config) cmd.ScenarioShowCommand {
	return cmd.NewScenarioShowCommand(scenario.NewLister(scenariosDir))
}

// CreateScenarioStatusCommand creates a ScenarioStatusCommand.
func CreateScenarioStatusCommand(_ config.Config) cmd.ScenarioStatusCommand {
	return cmd.NewScenarioStatusCommand(scenario.NewLister(scenariosDir))
}

// CreatePromptShowCommand creates a PromptShowCommand.
func CreatePromptShowCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.PromptShowCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)
	return cmd.NewPromptShowCommand(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.LogDir,
		promptManager,
	)
}

// CreateCombinedListCommand creates a CombinedListCommand.
func CreateCombinedListCommand(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.CombinedListCommand {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)
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
		cfg.Prompts.RejectedDir,
		spec.NewLister(
			currentDateTimeGetter,
			cfg.Specs.InboxDir,
			cfg.Specs.InProgressDir,
			cfg.Specs.CompletedDir,
			cfg.Specs.RejectedDir,
		),
		counter,
		promptManager,
	)
}

// buildIdleLogger returns the onIdle callback used by daemon-mode.
// Exposed for testing the burst-collapse and heartbeat behavior.
//
// Behavior:
//   - First call (or first call > 2*queueInterval since the last) emits unconditionally
//   - Subsequent calls within the same idle window emit only when the heartbeat sampler fires
//   - idleLogInterval == 0 disables the heartbeat; only first-entry emissions fire
func buildIdleLogger(
	idleLogInterval time.Duration,
	queueInterval time.Duration,
	emit func(),
) func(context.Context, context.CancelFunc) {
	var (
		mu           sync.Mutex
		lastIdleCall time.Time
		heartbeat    liblog.Sampler
	)
	newHeartbeat := func() liblog.Sampler {
		s := liblog.NewSampleTime(idleLogInterval)
		s.IsSample() // prime: consume the initial fire so the interval starts from now
		return s
	}
	if idleLogInterval > 0 {
		heartbeat = newHeartbeat()
	}
	return func(_ context.Context, _ context.CancelFunc) {
		mu.Lock()
		defer mu.Unlock()
		now := time.Now()
		isNewIdleEntry := lastIdleCall.IsZero() || now.Sub(lastIdleCall) > 2*queueInterval
		lastIdleCall = now
		if isNewIdleEntry {
			emit()
			if idleLogInterval > 0 {
				heartbeat = newHeartbeat()
			}
			return
		}
		if heartbeat != nil && heartbeat.IsSample() {
			emit()
		}
	}
}
