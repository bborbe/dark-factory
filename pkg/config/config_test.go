// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config_test

import (
	"context"
	"os"

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

	Describe("Defaults", func() {
		It("returns config with default values", func() {
			cfg := config.Defaults()
			Expect(cfg.Workflow).To(Equal(config.WorkflowDirect))
			Expect(cfg.Prompts.InboxDir).To(Equal("prompts"))
			Expect(cfg.Prompts.InProgressDir).To(Equal("prompts/in-progress"))
			Expect(cfg.Prompts.CompletedDir).To(Equal("prompts/completed"))
			Expect(cfg.Prompts.LogDir).To(Equal("prompts/log"))
			Expect(cfg.Specs.InboxDir).To(Equal("specs"))
			Expect(cfg.Specs.InProgressDir).To(Equal("specs/in-progress"))
			Expect(cfg.Specs.CompletedDir).To(Equal("specs/completed"))
			Expect(cfg.Specs.LogDir).To(Equal("specs/log"))
			Expect(cfg.ContainerImage).To(Equal(pkg.DefaultContainerImage))
			Expect(cfg.Model).To(Equal("claude-sonnet-4-6"))
			Expect(cfg.ValidationCommand).To(Equal("make precommit"))
			Expect(cfg.ValidationPrompt).To(Equal(""))
			Expect(cfg.TestCommand).To(Equal("make test"))
			Expect(cfg.DebounceMs).To(Equal(500))
			Expect(cfg.ServerPort).To(Equal(0))
			Expect(cfg.AutoMerge).To(BeFalse())
			Expect(cfg.AutoRelease).To(BeFalse())
			Expect(cfg.AutoReview).To(BeFalse())
			Expect(cfg.MaxReviewRetries).To(Equal(3))
			Expect(cfg.PollIntervalSec).To(Equal(60))
			Expect(cfg.UseCollaborators).To(BeFalse())
			Expect(cfg.GitHub.Token).To(BeEmpty())
			Expect(cfg.PreflightCommand).To(Equal("make precommit"))
			Expect(cfg.PreflightInterval).To(Equal("8h"))
			Expect(cfg.QueueInterval).To(Equal("5s"))
			Expect(cfg.SweepInterval).To(Equal("60s"))
		})
	})

	Describe("Validate", func() {
		It("succeeds for valid config", func() {
			cfg := config.Config{
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds when TestCommand is empty string (explicit opt-out)", func() {
			cfg := config.Config{
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
				TestCommand:    "",
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds when TestCommand is set to make test", func() {
			cfg := config.Config{
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
				TestCommand:    "make test",
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds for separate inbox and inProgress dirs", func() {
			cfg := config.Config{
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
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds for worktree workflow", func() {
			cfg := config.Config{
				Workflow: config.WorkflowWorktree,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails for invalid workflow", func() {
			cfg := config.Config{
				Workflow: "invalid",
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
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
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "",
					InProgressDir: "prompts",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("inboxDir"))
		})

		It("fails for empty inProgressDir", func() {
			cfg := config.Config{
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("inProgressDir"))
		})

		It("fails for empty completedDir", func() {
			cfg := config.Config{
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts",
					CompletedDir:  "",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
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
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts",
					CompletedDir:  "prompts/completed",
					LogDir:        "",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("logDir"))
		})

		It("fails when completedDir equals inProgressDir", func() {
			cfg := config.Config{
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/in-progress",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("completedDir cannot equal inProgressDir"))
		})

		It("fails when completedDir equals inboxDir", func() {
			cfg := config.Config{
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
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
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
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
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
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
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
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
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     0,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds for valid serverPort", func() {
			cfg := config.Config{
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails for negative serverPort", func() {
			cfg := config.Config{
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
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
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     65536,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("serverPort"))
		})

		It("succeeds for autoMerge true with workflow clone and pr: true", func() {
			cfg := config.Config{
				Workflow: config.WorkflowClone,
				PR:       true,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
				AutoMerge:      true,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It(
			"fails for autoMerge true with workflow direct and pr: true (direct incompatible with pr)",
			func() {
				cfg := config.Config{
					Workflow: config.WorkflowDirect,
					PR:       true,
					Prompts: config.PromptsConfig{
						InboxDir:      "prompts",
						InProgressDir: "prompts/in-progress",
						CompletedDir:  "prompts/completed",
						LogDir:        "prompts/log",
					},
					ContainerImage: pkg.DefaultContainerImage,
					Model:          "claude-sonnet-4-6",
					DebounceMs:     500,
					ServerPort:     8080,
					AutoMerge:      true,
				}
				err := cfg.Validate(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("direct"))
				Expect(err.Error()).To(ContainSubstring("pr: true"))
			},
		)

		It(
			"fails for autoMerge true with workflow worktree and pr: false (autoMerge requires pr)",
			func() {
				cfg := config.Config{
					Workflow: config.WorkflowWorktree,
					PR:       false,
					Prompts: config.PromptsConfig{
						InboxDir:      "prompts",
						InProgressDir: "prompts/in-progress",
						CompletedDir:  "prompts/completed",
						LogDir:        "prompts/log",
					},
					ContainerImage: pkg.DefaultContainerImage,
					Model:          "claude-sonnet-4-6",
					DebounceMs:     500,
					ServerPort:     8080,
					AutoMerge:      true,
				}
				err := cfg.Validate(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("autoMerge requires pr: true"))
			},
		)

		It("fails for autoMerge true with workflow direct", func() {
			cfg := config.Config{
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
				AutoMerge:      true,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(
				err.Error(),
			).To(ContainSubstring("autoMerge requires pr: true"))
		})

		It("fails for autoMerge true with pr: false (no workflow field)", func() {
			cfg := config.Config{
				Workflow: config.WorkflowDirect,
				PR:       false,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
				AutoMerge:      true,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("autoMerge requires pr: true"))
		})

		It("succeeds for autoRelease true with autoMerge true", func() {
			cfg := config.Config{
				Workflow: config.WorkflowClone,
				PR:       true,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
				AutoMerge:      true,
				AutoRelease:    true,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It(
			"succeeds for autoRelease true with autoMerge false (autoRelease is independent)",
			func() {
				cfg := config.Config{
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
					ServerPort:     8080,
					AutoMerge:      false,
					AutoRelease:    true,
				}
				err := cfg.Validate(ctx)
				Expect(err).NotTo(HaveOccurred())
			},
		)

		It("fails for autoReview true with workflow direct", func() {
			cfg := config.Config{
				Workflow: config.WorkflowDirect,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage:   pkg.DefaultContainerImage,
				Model:            "claude-sonnet-4-6",
				DebounceMs:       500,
				ServerPort:       8080,
				AutoMerge:        false,
				AutoReview:       true,
				AllowedReviewers: []string{"bborbe"},
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(
				err.Error(),
			).To(ContainSubstring("autoReview requires pr: true"))
		})

		It("fails for autoReview true with pr: false (no workflow field)", func() {
			cfg := config.Config{
				Workflow: config.WorkflowDirect,
				PR:       false,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage:   pkg.DefaultContainerImage,
				Model:            "claude-sonnet-4-6",
				DebounceMs:       500,
				ServerPort:       8080,
				AutoReview:       true,
				AllowedReviewers: []string{"bborbe"},
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("autoReview requires pr: true"))
		})

		It("fails for autoReview true with autoMerge false", func() {
			cfg := config.Config{
				Workflow: config.WorkflowClone,
				PR:       true,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage:   pkg.DefaultContainerImage,
				Model:            "claude-sonnet-4-6",
				DebounceMs:       500,
				ServerPort:       8080,
				AutoMerge:        false,
				AutoReview:       true,
				AllowedReviewers: []string{"bborbe"},
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("autoReview requires autoMerge"))
		})

		It("fails for autoReview true with no reviewer source", func() {
			cfg := config.Config{
				Workflow: config.WorkflowClone,
				PR:       true,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
				AutoMerge:      true,
				AutoReview:     true,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(
				err.Error(),
			).To(ContainSubstring("autoReview requires allowedReviewers or useCollaborators: true"))
		})

		It("succeeds for autoReview true with all required fields", func() {
			cfg := config.Config{
				Workflow: config.WorkflowClone,
				PR:       true,
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage:   pkg.DefaultContainerImage,
				Model:            "claude-sonnet-4-6",
				DebounceMs:       500,
				ServerPort:       8080,
				AutoMerge:        true,
				AutoReview:       true,
				AllowedReviewers: []string{"bborbe"},
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds with DefaultBranch set to master", func() {
			cfg := config.Config{
				Workflow:      config.WorkflowDirect,
				DefaultBranch: "master",
				Prompts: config.PromptsConfig{
					InboxDir:      "prompts",
					InProgressDir: "prompts/in-progress",
					CompletedDir:  "prompts/completed",
					LogDir:        "prompts/log",
				},
				ContainerImage: pkg.DefaultContainerImage,
				Model:          "claude-sonnet-4-6",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds without DefaultBranch set (optional field)", func() {
			cfg := config.Config{
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
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds when netrcFile is empty (no mount)", func() {
			cfg := config.Config{
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
				ServerPort:     8080,
				NetrcFile:      "",
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds when netrcFile points to an existing file", func() {
			tmpFile, err := os.CreateTemp("", "test-netrc-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Remove(tmpFile.Name()) }()
			_ = tmpFile.Close()

			cfg := config.Config{
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
				ServerPort:     8080,
				NetrcFile:      tmpFile.Name(),
			}
			err = cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails when netrcFile points to a nonexistent file", func() {
			cfg := config.Config{
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
				ServerPort:     8080,
				NetrcFile:      "/nonexistent/path/.netrc",
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("netrcFile"))
			Expect(err.Error()).To(ContainSubstring("does not exist"))
		})

		It("succeeds when gitconfigFile is empty (no mount)", func() {
			cfg := config.Config{
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
				ServerPort:     8080,
				GitconfigFile:  "",
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds when gitconfigFile points to an existing file", func() {
			tmpFile, err := os.CreateTemp("", "test-gitconfig-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Remove(tmpFile.Name()) }()
			_ = tmpFile.Close()

			cfg := config.Config{
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
				ServerPort:     8080,
				GitconfigFile:  tmpFile.Name(),
			}
			err = cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails when gitconfigFile points to a nonexistent file", func() {
			cfg := config.Config{
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
				ServerPort:     8080,
				GitconfigFile:  "/nonexistent/path/.gitconfig",
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("gitconfigFile"))
			Expect(err.Error()).To(ContainSubstring("does not exist"))
		})

		It("succeeds with valid env keys", func() {
			cfg := config.Config{
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
				ServerPort:     8080,
				Env: map[string]string{
					"GOPRIVATE":    "bitbucket.example.com/*",
					"GONOSUMCHECK": "bitbucket.example.com/*",
				},
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails when env has empty key", func() {
			cfg := config.Config{
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
				ServerPort:     8080,
				Env:            map[string]string{"": "value"},
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("env key must not be empty"))
		})

		It("fails when env has reserved key YOLO_PROMPT_FILE", func() {
			cfg := config.Config{
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
				ServerPort:     8080,
				Env:            map[string]string{"YOLO_PROMPT_FILE": "/custom/path"},
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reserved"))
			Expect(err.Error()).To(ContainSubstring("YOLO_PROMPT_FILE"))
		})

		It("fails when env has reserved key ANTHROPIC_MODEL", func() {
			cfg := config.Config{
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
				ServerPort:     8080,
				Env:            map[string]string{"ANTHROPIC_MODEL": "custom-model"},
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reserved"))
			Expect(err.Error()).To(ContainSubstring("ANTHROPIC_MODEL"))
		})

		DescribeTable("fails when env value contains control characters",
			func(value string) {
				cfg := config.Config{
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
					Env:            map[string]string{"MY_VAR": value},
				}
				err := cfg.Validate(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("contains invalid characters"))
			},
			Entry("newline", "value\ninjected"),
			Entry("null byte", "value\x00injected"),
			Entry("carriage return", "value\rinjected"),
		)

		It("succeeds with nil env (default, no env vars)", func() {
			cfg := config.Config{
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
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

})
