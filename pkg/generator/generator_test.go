// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package generator_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/generator"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

var _ = Describe("SpecGenerator", func() {
	var (
		ctx              context.Context
		executor         *mocks.Executor
		containerChecker *mocks.ContainerChecker
		promptMgr        *mocks.GeneratorPromptManager
		inboxDir         string
		completedDir     string
		specsDir         string
		logDir           string
		sg               generator.SpecGenerator
		specPath         string
	)

	BeforeEach(func() {
		ctx = context.Background()
		executor = &mocks.Executor{}
		containerChecker = &mocks.ContainerChecker{}
		containerChecker.IsRunningReturns(false, nil)

		var err error
		inboxDir, err = os.MkdirTemp("", "generator-inbox-*")
		Expect(err).NotTo(HaveOccurred())

		completedDir, err = os.MkdirTemp("", "generator-completed-*")
		Expect(err).NotTo(HaveOccurred())

		specsDir, err = os.MkdirTemp("", "generator-specs-*")
		Expect(err).NotTo(HaveOccurred())

		logDir, err = os.MkdirTemp("", "generator-logs-*")
		Expect(err).NotTo(HaveOccurred())

		promptMgr = &mocks.GeneratorPromptManager{}
		realPM := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime())
		promptMgr.LoadStub = func(ctx context.Context, path string) (*prompt.PromptFile, error) {
			return realPM.Load(ctx, path)
		}

		sg = generator.NewSpecGenerator(
			executor,
			containerChecker,
			inboxDir,
			completedDir,
			specsDir,
			logDir,
			libtime.NewCurrentDateTime(),
			&mocks.SpecSlugMigrator{},
			"/dark-factory:generate-prompts-for-spec",
			"",
			0,
			promptMgr,
		)

		// Write a spec file with status "approved"
		specPath = filepath.Join(specsDir, "020-auto-prompt-generation.md")
		content := "---\nstatus: approved\n---\n# Spec\n\nDescription.\n"
		Expect(os.WriteFile(specPath, []byte(content), 0600)).To(Succeed())
	})

	AfterEach(func() {
		_ = os.RemoveAll(inboxDir)
		_ = os.RemoveAll(completedDir)
		_ = os.RemoveAll(specsDir)
		_ = os.RemoveAll(logDir)
	})

	Describe("Generate", func() {
		Context("success path: executor called, new file appears in inbox", func() {
			BeforeEach(func() {
				// Executor succeeds and creates a new file in inboxDir
				executor.ExecuteStub = func(ctx context.Context, promptContent, logFile, containerName string) error {
					// Simulate generating a prompt file
					return os.WriteFile(
						filepath.Join(inboxDir, "106-generated-prompt.md"),
						[]byte("# Generated"),
						0600,
					)
				}
			})

			It("calls executor with correct arguments", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())

				Expect(executor.ExecuteCallCount()).To(Equal(1))
				_, gotPrompt, gotLogFile, gotContainer := executor.ExecuteArgsForCall(0)
				Expect(gotPrompt).To(Equal("/dark-factory:generate-prompts-for-spec " + specPath))
				Expect(gotContainer).To(Equal("dark-factory-gen-020-auto-prompt-generation"))
				Expect(
					gotLogFile,
				).To(Equal(filepath.Join(logDir, "gen-020-auto-prompt-generation.log")))
			})

			It("sets spec status to prompted", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())

				sf, err := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
				Expect(err).NotTo(HaveOccurred())
				Expect(sf.Frontmatter.Status).To(Equal(string(spec.StatusPrompted)))
			})
		})

		Context("no files produced: executor succeeds but inbox unchanged", func() {
			BeforeEach(func() {
				executor.ExecuteStub = nil
				executor.ExecuteReturns(nil)
			})

			It("returns an error about no prompt files", func() {
				err := sg.Generate(ctx, specPath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("generation produced no prompt files"))
			})

			It("does not change the spec status", func() {
				_ = sg.Generate(ctx, specPath)

				sf, err := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
				Expect(err).NotTo(HaveOccurred())
				Expect(sf.Frontmatter.Status).To(Equal("approved"))
			})
		})

		Context("no files produced but completed prompts exist for spec", func() {
			BeforeEach(func() {
				executor.ExecuteStub = nil
				executor.ExecuteReturns(nil)

				// Write a completed prompt linked to "020-auto-prompt-generation" in completedDir
				content := "---\nstatus: completed\nspec: \"020-auto-prompt-generation\"\n---\n# Done\n"
				Expect(os.WriteFile(
					filepath.Join(completedDir, "020-some-prompt.md"),
					[]byte(content),
					0600,
				)).To(Succeed())
			})

			It("returns no error", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())
			})

			It("does not change the spec status", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())

				sf, err := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
				Expect(err).NotTo(HaveOccurred())
				Expect(sf.Frontmatter.Status).To(Equal("approved"))
			})
		})

		Context("no files produced and no completed prompts for spec", func() {
			BeforeEach(func() {
				executor.ExecuteStub = nil
				executor.ExecuteReturns(nil)

				// Write a completed prompt linked to a different spec
				content := "---\nstatus: completed\nspec: \"999-other-spec\"\n---\n# Other\n"
				Expect(os.WriteFile(
					filepath.Join(completedDir, "001-other.md"),
					[]byte(content),
					0600,
				)).To(Succeed())
			})

			It("returns an error about no prompt files", func() {
				err := sg.Generate(ctx, specPath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("generation produced no prompt files"))
			})
		})

		Context("spec transitions to generating before Execute is called", func() {
			BeforeEach(func() {
				// Executor checks the spec status mid-execution and creates a file
				executor.ExecuteStub = func(ctx context.Context, promptContent, logFile, containerName string) error {
					sf, err := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
					Expect(err).NotTo(HaveOccurred())
					Expect(sf.Frontmatter.Status).To(Equal(string(spec.StatusGenerating)))
					return os.WriteFile(
						filepath.Join(inboxDir, "116-mid-execution.md"),
						[]byte("# Mid"),
						0600,
					)
				}
			})

			It("has generating status on disk when Execute runs", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())
			})

			It("ends with prompted status after successful Execute", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())

				sf, err := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
				Expect(err).NotTo(HaveOccurred())
				Expect(sf.Frontmatter.Status).To(Equal(string(spec.StatusPrompted)))
			})
		})

		Context("executor error: executor returns an error", func() {
			BeforeEach(func() {
				executor.ExecuteReturns(errors.New("docker run failed"))
			})

			It("returns the executor error", func() {
				err := sg.Generate(ctx, specPath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("execute spec generator"))
			})

			It("resets spec to approved on non-cancellation error", func() {
				_ = sg.Generate(ctx, specPath)

				sf, err := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
				Expect(err).NotTo(HaveOccurred())
				Expect(sf.Frontmatter.Status).To(Equal("approved"))
			})
		})

		Context("executor error: context is cancelled", func() {
			BeforeEach(func() {
				executor.ExecuteStub = func(execCtx context.Context, promptContent, logFile, containerName string) error {
					return execCtx.Err()
				}
			})

			It("does NOT reset spec to approved when context is cancelled", func() {
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel() // cancel immediately

				_ = sg.Generate(cancelCtx, specPath)

				sf, err := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
				Expect(err).NotTo(HaveOccurred())
				// Spec should remain generating (not reset to approved)
				Expect(sf.Frontmatter.Status).To(Equal(string(spec.StatusGenerating)))
			})
		})

		Context("inbox dir does not exist before execution", func() {
			BeforeEach(func() {
				// Remove inbox dir so os.IsNotExist path is hit on first countMDFiles call
				Expect(os.RemoveAll(inboxDir)).To(Succeed())

				// Executor creates the inbox dir and a new file
				executor.ExecuteStub = func(ctx context.Context, promptContent, logFile, containerName string) error {
					Expect(os.MkdirAll(inboxDir, 0750)).To(Succeed())
					return os.WriteFile(
						filepath.Join(inboxDir, "107-new-prompt.md"),
						[]byte("# New"),
						0600,
					)
				}
			})

			It("succeeds and sets spec status to prompted", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())

				sf, err := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
				Expect(err).NotTo(HaveOccurred())
				Expect(sf.Frontmatter.Status).To(Equal(string(spec.StatusPrompted)))
			})
		})

		Context("spec file does not exist", func() {
			BeforeEach(func() {
				// Executor creates a new file in inbox
				executor.ExecuteStub = func(ctx context.Context, promptContent, logFile, containerName string) error {
					return os.WriteFile(
						filepath.Join(inboxDir, "108-new-prompt.md"),
						[]byte("# New"),
						0600,
					)
				}
			})

			It("returns error when spec cannot be loaded", func() {
				err := sg.Generate(ctx, filepath.Join(specsDir, "nonexistent-spec.md"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("load spec file"))
			})
		})

		Context("spec has branch and issue: new prompts inherit both", func() {
			BeforeEach(func() {
				// Rewrite spec with branch and issue
				content := "---\nstatus: approved\nbranch: dark-factory/spec-028\nissue: BRO-123\n---\n# Spec\n"
				Expect(os.WriteFile(specPath, []byte(content), 0600)).To(Succeed())

				executor.ExecuteStub = func(ctx context.Context, promptContent, logFile, containerName string) error {
					return os.WriteFile(
						filepath.Join(inboxDir, "109-inherited-prompt.md"),
						[]byte("---\nstatus: draft\n---\n# Inherited"),
						0600,
					)
				}
			})

			It("copies branch and issue into the generated prompt", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())

				pf, err := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
				Expect(err).NotTo(HaveOccurred())
				Expect(pf.Frontmatter.Status).To(Equal(string(spec.StatusPrompted)))

				data, err := os.ReadFile(filepath.Join(inboxDir, "109-inherited-prompt.md"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(data)).To(ContainSubstring("branch: dark-factory/spec-028"))
				Expect(string(data)).To(ContainSubstring("issue: BRO-123"))
			})
		})

		Context("generated prompt already has branch set: inherited branch not applied", func() {
			BeforeEach(func() {
				content := "---\nstatus: approved\nbranch: dark-factory/spec-028\nissue: BRO-123\n---\n# Spec\n"
				Expect(os.WriteFile(specPath, []byte(content), 0600)).To(Succeed())

				executor.ExecuteStub = func(ctx context.Context, promptContent, logFile, containerName string) error {
					return os.WriteFile(
						filepath.Join(inboxDir, "110-override-prompt.md"),
						[]byte("---\nstatus: draft\nbranch: my-override\n---\n# Override"),
						0600,
					)
				}
			})

			It("preserves the explicit branch on the generated prompt", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())

				data, err := os.ReadFile(filepath.Join(inboxDir, "110-override-prompt.md"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(data)).To(ContainSubstring("branch: my-override"))
				Expect(string(data)).NotTo(ContainSubstring("dark-factory/spec-028"))
				// Issue still inherited since prompt had none
				Expect(string(data)).To(ContainSubstring("issue: BRO-123"))
			})
		})

		Context("spec has no branch or issue: prompts left unmodified", func() {
			BeforeEach(func() {
				// specPath already has no branch/issue (just status: approved)
				executor.ExecuteStub = func(ctx context.Context, promptContent, logFile, containerName string) error {
					return os.WriteFile(
						filepath.Join(inboxDir, "111-no-inherit.md"),
						[]byte("---\nstatus: draft\n---\n# No inherit"),
						0600,
					)
				}
			})

			It("generates prompts without adding branch or issue", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())

				data, err := os.ReadFile(filepath.Join(inboxDir, "111-no-inherit.md"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(data)).NotTo(ContainSubstring("branch:"))
				Expect(string(data)).NotTo(ContainSubstring("issue:"))
			})
		})

		Context(
			"container is running: Reattach called, prompts found in inbox, spec set to prompted",
			func() {
				BeforeEach(func() {
					containerChecker.IsRunningReturns(true, nil)

					// Reattach creates a prompt file in inbox
					executor.ReattachStub = func(ctx context.Context, logFile, containerName string, _ time.Duration) error {
						return os.WriteFile(
							filepath.Join(inboxDir, "112-reattached-prompt.md"),
							[]byte(
								"---\nstatus: draft\nspec: \"020-auto-prompt-generation\"\n---\n# Reattached",
							),
							0600,
						)
					}
				})

				It("calls Reattach and not Execute", func() {
					Expect(sg.Generate(ctx, specPath)).To(Succeed())

					Expect(executor.ReattachCallCount()).To(Equal(1))
					Expect(executor.ExecuteCallCount()).To(Equal(0))
				})

				It("sets spec status to prompted", func() {
					Expect(sg.Generate(ctx, specPath)).To(Succeed())

					sf, err := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
					Expect(err).NotTo(HaveOccurred())
					Expect(sf.Frontmatter.Status).To(Equal(string(spec.StatusPrompted)))
				})
			},
		)

		Context(
			"container is running: Reattach called, no prompts found in inbox, returns nil",
			func() {
				BeforeEach(func() {
					containerChecker.IsRunningReturns(true, nil)
					executor.ReattachReturns(nil)
					// No prompt files written to inbox
				})

				It("returns nil without error", func() {
					Expect(sg.Generate(ctx, specPath)).To(Succeed())
				})

				It("does not change the spec status", func() {
					Expect(sg.Generate(ctx, specPath)).To(Succeed())

					sf, err := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
					Expect(err).NotTo(HaveOccurred())
					Expect(sf.Frontmatter.Status).To(Equal("approved"))
				})
			},
		)

		Context("container check returns error: falls back to fresh Execute", func() {
			BeforeEach(func() {
				containerChecker.IsRunningReturns(false, errors.New("docker inspect failed"))

				executor.ExecuteStub = func(ctx context.Context, promptContent, logFile, containerName string) error {
					return os.WriteFile(
						filepath.Join(inboxDir, "113-fallback-prompt.md"),
						[]byte("# Fallback"),
						0600,
					)
				}
			})

			It("calls Execute and not Reattach", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())

				Expect(executor.ExecuteCallCount()).To(Equal(1))
				Expect(executor.ReattachCallCount()).To(Equal(0))
			})

			It("sets spec status to prompted", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())

				sf, err := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
				Expect(err).NotTo(HaveOccurred())
				Expect(sf.Frontmatter.Status).To(Equal(string(spec.StatusPrompted)))
			})
		})

		Context("container is running: Reattach called, inbox dir does not exist", func() {
			BeforeEach(func() {
				containerChecker.IsRunningReturns(true, nil)
				executor.ReattachReturns(nil)
				// Remove inbox dir so non-existent path is hit in listPromptsForSpec
				Expect(os.RemoveAll(inboxDir)).To(Succeed())
			})

			It("returns nil without error", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())
			})
		})

		Context("container is running: Reattach returns error", func() {
			BeforeEach(func() {
				containerChecker.IsRunningReturns(true, nil)
				executor.ReattachReturns(errors.New("reattach failed"))
			})

			It("returns the reattach error", func() {
				err := sg.Generate(ctx, specPath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("reattach to spec generation container"))
			})
		})

		Context("container is running: inbox has unrelated prompt (spec mismatch)", func() {
			BeforeEach(func() {
				containerChecker.IsRunningReturns(true, nil)
				executor.ReattachReturns(nil)
				// Write a prompt linked to a different spec
				Expect(os.WriteFile(
					filepath.Join(inboxDir, "114-other-spec.md"),
					[]byte("---\nstatus: draft\nspec: \"999-other-spec\"\n---\n# Other"),
					0600,
				)).To(Succeed())
			})

			It("returns nil when no prompts match the spec", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())
			})

			It("does not change spec status", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())

				sf, err := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
				Expect(err).NotTo(HaveOccurred())
				Expect(sf.Frontmatter.Status).To(Equal("approved"))
			})
		})

		Context(
			"container is running: inbox has spec with branch and issue, prompts inherit",
			func() {
				BeforeEach(func() {
					// Rewrite spec with branch and issue
					content := "---\nstatus: approved\nbranch: dark-factory/spec-020\nissue: BRO-999\n---\n# Spec\n"
					Expect(os.WriteFile(specPath, []byte(content), 0600)).To(Succeed())

					containerChecker.IsRunningReturns(true, nil)
					executor.ReattachStub = func(ctx context.Context, logFile, containerName string, _ time.Duration) error {
						return os.WriteFile(
							filepath.Join(inboxDir, "115-reattach-inherit.md"),
							[]byte(
								"---\nstatus: draft\nspec: \"020-auto-prompt-generation\"\n---\n# Inherit",
							),
							0600,
						)
					}
				})

				It("inherits branch and issue into prompt and sets spec to prompted", func() {
					Expect(sg.Generate(ctx, specPath)).To(Succeed())

					data, err := os.ReadFile(filepath.Join(inboxDir, "115-reattach-inherit.md"))
					Expect(err).NotTo(HaveOccurred())
					Expect(string(data)).To(ContainSubstring("branch: dark-factory/spec-020"))
					Expect(string(data)).To(ContainSubstring("issue: BRO-999"))

					sf, err := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
					Expect(err).NotTo(HaveOccurred())
					Expect(sf.Frontmatter.Status).To(Equal(string(spec.StatusPrompted)))
				})
			},
		)
	})

	Describe("additionalInstructions", func() {
		Context("with non-empty additionalInstructions", func() {
			BeforeEach(func() {
				sg = generator.NewSpecGenerator(
					executor,
					containerChecker,
					inboxDir,
					completedDir,
					specsDir,
					logDir,
					libtime.NewCurrentDateTime(),
					&mocks.SpecSlugMigrator{},
					"/dark-factory:generate-prompts-for-spec",
					"Read /docs/guidelines.md before starting.",
					0,
					promptMgr,
				)
				executor.ExecuteStub = func(ctx context.Context, promptContent, logFile, containerName string) error {
					return os.WriteFile(
						filepath.Join(inboxDir, "120-additional-instructions.md"),
						[]byte("# Generated"),
						0600,
					)
				}
			})

			It("prepends additionalInstructions to the prompt content", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())

				Expect(executor.ExecuteCallCount()).To(Equal(1))
				_, gotPrompt, _, _ := executor.ExecuteArgsForCall(0)
				Expect(gotPrompt).To(HavePrefix("Read /docs/guidelines.md before starting.\n\n"))
				Expect(
					gotPrompt,
				).To(ContainSubstring("/dark-factory:generate-prompts-for-spec " + specPath))
			})
		})

		Context("with empty additionalInstructions", func() {
			It("does not prepend anything to the prompt content", func() {
				executor.ExecuteStub = func(ctx context.Context, promptContent, logFile, containerName string) error {
					return os.WriteFile(
						filepath.Join(inboxDir, "121-no-additional.md"),
						[]byte("# Generated"),
						0600,
					)
				}
				Expect(sg.Generate(ctx, specPath)).To(Succeed())

				_, gotPrompt, _, _ := executor.ExecuteArgsForCall(0)
				Expect(gotPrompt).To(Equal("/dark-factory:generate-prompts-for-spec " + specPath))
			})
		})
	})
})
