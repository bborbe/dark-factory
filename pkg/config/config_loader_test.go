// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config_test

import (
	"context"
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

})
