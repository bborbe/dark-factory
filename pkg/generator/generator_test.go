// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package generator_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/generator"
	"github.com/bborbe/dark-factory/pkg/spec"
)

var _ = Describe("SpecGenerator", func() {
	var (
		ctx          context.Context
		mockExecutor *mocks.Executor
		inboxDir     string
		specsDir     string
		logDir       string
		sg           generator.SpecGenerator
		specPath     string
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockExecutor = &mocks.Executor{}

		var err error
		inboxDir, err = os.MkdirTemp("", "generator-inbox-*")
		Expect(err).NotTo(HaveOccurred())

		specsDir, err = os.MkdirTemp("", "generator-specs-*")
		Expect(err).NotTo(HaveOccurred())

		logDir, err = os.MkdirTemp("", "generator-logs-*")
		Expect(err).NotTo(HaveOccurred())

		sg = generator.NewSpecGenerator(mockExecutor, inboxDir, specsDir, logDir)

		// Write a spec file with status "approved"
		specPath = filepath.Join(specsDir, "020-auto-prompt-generation.md")
		content := "---\nstatus: approved\n---\n# Spec\n\nDescription.\n"
		Expect(os.WriteFile(specPath, []byte(content), 0600)).To(Succeed())
	})

	AfterEach(func() {
		_ = os.RemoveAll(inboxDir)
		_ = os.RemoveAll(specsDir)
		_ = os.RemoveAll(logDir)
	})

	Describe("Generate", func() {
		Context("success path: executor called, new file appears in inbox", func() {
			BeforeEach(func() {
				// Executor succeeds and creates a new file in inboxDir
				mockExecutor.ExecuteStub = func(ctx context.Context, promptContent, logFile, containerName string) error {
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

				Expect(mockExecutor.ExecuteCallCount()).To(Equal(1))
				_, gotPrompt, gotLogFile, gotContainer := mockExecutor.ExecuteArgsForCall(0)
				Expect(gotPrompt).To(Equal("/generate-prompts-for-spec " + specPath))
				Expect(gotContainer).To(Equal("dark-factory-gen-020-auto-prompt-generation"))
				Expect(
					gotLogFile,
				).To(Equal(filepath.Join(logDir, "gen-020-auto-prompt-generation.log")))
			})

			It("sets spec status to prompted", func() {
				Expect(sg.Generate(ctx, specPath)).To(Succeed())

				sf, err := spec.Load(ctx, specPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(sf.Frontmatter.Status).To(Equal(string(spec.StatusPrompted)))
			})
		})

		Context("no files produced: executor succeeds but inbox unchanged", func() {
			BeforeEach(func() {
				mockExecutor.ExecuteStub = nil
				mockExecutor.ExecuteReturns(nil)
			})

			It("returns an error about no prompt files", func() {
				err := sg.Generate(ctx, specPath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("generation produced no prompt files"))
			})

			It("does not change the spec status", func() {
				_ = sg.Generate(ctx, specPath)

				sf, err := spec.Load(ctx, specPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(sf.Frontmatter.Status).To(Equal("approved"))
			})
		})

		Context("executor error: executor returns an error", func() {
			BeforeEach(func() {
				mockExecutor.ExecuteReturns(errors.New("docker run failed"))
			})

			It("returns the executor error", func() {
				err := sg.Generate(ctx, specPath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("execute spec generator"))
			})

			It("does not change the spec status", func() {
				_ = sg.Generate(ctx, specPath)

				sf, err := spec.Load(ctx, specPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(sf.Frontmatter.Status).To(Equal("approved"))
			})
		})

		Context("inbox dir does not exist before execution", func() {
			BeforeEach(func() {
				// Remove inbox dir so os.IsNotExist path is hit on first countMDFiles call
				Expect(os.RemoveAll(inboxDir)).To(Succeed())

				// Executor creates the inbox dir and a new file
				mockExecutor.ExecuteStub = func(ctx context.Context, promptContent, logFile, containerName string) error {
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

				sf, err := spec.Load(ctx, specPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(sf.Frontmatter.Status).To(Equal(string(spec.StatusPrompted)))
			})
		})

		Context("spec file does not exist", func() {
			BeforeEach(func() {
				// Executor creates a new file in inbox
				mockExecutor.ExecuteStub = func(ctx context.Context, promptContent, logFile, containerName string) error {
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
	})
})
