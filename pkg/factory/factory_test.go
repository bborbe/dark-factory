// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/factory"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/globalconfig"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/preflightconditions"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/project"
	"github.com/bborbe/dark-factory/pkg/promptenricher"
	"github.com/bborbe/dark-factory/pkg/subproc"
)

var _ = Describe("Factory", func() {
	var cfg config.Config

	BeforeEach(func() {
		cfg = config.Defaults()
	})

	Describe("CreateRunner", func() {
		It("should return a non-nil runner", func() {
			runner := factory.CreateRunner(
				context.Background(),
				cfg,
				"v0.0.1",
				false,
				false,
				config.FieldSources{},
				libtime.NewCurrentDateTime(),
			)
			Expect(runner).NotTo(BeNil())
		})
	})

	Describe("CreateWatcher", func() {
		It("should return a non-nil watcher", func() {
			wakeup := make(chan struct{}, 10)
			watcher := factory.CreateWatcher(
				cfg.Prompts.InProgressDir,
				cfg.Prompts.InboxDir,
				nil, // promptManager not needed for nil check
				wakeup,
				100,
				libtime.NewCurrentDateTime(),
			)
			Expect(watcher).NotTo(BeNil())
		})
	})

	Describe("CreateProcessor", func() {
		It("should return a non-nil processor", func() {
			wakeup := make(chan struct{}, 10)
			pcfg := factory.ProcessorConfig{
				InProgressDir:      cfg.Prompts.InProgressDir,
				CompletedDir:       cfg.Prompts.CompletedDir,
				LogDir:             cfg.Prompts.LogDir,
				SpecsInboxDir:      "specs/inbox",
				SpecsInProgressDir: "specs/in-progress",
				SpecsCompletedDir:  "specs/completed",
				SpecsRejectedDir:   "specs/rejected",
				ContainerImage:     cfg.ContainerImage,
				Model:              cfg.Model,
				ClaudeDir:          cfg.ResolvedClaudeDir(),
				NetrcFile:          cfg.NetrcFile,
				GitconfigFile:      cfg.GitconfigFile,
				Workflow:           cfg.Workflow,
				PR:                 cfg.PR,
				TestCommand:        "make precommit",
			}
			processor := factory.CreateProcessor(
				context.Background(),
				pcfg,
				project.Name("test-project"),
				nil, // promptManager not needed for nil check
				nil, // releaser not needed for nil check
				nil, // versionGetter not needed for nil check
				wakeup,
				git.NewBrancher(),
				git.NewPRCreator(""),
				git.NewPRMerger("", libtime.NewCurrentDateTime()),
				libtime.NewCurrentDateTime(),
				notifier.NewMultiNotifier(),
				executor.NewDockerContainerCounter(subproc.NewRunner()),
				nil, // containerLock
				nil, // containerChecker
				processor.NewDirtyFileChecker("."),
				processor.NewGitLockChecker("."),
				nil, // preflightChecker
				nil, // onIdle
			)
			Expect(processor).NotTo(BeNil())
		})
	})

	Describe("CreateLocker", func() {
		It("should return a non-nil locker", func() {
			locker := factory.CreateLocker(".")
			Expect(locker).NotTo(BeNil())
		})
	})

	Describe("CreateServer", func() {
		It("should return a non-nil server", func() {
			server := factory.CreateServer(
				context.Background(),
				8080,
				cfg.Prompts.InboxDir,
				cfg.Prompts.InProgressDir,
				cfg.Prompts.CompletedDir,
				cfg.Prompts.LogDir,
				nil, // promptManager not needed for nil check
				libtime.NewCurrentDateTime(),
				0,
				project.Name("test-project"),
			)
			Expect(server).NotTo(BeNil())
		})
	})

	Describe("CreateStatusCommand", func() {
		It("should return a non-nil status command", func() {
			cmd := factory.CreateStatusCommand(
				context.Background(),
				cfg,
				libtime.NewCurrentDateTime(),
			)
			Expect(cmd).NotTo(BeNil())
		})
	})

	Describe("CreateCombinedStatusCommand", func() {
		It("should return a non-nil combined status command", func() {
			cmd := factory.CreateCombinedStatusCommand(
				context.Background(),
				cfg,
				libtime.NewCurrentDateTime(),
			)
			Expect(cmd).NotTo(BeNil())
		})
	})

	Describe("CreateCombinedListCommand", func() {
		It("should return a non-nil combined list command", func() {
			cmd := factory.CreateCombinedListCommand(cfg, libtime.NewCurrentDateTime())
			Expect(cmd).NotTo(BeNil())
		})
	})

	Describe("preflight subproc thresholds", func() {
		// Regression lock for the spec-100 (Centralize Subprocess Runner)
		// fallout: before spec 100, preflight ran via raw exec.CommandContext
		// with no internal timeout, and `make precommit` took as long as it
		// needed (30–180s for bborbe Go repos). After spec 100, the call site
		// switched to subprocRunner.RunWithWarnAndTimeoutDir which carries
		// subproc.DefaultTimeout (10s). Every non-trivial preflight failed
		// silently after that — operators worked around with
		// preflightCommand: "true" until the regression was diagnosed.
		// These tests pin the dedicated-runner thresholds so a future
		// subproc.DefaultTimeout change can't silently re-tighten preflight.

		It("warns after 30s (much later than the general default of 3s)", func() {
			Expect(factory.PreflightWarnAfterForTest).To(Equal(30 * time.Second))
		})

		It("times out at 30 minutes (vs subproc.DefaultTimeout=10s)", func() {
			Expect(factory.PreflightTimeoutForTest).To(Equal(30 * time.Minute))
		})

		It("preflight timeout is strictly longer than subproc.DefaultTimeout", func() {
			Expect(factory.PreflightTimeoutForTest).To(BeNumerically(">", subproc.DefaultTimeout))
		})
	})

	Describe("EffectiveMaxContainers", func() {
		It("returns global when project is zero", func() {
			Expect(factory.EffectiveMaxContainers(0, 3)).To(Equal(3))
		})

		It("returns project when project is set", func() {
			Expect(factory.EffectiveMaxContainers(5, 3)).To(Equal(5))
		})

		It("returns project when project is 1", func() {
			Expect(factory.EffectiveMaxContainers(1, 3)).To(Equal(1))
		})
	})

	Describe("LogEffectiveConfig", func() {
		var (
			logBuf      bytes.Buffer
			origDefault *slog.Logger
		)

		BeforeEach(func() {
			logBuf.Reset()
			origDefault = slog.Default()
			slog.SetDefault(
				slog.New(
					slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}),
				),
			)
		})

		AfterEach(func() {
			slog.SetDefault(origDefault)
		})

		fullTestConfig := func() config.Config {
			return config.Config{
				ContainerImage:    "ghcr.io/bborbe/yolo:test",
				Model:             "claude-sonnet-4-6",
				Worktree:          true,
				PR:                true,
				AutoRelease:       true,
				AutoMerge:         true,
				VerificationGate:  true,
				ValidationCommand: "make precommit",
				TestCommand:       "make test",
				DebounceMs:        500,
				Prompts: config.PromptsConfig{
					InboxDir:      "p",
					InProgressDir: "p/ip",
					CompletedDir:  "p/c",
					LogDir:        "p/l",
				},
			}
		}

		assertRequiredFields := func(output string) {
			Expect(strings.Count(output, `msg="effective config"`)).To(Equal(1))
			Expect(output).To(ContainSubstring("containerImage="))
			Expect(output).To(ContainSubstring("model="))
			Expect(output).To(ContainSubstring("workflow="))
			Expect(output).To(ContainSubstring("pr="))
			Expect(output).To(ContainSubstring("autoRelease="))
			Expect(output).To(ContainSubstring("autoMerge="))
			Expect(output).To(ContainSubstring("verificationGate="))
			Expect(output).To(ContainSubstring("validationCommand="))
			Expect(output).To(ContainSubstring("testCommand="))
			Expect(output).To(ContainSubstring("debounceMs="))
			Expect(output).To(ContainSubstring("promptsInboxDir="))
			Expect(output).To(ContainSubstring("promptsInProgressDir="))
			Expect(output).To(ContainSubstring("promptsCompletedDir="))
			Expect(output).To(ContainSubstring("promptsLogDir="))
			Expect(output).To(ContainSubstring("modelSource="))
			Expect(output).To(ContainSubstring("hideGitSource="))
			Expect(output).To(ContainSubstring("autoReleaseSource="))
			Expect(output).To(ContainSubstring("dirtyFileThresholdSource="))
		}

		assertNoSecrets := func(output string) {
			Expect(output).NotTo(ContainSubstring("env="))
			Expect(output).NotTo(ContainSubstring("extraMounts="))
			Expect(output).NotTo(ContainSubstring("github="))
			Expect(output).NotTo(ContainSubstring("netrcFile="))
			Expect(output).NotTo(ContainSubstring("gitconfigFile="))
			Expect(output).NotTo(ContainSubstring("notifications="))
			Expect(output).NotTo(ContainSubstring("bitbucket="))
		}

		DescribeTable("maxContainers source and value",
			func(cfgMaxContainers int, globalMaxContainers int, globalFilePresent bool,
				expectedMax int, expectedSource string,
			) {
				c := fullTestConfig()
				c.MaxContainers = cfgMaxContainers
				globalCfg := globalconfig.GlobalConfig{MaxContainers: globalMaxContainers}

				factory.LogEffectiveConfig(
					c,
					globalCfg,
					globalFilePresent,
					config.FieldSources{},
					nil,
				)

				output := logBuf.String()
				assertRequiredFields(output)
				assertNoSecrets(output)
				Expect(output).To(ContainSubstring(fmt.Sprintf("maxContainers=%d", expectedMax)))
				Expect(output).To(ContainSubstring("maxContainersSource=" + expectedSource))
			},
			Entry("defaults-only (no project, no global file)",
				0, globalconfig.DefaultMaxContainers, false,
				globalconfig.DefaultMaxContainers, "default",
			),
			Entry("global-only override",
				0, 7, true,
				7, "global",
			),
			Entry("project override beats global",
				5, 3, true,
				5, "project",
			),
			Entry("project override when global is default",
				5, globalconfig.DefaultMaxContainers, false,
				5, "project",
			),
			Entry("project override equals global (still project)",
				3, 3, true,
				3, "project",
			),
		)

		It("does not log secrets or env maps", func() {
			c := fullTestConfig()
			c.Env = map[string]string{"SECRET": "token"}
			c.NetrcFile = "/home/user/.netrc"
			c.GitconfigFile = "/home/user/.gitconfig"
			globalCfg := globalconfig.GlobalConfig{MaxContainers: globalconfig.DefaultMaxContainers}

			factory.LogEffectiveConfig(c, globalCfg, false, config.FieldSources{}, nil)

			output := logBuf.String()
			assertNoSecrets(output)
		})

		It("emits exactly one log line", func() {
			c := fullTestConfig()
			globalCfg := globalconfig.GlobalConfig{MaxContainers: globalconfig.DefaultMaxContainers}

			factory.LogEffectiveConfig(c, globalCfg, false, config.FieldSources{}, nil)

			output := logBuf.String()
			Expect(strings.Count(output, `msg="effective config"`)).To(Equal(1))
		})

		It("emits autoApprovePrompts and autoApprovePromptsSource", func() {
			c := fullTestConfig()
			c.AutoApprovePrompts = true
			globalCfg := globalconfig.GlobalConfig{MaxContainers: globalconfig.DefaultMaxContainers}
			sources := config.FieldSources{}
			sources.AutoApprovePrompts = "project"

			factory.LogEffectiveConfig(c, globalCfg, false, sources, nil)

			output := logBuf.String()
			Expect(output).To(ContainSubstring("autoApprovePrompts=true"))
			Expect(output).To(ContainSubstring("autoApprovePromptsSource=project"))
		})

		It("reports env keys by source group and never logs values", func() {
			c := fullTestConfig()
			c.Env = map[string]string{
				"GLOBAL_ONLY":  "gv",
				"SHARED":       "project-wins",
				"PROJECT_ONLY": "pv",
			}
			globalCfg := globalconfig.GlobalConfig{
				MaxContainers: globalconfig.DefaultMaxContainers,
				Env: map[string]string{
					"GLOBAL_ONLY": "gv",
					"SHARED":      "global-val",
				},
			}
			projectEnv := map[string]string{
				"SHARED":       "project-wins",
				"PROJECT_ONLY": "pv",
			}

			factory.LogEffectiveConfig(c, globalCfg, false, config.FieldSources{}, projectEnv)

			output := logBuf.String()
			// Each key appears specifically in its expected source group
			Expect(output).To(MatchRegexp(`envFromGlobal=\[[^]]*GLOBAL_ONLY`))
			Expect(output).To(MatchRegexp(`envProjectOverrides=\[[^]]*SHARED`))
			Expect(output).To(MatchRegexp(`envProjectOnly=\[[^]]*PROJECT_ONLY`))
			// Values must NOT appear anywhere
			Expect(output).NotTo(ContainSubstring("gv"))
			Expect(output).NotTo(ContainSubstring("project-wins"))
			Expect(output).NotTo(ContainSubstring("global-val"))
			Expect(output).NotTo(ContainSubstring("pv"))
		})
	})

	Describe("hideGit fragment wiring", func() {
		DescribeTable(
			"Config.EffectiveHideGit",
			// Phase-2 cleanup of [[Harden Dark Factory Architecture]]: the
			// helper used to live in factory.go as `resolveHideGit` and was
			// applied at 1 of 3 sites; the other 2 inlined the formula and
			// silently drifted. Promoted to a Config method matching the
			// established EffectiveMaxContainers / HealthcheckEnabledValue
			// pattern. The `scripts/hotpath-hidegit-check.sh` precommit
			// gate rejects the inline formula in pkg/.
			func(cfg config.Config, want bool) {
				Expect(cfg.EffectiveHideGit()).To(Equal(want))
			},
			Entry("default config -> false", config.Config{}, false),
			Entry("HideGit=true -> true", config.Config{HideGit: true}, true),
			Entry(
				"Workflow=worktree -> true",
				config.Config{Workflow: config.WorkflowWorktree},
				true,
			),
			Entry(
				"both set -> true",
				config.Config{HideGit: true, Workflow: config.WorkflowWorktree},
				true,
			),
		)

		DescribeTable(
			"createProviderDeps dispatches on cfg.Provider",
			// Regression lock for the 2026-06-27 audit finding: the
			// function ignored cfg.Provider entirely and always returned
			// GitHub deps, even when validation had accepted
			// `provider: bitbucket-server`. Operators on a Bitbucket Server
			// config saw no startup error — failures surfaced only at the
			// first PR-create attempt as an opaque gh-CLI error.
			func(provider config.Provider, want string) {
				cfg := config.Defaults()
				cfg.Provider = provider
				cfg.Bitbucket.BaseURL = "https://bitbucket.example.com"
				ctx := context.Background()
				getter := libtime.NewCurrentDateTime()
				Expect(factory.ProviderDepsBackendForTest(ctx, cfg, getter)).To(Equal(want))
			},
			Entry("github → github backend", config.ProviderGitHub, "github"),
			Entry(
				"bitbucket-server → bitbucket backend",
				config.ProviderBitbucketServer,
				"bitbucket",
			),
			Entry("empty provider → github (default)", config.Provider(""), "github"),
		)

		It("enricher emits hideGit guidance fragment when hideGit=true", func() {
			releaser := &mocks.Releaser{}
			releaser.CommitWithRetryStub = func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }
			releaser.HasChangelogReturns(false)
			resolverMock := &mocks.Resolver{}
			resolverMock.ResolveReturns("", false, nil)

			enricher := promptenricher.NewEnricher(
				releaser,
				"",
				"",
				"",
				"",
				resolverMock,
				true,
			)
			result := enricher.Enrich(context.Background(), "PROMPT_BODY")
			Expect(result).To(ContainSubstring("character device"))
			Expect(result).To(ContainSubstring("hideGit=true active"))
			Expect(result).NotTo(ContainSubstring("hideGit=true active\n\nPROMPT_BODY"))
		})

		It("enricher does not emit hideGit guidance fragment when hideGit=false", func() {
			releaser := &mocks.Releaser{}
			releaser.CommitWithRetryStub = func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }
			releaser.HasChangelogReturns(false)
			resolverMock := &mocks.Resolver{}
			resolverMock.ResolveReturns("", false, nil)

			enricher := promptenricher.NewEnricher(
				releaser,
				"",
				"",
				"",
				"",
				resolverMock,
				false,
			)
			result := enricher.Enrich(context.Background(), "PROMPT_BODY")
			Expect(result).NotTo(ContainSubstring("hideGit=true active"))
			Expect(result).To(HavePrefix("PROMPT_BODY"))
		})
	})

	Describe("preflight failure terminates runners", func() {
		var (
			tempDir string
			origDir string
		)

		BeforeEach(func() {
			var err error
			origDir, err = os.Getwd()
			Expect(err).NotTo(HaveOccurred())

			tempDir, err = os.MkdirTemp("", "factory-preflight-test-*")
			Expect(err).NotTo(HaveOccurred())

			for _, dir := range []string{
				"prompts/inbox", "prompts/in-progress", "prompts/completed",
				"prompts/log", "prompts/rejected",
				"specs/inbox", "specs/in-progress", "specs/completed",
				"specs/log", "specs/rejected",
			} {
				Expect(os.MkdirAll(filepath.Join(tempDir, dir), 0750)).To(Succeed())
			}

			// A single queued prompt so the preflight check runs.
			Expect(os.WriteFile(
				filepath.Join(tempDir, "prompts/in-progress/001-preflight-test.md"),
				[]byte("---\nstatus: approved\n---\n# Test\n\nTest content\n"),
				0600,
			)).To(Succeed())

			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			_ = os.Chdir(origDir)
			if tempDir != "" {
				_ = os.RemoveAll(tempDir)
			}
		})

		buildPreflightConfig := func() config.Config {
			c := config.Defaults()
			c.PreflightCommand = "false"
			c.HideGit = true
			c.Prompts = config.PromptsConfig{
				InboxDir:      filepath.Join(tempDir, "prompts/inbox"),
				InProgressDir: filepath.Join(tempDir, "prompts/in-progress"),
				CompletedDir:  filepath.Join(tempDir, "prompts/completed"),
				LogDir:        filepath.Join(tempDir, "prompts/log"),
				RejectedDir:   filepath.Join(tempDir, "prompts/rejected"),
			}
			c.Specs = config.SpecsConfig{
				InboxDir:      filepath.Join(tempDir, "specs/inbox"),
				InProgressDir: filepath.Join(tempDir, "specs/in-progress"),
				CompletedDir:  filepath.Join(tempDir, "specs/completed"),
				LogDir:        filepath.Join(tempDir, "specs/log"),
				RejectedDir:   filepath.Join(tempDir, "specs/rejected"),
			}
			return c
		}

		It("CreateRunner.Run returns ErrPreflightFailed", func() {
			c := buildPreflightConfig()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			err := factory.CreateRunner(ctx, c, "v0.0.1", false, false, config.FieldSources{}, libtime.NewCurrentDateTime()).
				Run(ctx)
			Expect(stderrors.Is(err, preflightconditions.ErrPreflightFailed)).To(BeTrue())
		})

		It("CreateOneShotRunner.Run returns ErrPreflightFailed", func() {
			c := buildPreflightConfig()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			err := factory.CreateOneShotRunner(
				ctx, c, "v0.0.1", false, false, config.FieldSources{}, libtime.NewCurrentDateTime(),
			).Run(ctx)
			Expect(stderrors.Is(err, preflightconditions.ErrPreflightFailed)).To(BeTrue())
		})

		It(
			"CreateOneShotRunner.Run returns nil when skip-preflight bypasses failing preflight",
			func() {
				c := buildPreflightConfig()
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				// skipPreflight=true — preflight checker not created, queue is empty → exits with nil
				err := factory.CreateOneShotRunner(ctx, c, "v0.0.1", false, true, config.FieldSources{}, libtime.NewCurrentDateTime()).
					Run(ctx)
				Expect(err).NotTo(HaveOccurred())
			},
		)
	})
})
