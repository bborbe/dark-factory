// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/factory"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/globalconfig"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/subproc"
)

var _ = Describe("Factory", func() {
	var cfg config.Config

	BeforeEach(func() {
		cfg = config.Defaults()
	})

	Describe("CreateRunner", func() {
		It("should return a non-nil runner", func() {
			runner := factory.CreateRunner(context.Background(), cfg, "v0.0.1")
			Expect(runner).NotTo(BeNil())
		})
	})

	Describe("CreateWatcher", func() {
		It("should return a non-nil watcher", func() {
			ready := make(chan struct{}, 10)
			watcher := factory.CreateWatcher(
				cfg.Prompts.InProgressDir,
				cfg.Prompts.InboxDir,
				nil, // promptManager not needed for nil check
				ready,
				100,
				libtime.NewCurrentDateTime(),
			)
			Expect(watcher).NotTo(BeNil())
		})
	})

	Describe("CreateProcessor", func() {
		It("should return a non-nil processor", func() {
			ready := make(chan struct{}, 10)
			processor := factory.CreateProcessor(
				cfg.Prompts.InProgressDir,
				cfg.Prompts.CompletedDir,
				cfg.Prompts.LogDir,
				"test-project",
				nil, // promptManager not needed for nil check
				nil, // releaser not needed for nil check
				nil, // versionGetter not needed for nil check
				ready,
				cfg.ContainerImage,
				cfg.Model,
				cfg.NetrcFile,
				cfg.GitconfigFile,
				cfg.Workflow,
				cfg.PR,
				git.NewBrancher(),
				git.NewPRCreator(""),
				git.NewPRMerger("", libtime.NewCurrentDateTime()),
				false,
				false,
				false,
				"make precommit",
				"",
				"",
				"specs/inbox",
				"specs/in-progress",
				"specs/completed",
				"specs/rejected",
				false,
				nil,
				nil,
				libtime.NewCurrentDateTime(),
				notifier.NewMultiNotifier(),
				cfg.ResolvedClaudeDir(),
				executor.NewDockerContainerCounter(subproc.NewRunner()),
				0,
				"",
				nil,
				nil,
				0,
				processor.NewDirtyFileChecker("."),
				processor.NewGitLockChecker("."),
				0,
				0,
				false,
				nil,
				0, // queueInterval: 0 → default 5s
				0, // sweepInterval: 0 → default 60s
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
			)
			Expect(server).NotTo(BeNil())
		})
	})

	Describe("CreateStatusCommand", func() {
		It("should return a non-nil status command", func() {
			cmd := factory.CreateStatusCommand(context.Background(), cfg)
			Expect(cmd).NotTo(BeNil())
		})
	})

	Describe("CreateCombinedStatusCommand", func() {
		It("should return a non-nil combined status command", func() {
			cmd := factory.CreateCombinedStatusCommand(context.Background(), cfg)
			Expect(cmd).NotTo(BeNil())
		})
	})

	Describe("CreateCombinedListCommand", func() {
		It("should return a non-nil combined list command", func() {
			cmd := factory.CreateCombinedListCommand(cfg)
			Expect(cmd).NotTo(BeNil())
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

				factory.LogEffectiveConfig(c, globalCfg, globalFilePresent)

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

			factory.LogEffectiveConfig(c, globalCfg, false)

			output := logBuf.String()
			assertNoSecrets(output)
		})

		It("emits exactly one log line", func() {
			c := fullTestConfig()
			globalCfg := globalconfig.GlobalConfig{MaxContainers: globalconfig.DefaultMaxContainers}

			factory.LogEffectiveConfig(c, globalCfg, false)

			output := logBuf.String()
			Expect(strings.Count(output, `msg="effective config"`)).To(Equal(1))
		})
	})
})
