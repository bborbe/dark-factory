// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/config"
)

var _ = Describe("Config", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("Defaults", func() {
		It("returns config with default values", func() {
			cfg := config.Defaults()
			Expect(cfg.Workflow).To(Equal(config.WorkflowDirect))
			Expect(cfg.InboxDir).To(Equal("prompts"))
			Expect(cfg.QueueDir).To(Equal("prompts/queue"))
			Expect(cfg.CompletedDir).To(Equal("prompts/completed"))
			Expect(cfg.LogDir).To(Equal("prompts/log"))
			Expect(cfg.ContainerImage).To(Equal("docker.io/bborbe/claude-yolo:v0.2.2"))
			Expect(cfg.Model).To(Equal("claude-sonnet-4-6"))
			Expect(cfg.DebounceMs).To(Equal(500))
			Expect(cfg.ServerPort).To(Equal(0))
			Expect(cfg.SpecDir).To(Equal("specs"))
			Expect(cfg.AutoMerge).To(BeFalse())
			Expect(cfg.AutoRelease).To(BeFalse())
			Expect(cfg.GitHub.Token).To(Equal(config.DefaultGitHubTokenRef))
		})
	})

	Describe("Validate", func() {
		It("succeeds for valid config", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds for separate inbox and queue dirs", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts/queue",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds for worktree workflow", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowWorktree,
				InboxDir:       "prompts",
				QueueDir:       "prompts/queue",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails for invalid workflow", func() {
			cfg := config.Config{
				Workflow:       "invalid",
				InboxDir:       "prompts",
				QueueDir:       "prompts",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workflow"))
		})

		It("fails for empty inboxDir", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "",
				QueueDir:       "prompts",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("inboxDir"))
		})

		It("fails for empty queueDir", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("queueDir"))
		})

		It("fails for empty completedDir", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts",
				CompletedDir:   "",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("completedDir"))
		})

		It("fails for empty logDir", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts",
				CompletedDir:   "prompts/completed",
				LogDir:         "",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("logDir"))
		})

		It("fails when completedDir equals queueDir", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts/queue",
				CompletedDir:   "prompts/queue",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("completedDir cannot equal queueDir"))
		})

		It("fails when completedDir equals inboxDir", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts/queue",
				CompletedDir:   "prompts",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("completedDir cannot equal inboxDir"))
		})

		It("fails for empty containerImage", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("containerImage"))
		})

		It("fails for negative debounceMs", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     -1,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("debounceMs"))
		})

		It("fails for zero debounceMs", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     0,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("debounceMs"))
		})

		It("succeeds for serverPort 0 (disabled)", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     0,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds for valid serverPort", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails for negative serverPort", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     -1,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("serverPort"))
		})

		It("fails for serverPort greater than 65535", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     65536,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("serverPort"))
		})

		It("succeeds for autoMerge true with workflow pr", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowPR,
				InboxDir:       "prompts",
				QueueDir:       "prompts/queue",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
				AutoMerge:      true,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds for autoMerge true with workflow worktree", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowWorktree,
				InboxDir:       "prompts",
				QueueDir:       "prompts/queue",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
				AutoMerge:      true,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails for autoMerge true with workflow direct", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
				AutoMerge:      true,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(
				err.Error(),
			).To(ContainSubstring("autoMerge requires workflow 'pr' or 'worktree'"))
		})

		It("succeeds for autoRelease true with autoMerge true", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowPR,
				InboxDir:       "prompts",
				QueueDir:       "prompts/queue",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
				AutoMerge:      true,
				AutoRelease:    true,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails for autoRelease true with autoMerge false", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowPR,
				InboxDir:       "prompts",
				QueueDir:       "prompts/queue",
				CompletedDir:   "prompts/completed",
				LogDir:         "prompts/log",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.2.2",
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
				AutoMerge:      false,
				AutoRelease:    true,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("autoRelease requires autoMerge"))
		})
	})

	Describe("Workflow", func() {
		Describe("Validate", func() {
			It("succeeds for direct workflow", func() {
				err := config.WorkflowDirect.Validate(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("succeeds for pr workflow", func() {
				err := config.WorkflowPR.Validate(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("succeeds for worktree workflow", func() {
				err := config.WorkflowWorktree.Validate(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("fails for unknown workflow", func() {
				err := config.Workflow("unknown").Validate(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown workflow"))
			})
		})

		Describe("String", func() {
			It("returns string representation", func() {
				Expect(config.WorkflowDirect.String()).To(Equal("direct"))
				Expect(config.WorkflowPR.String()).To(Equal("pr"))
				Expect(config.WorkflowWorktree.String()).To(Equal("worktree"))
			})
		})

		Describe("Ptr", func() {
			It("returns pointer to workflow", func() {
				ptr := config.WorkflowDirect.Ptr()
				Expect(ptr).NotTo(BeNil())
				Expect(*ptr).To(Equal(config.WorkflowDirect))
			})
		})
	})

	Describe("Workflows", func() {
		Describe("Contains", func() {
			It("returns true for valid workflow", func() {
				Expect(config.AvailableWorkflows.Contains(config.WorkflowDirect)).To(BeTrue())
				Expect(config.AvailableWorkflows.Contains(config.WorkflowPR)).To(BeTrue())
				Expect(config.AvailableWorkflows.Contains(config.WorkflowWorktree)).To(BeTrue())
			})

			It("returns false for invalid workflow", func() {
				Expect(config.AvailableWorkflows.Contains("invalid")).To(BeFalse())
			})
		})
	})

	Describe("Loader", func() {
		var tmpDir string
		var originalDir string
		var loader config.Loader

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "config-test-*")
			Expect(err).NotTo(HaveOccurred())

			originalDir, err = os.Getwd()
			Expect(err).NotTo(HaveOccurred())

			err = os.Chdir(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			loader = config.NewLoader()
		})

		AfterEach(func() {
			err := os.Chdir(originalDir)
			Expect(err).NotTo(HaveOccurred())
			err = os.RemoveAll(tmpDir)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("Load", func() {
			It("returns defaults when config file does not exist", func() {
				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).To(Equal(config.Defaults()))
			})

			It("loads full config from file", func() {
				configContent := `workflow: pr
inboxDir: custom-prompts
queueDir: custom-prompts/queue
completedDir: custom-prompts/done
logDir: custom-prompts/logs
containerImage: custom-image:latest
debounceMs: 1000
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Workflow).To(Equal(config.WorkflowPR))
				Expect(cfg.InboxDir).To(Equal("custom-prompts"))
				Expect(cfg.QueueDir).To(Equal("custom-prompts/queue"))
				Expect(cfg.CompletedDir).To(Equal("custom-prompts/done"))
				Expect(cfg.LogDir).To(Equal("custom-prompts/logs"))
				Expect(cfg.ContainerImage).To(Equal("custom-image:latest"))
				Expect(cfg.DebounceMs).To(Equal(1000))
			})

			It("merges partial config with defaults", func() {
				configContent := `workflow: pr
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Workflow).To(Equal(config.WorkflowPR))
				Expect(cfg.InboxDir).To(Equal("prompts"))
				Expect(cfg.QueueDir).To(Equal("prompts/queue"))
				Expect(cfg.CompletedDir).To(Equal("prompts/completed"))
				Expect(cfg.LogDir).To(Equal("prompts/log"))
				Expect(cfg.ContainerImage).To(Equal("docker.io/bborbe/claude-yolo:v0.2.2"))
				Expect(cfg.DebounceMs).To(Equal(500))
			})

			It("returns error for invalid YAML", func() {
				configContent := `workflow: pr
invalid yaml: [unclosed
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				_, err = loader.Load(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("parse config file"))
			})

			It("returns error for invalid workflow value", func() {
				configContent := `workflow: invalid
inboxDir: prompts
queueDir: prompts
completedDir: prompts/completed
logDir: prompts/log
containerImage: docker.io/bborbe/claude-yolo:v0.2.2
debounceMs: 500
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				_, err = loader.Load(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("validate config"))
			})

			It("returns error for negative debounceMs", func() {
				configContent := `workflow: direct
inboxDir: prompts
queueDir: prompts
completedDir: prompts/completed
logDir: prompts/log
containerImage: docker.io/bborbe/claude-yolo:v0.2.2
debounceMs: -100
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				_, err = loader.Load(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("validate config"))
			})

			It("loads config from world-readable file without error", func() {
				configContent := `workflow: pr
`
				configPath := filepath.Join(tmpDir, ".dark-factory.yaml")
				err := os.WriteFile(configPath, []byte(configContent), 0600)
				Expect(err).NotTo(HaveOccurred())

				// Make world-readable after creation to avoid gosec G306
				err = os.Chmod(configPath, 0644) // #nosec G302 -- intentional for test
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Workflow).To(Equal(config.WorkflowPR))
			})

			It("loads config with github token from env var", func() {
				configContent := `workflow: pr
github:
  token: ${TEST_VAR}
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				err = os.Setenv("TEST_VAR", "test-token-value")
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					err := os.Unsetenv("TEST_VAR")
					Expect(err).NotTo(HaveOccurred())
				}()

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.GitHub.Token).To(Equal("${TEST_VAR}"))
				Expect(cfg.ResolvedGitHubToken()).To(Equal("test-token-value"))
			})

			It("loads config without github section uses default token ref", func() {
				configContent := `workflow: pr
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.GitHub.Token).To(Equal(config.DefaultGitHubTokenRef))
			})

			It("loads config with autoMerge and autoRelease", func() {
				configContent := `workflow: pr
autoMerge: true
autoRelease: true
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Workflow).To(Equal(config.WorkflowPR))
				Expect(cfg.AutoMerge).To(BeTrue())
				Expect(cfg.AutoRelease).To(BeTrue())
			})
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

		It("returns literal value when token does not match pattern", func() {
			cfg := config.Config{
				GitHub: config.GitHubConfig{
					Token: "literal-token",
				},
			}
			Expect(cfg.ResolvedGitHubToken()).To(Equal("literal-token"))
		})
	})
})
