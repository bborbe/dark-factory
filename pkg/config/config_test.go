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

		It("fails for worktree workflow with migration error", func() {
			cfg := config.Config{
				Workflow: config.Workflow("worktree"),
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
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("removed"))
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

		It("succeeds for autoMerge true with workflow pr", func() {
			cfg := config.Config{
				Workflow: config.WorkflowPR,
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

		It("succeeds for autoMerge true with pr: true (no workflow field)", func() {
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
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails for autoMerge true with workflow worktree", func() {
			cfg := config.Config{
				Workflow: config.Workflow("worktree"),
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
			Expect(err.Error()).To(ContainSubstring("removed"))
		})

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
				Workflow: config.WorkflowPR,
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
				Workflow: config.WorkflowPR,
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
				Workflow: config.WorkflowPR,
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
				Workflow: config.WorkflowPR,
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

			It("succeeds for pr workflow", func() {
				err := config.WorkflowPR.Validate(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns removed error for worktree workflow", func() {
				err := config.Workflow("worktree").Validate(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("removed"))
			})

			It("fails for unknown workflow", func() {
				err := config.Workflow("unknown").Validate(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown workflow"))
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
				Expect(config.WorkflowPR.String()).To(Equal("pr"))
				Expect(config.Workflow("worktree").String()).To(Equal("worktree"))
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
				Expect(
					config.AvailableWorkflows.Contains(config.Workflow("worktree")),
				).To(BeFalse())
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
				Expect(cfg.Workflow).To(Equal(config.WorkflowPR))
				Expect(cfg.Prompts.InboxDir).To(Equal("custom-prompts"))
				Expect(cfg.Prompts.InProgressDir).To(Equal("custom-prompts/in-progress"))
				Expect(cfg.Prompts.CompletedDir).To(Equal("custom-prompts/done"))
				Expect(cfg.Prompts.LogDir).To(Equal("custom-prompts/logs"))
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
				Expect(cfg.Workflow).To(Equal(config.WorkflowPR))
				Expect(cfg.AutoMerge).To(BeTrue())
				Expect(cfg.AutoRelease).To(BeTrue())
			})

			It("maps workflow: pr to PR: true and Worktree: true via deprecation", func() {
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
				Expect(cfg.PR).To(BeTrue())
				Expect(cfg.Worktree).To(BeTrue())
			})

			It("maps workflow: direct to PR: false and Worktree: false via deprecation", func() {
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
				Expect(cfg.PR).To(BeFalse())
				Expect(cfg.Worktree).To(BeFalse())
			})

			It("loads pr: true and worktree: true without deprecation warning", func() {
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
				Expect(cfg.PR).To(BeTrue())
				Expect(cfg.Worktree).To(BeTrue())
			})

			It("returns error when both workflow and pr are set", func() {
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
				Expect(
					err.Error(),
				).To(ContainSubstring("cannot set both 'workflow' and 'pr'/'worktree'"))
			})

			It("returns error when both workflow and worktree are set", func() {
				configContent := `workflow: direct
worktree: false
`
				err := os.WriteFile(
					filepath.Join(tmpDir, ".dark-factory.yaml"),
					[]byte(configContent),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				_, err = loader.Load(ctx)
				Expect(err).To(HaveOccurred())
				Expect(
					err.Error(),
				).To(ContainSubstring("cannot set both 'workflow' and 'pr'/'worktree'"))
			})

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
    readonly: false
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
				Expect(cfg.ExtraMounts[0].IsReadonly()).To(BeTrue())
				Expect(cfg.ExtraMounts[1].Src).To(Equal("~/docs"))
				Expect(cfg.ExtraMounts[1].Dst).To(Equal("/docs"))
				Expect(cfg.ExtraMounts[1].IsReadonly()).To(BeFalse())
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
		It("returns true when Readonly is nil (default)", func() {
			m := config.ExtraMount{Src: "/src", Dst: "/dst"}
			Expect(m.IsReadonly()).To(BeTrue())
		})

		It("returns true when Readonly is explicitly true", func() {
			t := true
			m := config.ExtraMount{Src: "/src", Dst: "/dst", Readonly: &t}
			Expect(m.IsReadonly()).To(BeTrue())
		})

		It("returns false when Readonly is false", func() {
			f := false
			m := config.ExtraMount{Src: "/src", Dst: "/dst", Readonly: &f}
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
})
