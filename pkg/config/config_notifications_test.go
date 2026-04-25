// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg"
	"github.com/bborbe/dark-factory/pkg/config"
)

var _ = Describe("Config", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("VerificationGate", func() {
		It("defaults to false", func() {
			cfg := config.Defaults()
			Expect(cfg.VerificationGate).To(BeFalse())
		})

		It("round-trips verificationGate: true through YAML", func() {
			tmpDir, err := os.MkdirTemp("", "vgate-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			origDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Chdir(origDir) }()

			configContent := "verificationGate: true\n"
			err = os.WriteFile(
				filepath.Join(tmpDir, ".dark-factory.yaml"),
				[]byte(configContent),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := config.NewLoader().Load(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.VerificationGate).To(BeTrue())
		})
	})

	Describe("ResolvedGitHubToken", func() {
		It("returns empty string when github token is not set", func() {
			cfg := config.Config{}
			Expect(cfg.ResolvedGitHubToken()).To(Equal(""))
		})

		It("resolves env var when token matches pattern", func() {
			err := os.Setenv("TEST_VAR", "resolved-value")
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				err := os.Unsetenv("TEST_VAR")
				Expect(err).NotTo(HaveOccurred())
			}()

			cfg := config.Config{
				// #nosec G101 -- test value, not a real credential
				GitHub: config.GitHubConfig{
					Token: "${TEST_VAR}",
				},
			}
			Expect(cfg.ResolvedGitHubToken()).To(Equal("resolved-value"))
		})

		It("returns empty string when env var is unset", func() {
			err := os.Unsetenv("UNSET_VAR")
			Expect(err).NotTo(HaveOccurred())

			cfg := config.Config{
				// #nosec G101 -- test value, not a real credential
				GitHub: config.GitHubConfig{
					Token: "${UNSET_VAR}",
				},
			}
			Expect(cfg.ResolvedGitHubToken()).To(Equal(""))
		})

		It("resolves GITHUB_TOKEN env var reference", func() {
			GinkgoT().Setenv("GITHUB_TOKEN", "resolved-via-github-token")
			cfg := config.Config{
				// #nosec G101 -- test value, not a real credential
				GitHub: config.GitHubConfig{
					Token: "${GITHUB_TOKEN}",
				},
			}
			Expect(cfg.ResolvedGitHubToken()).To(Equal("resolved-via-github-token"))
		})
	})

	Describe("validateGitHubToken", func() {
		It("succeeds when github token is empty", func() {
			cfg := config.Defaults()
			cfg.GitHub.Token = ""
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("succeeds when github token is an env var reference", func() {
			cfg := config.Defaults()
			// #nosec G101 -- test value, not a real credential
			cfg.GitHub.Token = "${GITHUB_TOKEN}"
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("fails when github token is a literal value", func() {
			cfg := config.Defaults()
			// #nosec G101 -- test value, not a real credential
			cfg.GitHub.Token = "ghp_abc123"
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(
				err.Error(),
			).To(ContainSubstring("github.token must be an env var reference like ${GITHUB_TOKEN}, not a literal value"))
		})
	})

	Describe("Validate provider field", func() {
		Context("provider field", func() {
			It("accepts github provider", func() {
				cfg := config.Defaults()
				cfg.Provider = config.ProviderGitHub
				Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
			})

			It("accepts bitbucket-server provider with baseURL", func() {
				cfg := config.Defaults()
				cfg.Provider = config.ProviderBitbucketServer
				cfg.Bitbucket.BaseURL = "https://bitbucket.example.com"
				cfg.Bitbucket.TokenEnv = "BITBUCKET_TOKEN"
				Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
			})

			It("rejects bitbucket-server without baseURL", func() {
				cfg := config.Defaults()
				cfg.Provider = config.ProviderBitbucketServer
				cfg.Bitbucket.BaseURL = ""
				Expect(cfg.Validate(ctx)).To(HaveOccurred())
			})

			It("rejects invalid provider", func() {
				cfg := config.Defaults()
				cfg.Provider = config.Provider("invalid")
				Expect(cfg.Validate(ctx)).To(HaveOccurred())
			})

			It("default provider is github", func() {
				cfg := config.Defaults()
				Expect(cfg.Provider).To(Equal(config.ProviderGitHub))
			})
		})
	})

	Describe("ResolvedDiscordWebhook", func() {
		It("returns empty string when WebhookEnv is empty", func() {
			cfg := config.Config{}
			Expect(cfg.ResolvedDiscordWebhook()).To(Equal(""))
		})

		It("returns env var value when WebhookEnv is set", func() {
			GinkgoT().Setenv("TEST_DISCORD_WEBHOOK", "https://discord.example.com/webhook")
			cfg := config.Config{
				Notifications: config.NotificationsConfig{
					Discord: config.DiscordConfig{WebhookEnv: "TEST_DISCORD_WEBHOOK"},
				},
			}
			Expect(cfg.ResolvedDiscordWebhook()).To(Equal("https://discord.example.com/webhook"))
		})
	})

	Describe("ResolvedTelegramBotToken", func() {
		It("returns empty string when BotTokenEnv is empty", func() {
			cfg := config.Config{}
			Expect(cfg.ResolvedTelegramBotToken()).To(Equal(""))
		})
	})

	Describe("ResolvedTelegramChatID", func() {
		It("returns empty string when ChatIDEnv is empty", func() {
			cfg := config.Config{}
			Expect(cfg.ResolvedTelegramChatID()).To(Equal(""))
		})
	})

	Describe("validateNotifications", func() {
		It("returns nil when no discord webhook configured", func() {
			cfg := config.Defaults()
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("returns nil when discord webhook is HTTPS", func() {
			GinkgoT().Setenv("TEST_DISCORD_WEBHOOK_HTTPS", "https://discord.example.com/webhook")
			cfg := config.Defaults()
			cfg.Notifications.Discord.WebhookEnv = "TEST_DISCORD_WEBHOOK_HTTPS"
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("returns error when discord webhook is HTTP", func() {
			GinkgoT().Setenv("TEST_DISCORD_WEBHOOK_HTTP", "http://discord.example.com/webhook")
			cfg := config.Defaults()
			cfg.Notifications.Discord.WebhookEnv = "TEST_DISCORD_WEBHOOK_HTTP"
			Expect(cfg.Validate(ctx)).To(HaveOccurred())
		})
	})

	Describe("ValidationPrompt", func() {
		It("defaults to empty string", func() {
			Expect(config.Defaults().ValidationPrompt).To(Equal(""))
		})

		It("accepts empty string", func() {
			cfg := config.Defaults()
			cfg.ValidationPrompt = ""
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("accepts inline text with spaces", func() {
			cfg := config.Defaults()
			cfg.ValidationPrompt = "readme.md is updated"
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("accepts relative path", func() {
			cfg := config.Defaults()
			cfg.ValidationPrompt = "docs/dod.md"
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("rejects absolute path", func() {
			cfg := config.Defaults()
			cfg.ValidationPrompt = "/etc/passwd"
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("absolute path"))
		})

		It("rejects path traversal with ..", func() {
			cfg := config.Defaults()
			cfg.ValidationPrompt = "../outside.md"
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("outside the project root"))
		})

		It("rejects deep path traversal", func() {
			cfg := config.Defaults()
			cfg.ValidationPrompt = "../../etc/passwd"
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("outside the project root"))
		})
	})

	Describe("validateExtraMounts", func() {
		It("passes when ExtraMounts is nil", func() {
			cfg := config.Defaults()
			cfg.ExtraMounts = nil
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("passes when ExtraMounts is empty slice", func() {
			cfg := config.Defaults()
			cfg.ExtraMounts = []config.ExtraMount{}
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("passes for valid entry with non-empty src and dst", func() {
			cfg := config.Defaults()
			cfg.ExtraMounts = []config.ExtraMount{
				{Src: "/some/path", Dst: "/container/path"},
			}
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("fails for entry with empty src", func() {
			cfg := config.Defaults()
			cfg.ExtraMounts = []config.ExtraMount{
				{Src: "", Dst: "/container/path"},
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("src must not be empty"))
		})

		It("fails for entry with empty dst", func() {
			cfg := config.Defaults()
			cfg.ExtraMounts = []config.ExtraMount{
				{Src: "/some/path", Dst: ""},
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dst must not be empty"))
		})
	})

	Describe("ExtraMount.IsReadonly", func() {
		It("returns false when ReadOnly is nil (default)", func() {
			m := config.ExtraMount{Src: "/src", Dst: "/dst"}
			Expect(m.IsReadonly()).To(BeFalse())
		})

		It("returns true when ReadOnly is explicitly true", func() {
			t := true
			m := config.ExtraMount{Src: "/src", Dst: "/dst", ReadOnly: &t}
			Expect(m.IsReadonly()).To(BeTrue())
		})

		It("returns false when ReadOnly is explicitly false", func() {
			f := false
			m := config.ExtraMount{Src: "/src", Dst: "/dst", ReadOnly: &f}
			Expect(m.IsReadonly()).To(BeFalse())
		})
	})

	Describe("MaxContainers validation", func() {
		validBase := func() config.Config {
			return config.Config{
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
			}
		}

		It("succeeds when maxContainers is missing (zero)", func() {
			cfg := validBase()
			Expect(cfg.MaxContainers).To(Equal(0))
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("succeeds when maxContainers is zero (unset)", func() {
			cfg := validBase()
			cfg.MaxContainers = 0
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("succeeds when maxContainers is 1", func() {
			cfg := validBase()
			cfg.MaxContainers = 1
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("succeeds when maxContainers is 5", func() {
			cfg := validBase()
			cfg.MaxContainers = 5
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("fails when maxContainers is -1", func() {
			cfg := validBase()
			cfg.MaxContainers = -1
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maxContainers"))
		})
	})

	Describe("DirtyFileThreshold", func() {
		validBase := func() config.Config {
			return config.Config{
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
			}
		}

		It("succeeds when dirtyFileThreshold is 0 (disabled)", func() {
			cfg := validBase()
			cfg.DirtyFileThreshold = 0
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("succeeds when dirtyFileThreshold is 10", func() {
			cfg := validBase()
			cfg.DirtyFileThreshold = 10
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("fails when dirtyFileThreshold is -1", func() {
			cfg := validBase()
			cfg.DirtyFileThreshold = -1
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dirtyFileThreshold"))
		})
	})

	Describe("MaxPromptDuration", func() {
		validBase := func() config.Config {
			return config.Config{
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
			}
		}

		It("succeeds when maxPromptDuration is empty (disabled)", func() {
			cfg := validBase()
			cfg.MaxPromptDuration = ""
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
			Expect(cfg.ParsedMaxPromptDuration()).To(Equal(time.Duration(0)))
		})

		It("succeeds when maxPromptDuration is '0'", func() {
			cfg := validBase()
			cfg.MaxPromptDuration = "0"
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
			Expect(cfg.ParsedMaxPromptDuration()).To(Equal(time.Duration(0)))
		})

		It("succeeds when maxPromptDuration is '90m'", func() {
			cfg := validBase()
			cfg.MaxPromptDuration = "90m"
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
			Expect(cfg.ParsedMaxPromptDuration()).To(Equal(90 * time.Minute))
		})

		It("fails when maxPromptDuration is invalid", func() {
			cfg := validBase()
			cfg.MaxPromptDuration = "invalid"
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maxPromptDuration"))
		})
	})

	Describe("AutoRetryLimit", func() {
		validBase := func() config.Config {
			return config.Config{
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
			}
		}

		It("succeeds when autoRetryLimit is 0 (disabled)", func() {
			cfg := validBase()
			cfg.AutoRetryLimit = 0
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("succeeds when autoRetryLimit is 3", func() {
			cfg := validBase()
			cfg.AutoRetryLimit = 3
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("fails when autoRetryLimit is -1", func() {
			cfg := validBase()
			cfg.AutoRetryLimit = -1
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("autoRetryLimit"))
		})
	})

	Describe("legacy worktree: bool mapping", func() {
		var tmpDir string
		var origDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "compat-test-*")
			Expect(err).NotTo(HaveOccurred())
			origDir, err = os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tmpDir)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			err := os.Chdir(origDir)
			Expect(err).NotTo(HaveOccurred())
			err = os.RemoveAll(tmpDir)
			Expect(err).NotTo(HaveOccurred())
		})

		writeConfig := func(yaml string) {
			err := os.WriteFile(
				filepath.Join(tmpDir, ".dark-factory.yaml"),
				[]byte(yaml),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())
		}

		DescribeTable(
			"compatibility matrix",
			func(yamlContent string, expectedWorkflow config.Workflow, expectedPR bool, expectedWorktree bool) {
				writeConfig(yamlContent)
				cfg, err := config.NewLoader().Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Workflow).To(Equal(expectedWorkflow), "workflow mismatch")
				Expect(cfg.PR).To(Equal(expectedPR), "pr mismatch")
				Expect(cfg.Worktree).To(BeFalse(), "worktree must always be zeroed after load")
			},
			Entry("worktree: false, pr: false → direct",
				"worktree: false\npr: false\n",
				config.WorkflowDirect, false, false,
			),
			Entry("worktree: false, pr: true → branch",
				"worktree: false\npr: true\n",
				config.WorkflowBranch, true, false,
			),
			Entry("worktree: true, pr: true → clone",
				"worktree: true\npr: true\n",
				config.WorkflowClone, true, false,
			),
			Entry("worktree: true, pr: false → clone (pr overridden to true)",
				"worktree: true\npr: false\n",
				config.WorkflowClone, true, false,
			),
			Entry("workflow: pr → clone, pr: true",
				"workflow: pr\n",
				config.WorkflowClone, true, false,
			),
			Entry("workflow: worktree + worktree: true → worktree wins, worktree field zeroed",
				"workflow: worktree\nworktree: true\n",
				config.WorkflowWorktree, false, false,
			),
		)

		It("row 4 (worktree: true, pr: false) emits a slog.Warn about pr override", func() {
			origDefault := slog.Default()
			var logBuf bytes.Buffer
			slog.SetDefault(
				slog.New(
					slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}),
				),
			)
			defer slog.SetDefault(origDefault)

			writeConfig("worktree: true\npr: false\n")
			_, err := config.NewLoader().Load(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(logBuf.String()).To(ContainSubstring("worktree: true, pr: false"))
		})

		It(
			"workflow: worktree, pr: true (new-style, no legacy fields) emits no deprecation warning",
			func() {
				origDefault := slog.Default()
				var logBuf bytes.Buffer
				slog.SetDefault(
					slog.New(
						slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}),
					),
				)
				defer slog.SetDefault(origDefault)

				writeConfig("workflow: worktree\npr: true\n")
				_, err := config.NewLoader().Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(logBuf.String()).NotTo(ContainSubstring("deprecated"))
				Expect(logBuf.String()).NotTo(ContainSubstring("deprecated"))
			},
		)
	})

	Describe("Workflow.Validate enum coverage", func() {
		DescribeTable("valid workflows",
			func(w config.Workflow) {
				err := w.Validate(ctx)
				Expect(err).NotTo(HaveOccurred())
			},
			Entry("direct", config.WorkflowDirect),
			Entry("branch", config.WorkflowBranch),
			Entry("worktree", config.WorkflowWorktree),
			Entry("clone", config.WorkflowClone),
		)

		It("typo returns error listing all four valid values", func() {
			err := config.Workflow("typo").Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("typo"))
			Expect(err.Error()).To(ContainSubstring("direct"))
			Expect(err.Error()).To(ContainSubstring("branch"))
			Expect(err.Error()).To(ContainSubstring("worktree"))
			Expect(err.Error()).To(ContainSubstring("clone"))
		})

		It("empty string returns error", func() {
			err := config.Workflow("").Validate(ctx)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Config.Validate workflow+pr combination", func() {
		validBase := func() config.Config {
			return config.Config{
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
			}
		}

		It("workflow: direct, pr: false → valid", func() {
			cfg := validBase()
			cfg.Workflow = config.WorkflowDirect
			cfg.PR = false
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("workflow: direct, pr: true → error mentioning 'direct' and 'pr: true'", func() {
			cfg := validBase()
			cfg.Workflow = config.WorkflowDirect
			cfg.PR = true
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("direct"))
			Expect(err.Error()).To(ContainSubstring("pr: true"))
		})

		It("workflow: branch, pr: true → valid", func() {
			cfg := validBase()
			cfg.Workflow = config.WorkflowBranch
			cfg.PR = true
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})

		It("workflow: clone, pr: false → valid", func() {
			cfg := validBase()
			cfg.Workflow = config.WorkflowClone
			cfg.PR = false
			Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
		})
	})

	Describe("sibling project configs", func() {
		var tmpDir string
		var origDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "sibling-test-*")
			Expect(err).NotTo(HaveOccurred())
			origDir, err = os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tmpDir)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			err := os.Chdir(origDir)
			Expect(err).NotTo(HaveOccurred())
			err = os.RemoveAll(tmpDir)
			Expect(err).NotTo(HaveOccurred())
		})

		writeConfig := func(yaml string) {
			err := os.WriteFile(
				filepath.Join(tmpDir, ".dark-factory.yaml"),
				[]byte(yaml),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())
		}

		It(
			"billomat/mdm/commerce style (worktree: true, no explicit pr) → workflow: clone, pr: true",
			func() {
				writeConfig("worktree: true\n")
				cfg, err := config.NewLoader().Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Workflow).To(Equal(config.WorkflowClone))
				Expect(cfg.PR).To(BeTrue())
				Expect(cfg.Worktree).To(BeFalse())
			},
		)

		It(
			"projects using direct mode (no workflow field at all) → workflow: direct, pr: false",
			func() {
				writeConfig("projectName: some-project\n")
				cfg, err := config.NewLoader().Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Workflow).To(Equal(config.WorkflowDirect))
				Expect(cfg.PR).To(BeFalse())
				Expect(cfg.Worktree).To(BeFalse())
			},
		)

		It("projects using workflow: direct explicitly → workflow: direct, pr: false", func() {
			writeConfig("workflow: direct\n")
			cfg, err := config.NewLoader().Load(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Workflow).To(Equal(config.WorkflowDirect))
			Expect(cfg.PR).To(BeFalse())
			Expect(cfg.Worktree).To(BeFalse())
		})
	})

	Describe("validatePreflightInterval", func() {
		It("rejects invalid preflightInterval", func() {
			cfg := config.Defaults()
			cfg.PreflightInterval = "2x"
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("preflightInterval"))
		})

		It("allows empty preflightInterval (disables preflight)", func() {
			cfg := config.Defaults()
			cfg.PreflightInterval = ""
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("ParsedPreflightInterval", func() {
		It("parses valid duration", func() {
			cfg := config.Defaults()
			cfg.PreflightInterval = "2h"
			Expect(cfg.ParsedPreflightInterval()).To(Equal(2 * time.Hour))
		})

		It("returns 0 for empty", func() {
			cfg := config.Defaults()
			cfg.PreflightInterval = ""
			Expect(cfg.ParsedPreflightInterval()).To(Equal(time.Duration(0)))
		})
	})

	Describe("validateQueueInterval", func() {
		It("rejects invalid duration string", func() {
			cfg := config.Defaults()
			cfg.QueueInterval = "bad"
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("queueInterval"))
		})

		It("rejects zero duration", func() {
			cfg := config.Defaults()
			cfg.QueueInterval = "0s"
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("queueInterval"))
		})

		It("rejects negative duration", func() {
			cfg := config.Defaults()
			cfg.QueueInterval = "-1s"
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("queueInterval"))
		})

		It("allows empty queueInterval (uses default)", func() {
			cfg := config.Defaults()
			cfg.QueueInterval = ""
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("allows valid positive duration", func() {
			cfg := config.Defaults()
			cfg.QueueInterval = "10s"
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("validateSweepInterval", func() {
		It("rejects invalid duration string", func() {
			cfg := config.Defaults()
			cfg.SweepInterval = "bad"
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("sweepInterval"))
		})

		It("rejects zero duration", func() {
			cfg := config.Defaults()
			cfg.SweepInterval = "0s"
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("sweepInterval"))
		})

		It("rejects negative duration", func() {
			cfg := config.Defaults()
			cfg.SweepInterval = "-1s"
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("sweepInterval"))
		})

		It("allows empty sweepInterval (uses default)", func() {
			cfg := config.Defaults()
			cfg.SweepInterval = ""
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("allows valid positive duration", func() {
			cfg := config.Defaults()
			cfg.SweepInterval = "2m"
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("ParsedQueueInterval", func() {
		It("parses valid duration", func() {
			cfg := config.Defaults()
			cfg.QueueInterval = "10s"
			Expect(cfg.ParsedQueueInterval()).To(Equal(10 * time.Second))
		})

		It("returns 5s for empty", func() {
			cfg := config.Defaults()
			cfg.QueueInterval = ""
			Expect(cfg.ParsedQueueInterval()).To(Equal(5 * time.Second))
		})

		It("returns 5s for unparseable string", func() {
			cfg := config.Defaults()
			cfg.QueueInterval = "bad"
			Expect(cfg.ParsedQueueInterval()).To(Equal(5 * time.Second))
		})
	})

	Describe("ParsedSweepInterval", func() {
		It("parses valid duration", func() {
			cfg := config.Defaults()
			cfg.SweepInterval = "2m"
			Expect(cfg.ParsedSweepInterval()).To(Equal(2 * time.Minute))
		})

		It("returns 60s for empty", func() {
			cfg := config.Defaults()
			cfg.SweepInterval = ""
			Expect(cfg.ParsedSweepInterval()).To(Equal(60 * time.Second))
		})

		It("returns 60s for unparseable string", func() {
			cfg := config.Defaults()
			cfg.SweepInterval = "bad"
			Expect(cfg.ParsedSweepInterval()).To(Equal(60 * time.Second))
		})
	})
})
