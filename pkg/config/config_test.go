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
			Expect(cfg.QueueDir).To(Equal("prompts"))
			Expect(cfg.CompletedDir).To(Equal("prompts/completed"))
			Expect(cfg.ContainerImage).To(Equal("docker.io/bborbe/claude-yolo:v0.0.7"))
			Expect(cfg.DebounceMs).To(Equal(500))
			Expect(cfg.ServerPort).To(Equal(8080))
		})
	})

	Describe("Validate", func() {
		It("succeeds for valid config", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts",
				CompletedDir:   "prompts/completed",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.0.7",
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
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.0.7",
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
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.0.7",
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
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.0.7",
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
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.0.7",
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
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.0.7",
				DebounceMs:     500,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("completedDir"))
		})

		It("fails when completedDir equals queueDir", func() {
			cfg := config.Config{
				Workflow:       config.WorkflowDirect,
				InboxDir:       "prompts",
				QueueDir:       "prompts/queue",
				CompletedDir:   "prompts/queue",
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.0.7",
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
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.0.7",
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
				ContainerImage: "",
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
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.0.7",
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
				ContainerImage: "docker.io/bborbe/claude-yolo:v0.0.7",
				DebounceMs:     0,
				ServerPort:     8080,
			}
			err := cfg.Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("debounceMs"))
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
				Expect(cfg.QueueDir).To(Equal("prompts"))
				Expect(cfg.CompletedDir).To(Equal("prompts/completed"))
				Expect(cfg.ContainerImage).To(Equal("docker.io/bborbe/claude-yolo:v0.0.7"))
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
containerImage: docker.io/bborbe/claude-yolo:v0.0.7
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
containerImage: docker.io/bborbe/claude-yolo:v0.0.7
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
		})
	})
})
