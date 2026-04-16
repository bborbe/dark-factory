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

	Describe("Workflow", func() {
		Describe("Validate", func() {
			It("succeeds for direct workflow", func() {
				err := config.WorkflowDirect.Validate(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("fails for legacy pr workflow (not in AvailableWorkflows)", func() {
				err := config.WorkflowPR.Validate(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown workflow"))
			})

			It("succeeds for worktree workflow", func() {
				err := config.WorkflowWorktree.Validate(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("succeeds for branch workflow", func() {
				err := config.WorkflowBranch.Validate(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("succeeds for clone workflow", func() {
				err := config.WorkflowClone.Validate(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("fails for unknown workflow and lists valid values", func() {
				err := config.Workflow("unknown").Validate(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown workflow"))
				Expect(err.Error()).To(ContainSubstring("direct"))
				Expect(err.Error()).To(ContainSubstring("branch"))
				Expect(err.Error()).To(ContainSubstring("worktree"))
				Expect(err.Error()).To(ContainSubstring("clone"))
			})

			It("fails for empty string", func() {
				err := config.Workflow("").Validate(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown workflow"))
			})
		})

		Describe("String", func() {
			It("returns string representation", func() {
				Expect(config.WorkflowDirect.String()).To(Equal("direct"))
				Expect(config.WorkflowBranch.String()).To(Equal("branch"))
				Expect(config.WorkflowWorktree.String()).To(Equal("worktree"))
				Expect(config.WorkflowClone.String()).To(Equal("clone"))
				Expect(config.WorkflowPR.String()).To(Equal("pr"))
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
			It("returns true for all four valid workflows", func() {
				Expect(config.AvailableWorkflows.Contains(config.WorkflowDirect)).To(BeTrue())
				Expect(config.AvailableWorkflows.Contains(config.WorkflowBranch)).To(BeTrue())
				Expect(config.AvailableWorkflows.Contains(config.WorkflowWorktree)).To(BeTrue())
				Expect(config.AvailableWorkflows.Contains(config.WorkflowClone)).To(BeTrue())
			})

			It("returns false for legacy pr workflow", func() {
				Expect(config.AvailableWorkflows.Contains(config.WorkflowPR)).To(BeFalse())
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

			It("loads full config from file (workflow: pr maps to clone+pr)", func() {
				configContent := `workflow: pr
prompts:
  inboxDir: custom-prompts
  inProgressDir: custom-prompts/in-progress
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
				Expect(cfg.Workflow).To(Equal(config.WorkflowClone))
				Expect(cfg.PR).To(BeTrue())
				Expect(cfg.Prompts.InboxDir).To(Equal("custom-prompts"))
				Expect(cfg.Prompts.InProgressDir).To(Equal("custom-prompts/in-progress"))
				Expect(cfg.Prompts.CompletedDir).To(Equal("custom-prompts/done"))
				Expect(cfg.Prompts.LogDir).To(Equal("custom-prompts/logs"))
				Expect(cfg.ContainerImage).To(Equal("custom-image:latest"))
				Expect(cfg.DebounceMs).To(Equal(1000))
			})

			It("merges partial config with defaults (workflow: pr maps to clone+pr)", func() {
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
				Expect(cfg.Workflow).To(Equal(config.WorkflowClone))
				Expect(cfg.PR).To(BeTrue())
				Expect(cfg.Prompts.InboxDir).To(Equal("prompts"))
				Expect(cfg.Prompts.InProgressDir).To(Equal("prompts/in-progress"))
				Expect(cfg.Prompts.CompletedDir).To(Equal("prompts/completed"))
				Expect(cfg.Prompts.LogDir).To(Equal("prompts/log"))
				Expect(cfg.ContainerImage).To(Equal(pkg.DefaultContainerImage))
				Expect(cfg.DebounceMs).To(Equal(500))
			})

			It("loads defaultBranch from config", func() {
				configContent := `workflow: direct
defaultBranch: master
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.DefaultBranch).To(Equal("master"))
			})

			It("leaves defaultBranch empty when not set in config", func() {
				configContent := `workflow: direct
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.DefaultBranch).To(BeEmpty())
			})

			It("loads netrcFile from config when file exists", func() {
				netrcFile, err := os.CreateTemp("", "test-netrc-*")
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = os.Remove(netrcFile.Name()) }()
				_ = netrcFile.Close()

				configContent := "workflow: direct\nnetrcFile: " + netrcFile.Name() + "\n"
				err = os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.NetrcFile).To(Equal(netrcFile.Name()))
			})

			It("leaves netrcFile empty when not set in config", func() {
				configContent := `workflow: direct
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.NetrcFile).To(BeEmpty())
			})

			It("loads gitconfigFile from config when file exists", func() {
				gitconfigFile, err := os.CreateTemp("", "test-gitconfig-*")
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = os.Remove(gitconfigFile.Name()) }()
				_ = gitconfigFile.Close()

				configContent := "workflow: direct\ngitconfigFile: " + gitconfigFile.Name() + "\n"
				err = os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.GitconfigFile).To(Equal(gitconfigFile.Name()))
			})

			It("leaves gitconfigFile empty when not set in config", func() {
				configContent := `workflow: direct
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.GitconfigFile).To(BeEmpty())
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
containerImage: docker.io/bborbe/claude-yolo:v0.2.9
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
containerImage: docker.io/bborbe/claude-yolo:v0.2.9
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
				Expect(cfg.Workflow).To(Equal(config.WorkflowClone))
				Expect(cfg.PR).To(BeTrue())
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
				Expect(cfg.GitHub.Token).To(BeEmpty())
			})

			It("loads env map from config", func() {
				configContent := `workflow: direct
env:
  GOPRIVATE: "bitbucket.example.com/*"
  GONOSUMCHECK: "bitbucket.example.com/*"
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Env).To(Equal(map[string]string{
					"GOPRIVATE":    "bitbucket.example.com/*",
					"GONOSUMCHECK": "bitbucket.example.com/*",
				}))
			})

			It("leaves env nil when not set in config", func() {
				configContent := `workflow: direct
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Env).To(BeNil())
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
				Expect(cfg.Workflow).To(Equal(config.WorkflowClone))
				Expect(cfg.PR).To(BeTrue())
				Expect(cfg.AutoMerge).To(BeTrue())
				Expect(cfg.AutoRelease).To(BeTrue())
			})

			It("maps workflow: pr to workflow: clone, PR: true, Worktree: false", func() {
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
				Expect(cfg.Workflow).To(Equal(config.WorkflowClone))
				Expect(cfg.PR).To(BeTrue())
				Expect(cfg.Worktree).To(BeFalse())
			})

			It("maps workflow: direct to PR: false and Worktree: false", func() {
				configContent := `workflow: direct
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Workflow).To(Equal(config.WorkflowDirect))
				Expect(cfg.PR).To(BeFalse())
				Expect(cfg.Worktree).To(BeFalse())
			})

			It(
				"maps legacy worktree: true, pr: true to workflow: clone, pr: true, worktree: false",
				func() {
					configContent := `pr: true
worktree: true
`
					err := os.WriteFile(
						filepath.Join(tmpDir, ".dark-factory.yaml"),
						[]byte(configContent),
						0600,
					)
					Expect(err).NotTo(HaveOccurred())

					cfg, err := loader.Load(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(cfg.Workflow).To(Equal(config.WorkflowClone))
					Expect(cfg.PR).To(BeTrue())
					Expect(cfg.Worktree).To(BeFalse())
				},
			)

			It("fails when workflow: direct and pr: true (incompatible combination)", func() {
				configContent := `workflow: direct
pr: true
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				_, err = loader.Load(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("direct"))
				Expect(err.Error()).To(ContainSubstring("pr: true"))
			})

			It(
				"succeeds when workflow and worktree are both set (workflow wins, worktree ignored)",
				func() {
					configContent := `workflow: direct
worktree: false
`
					err := os.WriteFile(
						filepath.Join(tmpDir, ".dark-factory.yaml"),
						[]byte(configContent),
						0600,
					)
					Expect(err).NotTo(HaveOccurred())

					cfg, err := loader.Load(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(cfg.Workflow).To(Equal(config.WorkflowDirect))
					Expect(cfg.Worktree).To(BeFalse())
				},
			)

			It(
				"loads with PR: false and Worktree: false when neither workflow nor booleans set",
				func() {
					configContent := `containerImage: docker.io/bborbe/claude-yolo:latest
`
					err := os.WriteFile(
						filepath.Join(tmpDir, ".dark-factory.yaml"),
						[]byte(configContent),
						0600,
					)
					Expect(err).NotTo(HaveOccurred())

					cfg, err := loader.Load(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(cfg.PR).To(BeFalse())
					Expect(cfg.Worktree).To(BeFalse())
				},
			)

			It("loads extraMounts from config file", func() {
				configContent := `extraMounts:
  - src: /some/host/path
    dst: /container/path
  - src: ~/docs
    dst: /docs
    readOnly: true
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.ExtraMounts).To(HaveLen(2))
				Expect(cfg.ExtraMounts[0].Src).To(Equal("/some/host/path"))
				Expect(cfg.ExtraMounts[0].Dst).To(Equal("/container/path"))
				Expect(cfg.ExtraMounts[0].IsReadonly()).To(BeFalse())
				Expect(cfg.ExtraMounts[1].Src).To(Equal("~/docs"))
				Expect(cfg.ExtraMounts[1].Dst).To(Equal("/docs"))
				Expect(cfg.ExtraMounts[1].IsReadonly()).To(BeTrue())
			})

			It("leaves ExtraMounts nil when field is absent", func() {
				configContent := `containerImage: docker.io/bborbe/claude-yolo:latest
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.ExtraMounts).To(BeNil())
			})

			It("loads additionalInstructions from config file", func() {
				configContent := "additionalInstructions: |\n  Read /docs/guidelines.md before starting.\n  Follow conventions in /docs/go-testing-guide.md for all test code.\n"
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(
					cfg.AdditionalInstructions,
				).To(ContainSubstring("Read /docs/guidelines.md before starting."))
				Expect(
					cfg.AdditionalInstructions,
				).To(ContainSubstring("Follow conventions in /docs/go-testing-guide.md"))
			})

			It("leaves AdditionalInstructions empty when field is absent", func() {
				configContent := `containerImage: docker.io/bborbe/claude-yolo:latest
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.AdditionalInstructions).To(BeEmpty())
			})

			// If you add a new field to config.Config, you MUST:
			//   1. Add the field to partialConfig in loader.go
			//   2. Add the merge block to mergePartial
			//   3. Add the YAML value to fullYAML() below
			//   4. Add the Expect() assertion to assertFullConfig() below
			// The loader otherwise silently ignores the YAML key.
			Describe("loads every Config field from YAML", func() {
				var netrcFile string
				var gitconfigFile string

				BeforeEach(func() {
					// Create temp files needed for netrcFile / gitconfigFile validation.
					f, err := os.CreateTemp(tmpDir, "netrc-*")
					Expect(err).NotTo(HaveOccurred())
					_ = f.Close()
					netrcFile = f.Name()

					g, err := os.CreateTemp(tmpDir, "gitconfig-*")
					Expect(err).NotTo(HaveOccurred())
					_ = g.Close()
					gitconfigFile = g.Name()
				})

				fullYAML := func(netrc, gitconfig string) string {
					return `pr: true
worktree: true
projectName: test-projectname
defaultBranch: test-branch
prompts:
  inboxDir: test-prompts
  inProgressDir: test-prompts/in-progress
  completedDir: test-prompts/completed
  logDir: test-prompts/log
specs:
  inboxDir: test-specs
  inProgressDir: test-specs/in-progress
  completedDir: test-specs/completed
  logDir: test-specs/log
containerImage: test-image:latest
netrcFile: ` + netrc + `
gitconfigFile: ` + gitconfig + `
model: test-model
validationCommand: test-validate
validationPrompt: test-validation-prompt
testCommand: test-command
debounceMs: 750
serverPort: 9000
autoMerge: true
autoRelease: true
verificationGate: true
autoReview: true
maxReviewRetries: 5
allowedReviewers:
  - test-reviewer
useCollaborators: true
pollIntervalSec: 30
github:
  token: ${TEST_GITHUB_TOKEN}
provider: bitbucket-server
bitbucket:
  baseURL: https://bitbucket.example.com
  tokenEnv: TEST_BITBUCKET_TOKEN
notifications:
  telegram:
    botTokenEnv: TEST_BOT_TOKEN_ENV
    chatIDEnv: TEST_CHAT_ID_ENV
  discord:
    webhookEnv: TEST_WEBHOOK_ENV
env:
  GOPRIVATE: test-value
extraMounts:
  - src: /test-src
    dst: /test-dst
claudeDir: ~/.test-claude-dir
generateCommand: test-generate-command
additionalInstructions: test-instructions
maxContainers: 2
dirtyFileThreshold: 100
maxPromptDuration: 45m
autoRetryLimit: 2
`
				}

				assertFullConfig := func(cfg config.Config, netrc, gitconfig string) {
					// NOTE: Workflow is not tested in the full YAML because the legacy
					// worktree: true, pr: true pair maps to WorkflowClone at load time.
					// The legacy mapping (worktree: true, pr: true → workflow: clone) is tested separately.
					Expect(cfg.PR).To(BeTrue())
					Expect(cfg.Worktree).To(BeFalse())
					Expect(cfg.ProjectName).To(Equal("test-projectname"))
					Expect(cfg.DefaultBranch).To(Equal("test-branch"))
					Expect(cfg.Prompts.InboxDir).To(Equal("test-prompts"))
					Expect(cfg.Prompts.InProgressDir).To(Equal("test-prompts/in-progress"))
					Expect(cfg.Prompts.CompletedDir).To(Equal("test-prompts/completed"))
					Expect(cfg.Prompts.LogDir).To(Equal("test-prompts/log"))
					Expect(cfg.Specs.InboxDir).To(Equal("test-specs"))
					Expect(cfg.Specs.InProgressDir).To(Equal("test-specs/in-progress"))
					Expect(cfg.Specs.CompletedDir).To(Equal("test-specs/completed"))
					Expect(cfg.Specs.LogDir).To(Equal("test-specs/log"))
					Expect(cfg.ContainerImage).To(Equal("test-image:latest"))
					Expect(cfg.NetrcFile).To(Equal(netrc))
					Expect(cfg.GitconfigFile).To(Equal(gitconfig))
					Expect(cfg.Model).To(Equal("test-model"))
					Expect(cfg.ValidationCommand).To(Equal("test-validate"))
					Expect(cfg.ValidationPrompt).To(Equal("test-validation-prompt"))
					Expect(cfg.TestCommand).To(Equal("test-command"))
					Expect(cfg.DebounceMs).To(Equal(750))
					Expect(cfg.ServerPort).To(Equal(9000))
					Expect(cfg.AutoMerge).To(BeTrue())
					Expect(cfg.AutoRelease).To(BeTrue())
					Expect(cfg.VerificationGate).To(BeTrue())
					Expect(cfg.AutoReview).To(BeTrue())
					Expect(cfg.MaxReviewRetries).To(Equal(5))
					Expect(cfg.AllowedReviewers).To(Equal([]string{"test-reviewer"}))
					Expect(cfg.UseCollaborators).To(BeTrue())
					Expect(cfg.PollIntervalSec).To(Equal(30))
					Expect(cfg.GitHub.Token).To(Equal("${TEST_GITHUB_TOKEN}"))
					Expect(cfg.Provider).To(Equal(config.ProviderBitbucketServer))
					Expect(cfg.Bitbucket.BaseURL).To(Equal("https://bitbucket.example.com"))
					Expect(cfg.Bitbucket.TokenEnv).To(Equal("TEST_BITBUCKET_TOKEN"))
					Expect(cfg.Notifications.Telegram.BotTokenEnv).To(Equal("TEST_BOT_TOKEN_ENV"))
					Expect(cfg.Notifications.Telegram.ChatIDEnv).To(Equal("TEST_CHAT_ID_ENV"))
					Expect(cfg.Notifications.Discord.WebhookEnv).To(Equal("TEST_WEBHOOK_ENV"))
					Expect(cfg.Env).To(Equal(map[string]string{"GOPRIVATE": "test-value"}))
					Expect(cfg.ExtraMounts).To(HaveLen(1))
					Expect(cfg.ExtraMounts[0].Src).To(Equal("/test-src"))
					Expect(cfg.ExtraMounts[0].Dst).To(Equal("/test-dst"))
					Expect(cfg.ClaudeDir).To(Equal("~/.test-claude-dir"))
					Expect(cfg.GenerateCommand).To(Equal("test-generate-command"))
					Expect(cfg.AdditionalInstructions).To(Equal("test-instructions"))
					Expect(cfg.MaxContainers).To(Equal(2))
					Expect(cfg.DirtyFileThreshold).To(Equal(100))
					Expect(cfg.MaxPromptDuration).To(Equal("45m"))
					Expect(cfg.AutoRetryLimit).To(Equal(2))
				}

				It("round-trips every Config field through YAML", func() {
					err := os.WriteFile(
						filepath.Join(tmpDir, ".dark-factory.yaml"),
						[]byte(fullYAML(netrcFile, gitconfigFile)),
						0600,
					)
					Expect(err).NotTo(HaveOccurred())

					cfg, err := loader.Load(ctx)
					Expect(err).NotTo(HaveOccurred())
					assertFullConfig(cfg, netrcFile, gitconfigFile)
				})

				It("returns exactly Defaults() when YAML file is empty", func() {
					err := os.WriteFile(
						filepath.Join(tmpDir, ".dark-factory.yaml"),
						[]byte(""),
						0600,
					)
					Expect(err).NotTo(HaveOccurred())

					cfg, err := loader.Load(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(cfg).To(Equal(config.Defaults()))
				})

				It(
					"regression: maxPromptDuration, dirtyFileThreshold, autoRetryLimit round-trip",
					func() {
						// These three fields were previously silently dropped by the loader,
						// causing prompts to run past their configured timeout (billomat 009-015).
						configContent := `maxPromptDuration: 60m
dirtyFileThreshold: 500
autoRetryLimit: 3
`
						err := os.WriteFile(
							filepath.Join(tmpDir, ".dark-factory.yaml"),
							[]byte(configContent),
							0600,
						)
						Expect(err).NotTo(HaveOccurred())

						cfg, err := loader.Load(ctx)
						Expect(err).NotTo(HaveOccurred())
						Expect(cfg.MaxPromptDuration).To(Equal("60m"))
						Expect(cfg.DirtyFileThreshold).To(Equal(500))
						Expect(cfg.AutoRetryLimit).To(Equal(3))
						Expect(cfg.ParsedMaxPromptDuration()).To(Equal(60 * time.Minute))
					},
				)
			})
		})
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
})
