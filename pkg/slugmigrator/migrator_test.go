// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slugmigrator_test

import (
	"context"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/slugmigrator"
)

var _ = Describe("Migrator", func() {
	var (
		ctx                   context.Context
		specsDir              string
		promptsDir            string
		currentDateTimeGetter libtime.CurrentDateTimeGetter
		promptManager         *prompt.Manager
		migrator              slugmigrator.Migrator
	)

	BeforeEach(func() {
		ctx = context.Background()
		currentDateTimeGetter = libtime.NewCurrentDateTime()

		var err error
		specsDir, err = os.MkdirTemp("", "slugmigrator-specs-*")
		Expect(err).To(BeNil())

		promptsDir, err = os.MkdirTemp("", "slugmigrator-prompts-*")
		Expect(err).To(BeNil())

		mover := &mocks.FileMover{}
		mover.MoveFileStub = func(ctx context.Context, src, dst string) error {
			return os.Rename(src, dst)
		}
		promptManager = prompt.NewManager(
			promptsDir,
			promptsDir,
			promptsDir,
			mover,
			currentDateTimeGetter,
		)

		migrator = slugmigrator.NewMigrator([]string{specsDir}, promptManager)
	})

	AfterEach(func() {
		_ = os.RemoveAll(specsDir)
		_ = os.RemoveAll(promptsDir)
	})

	writeSpec := func(dir, name string) {
		err := os.WriteFile(
			filepath.Join(dir, name),
			[]byte("---\nstatus: draft\n---\n# Spec\n"),
			0600,
		)
		Expect(err).To(BeNil())
	}

	writePrompt := func(dir, name string, specs []string) {
		pf := prompt.NewPromptFile(
			filepath.Join(dir, name),
			prompt.Frontmatter{
				Status: "draft",
				Specs:  prompt.SpecList(specs),
			},
			[]byte("# Prompt body\n"),
			currentDateTimeGetter,
		)
		err := pf.Save(ctx)
		Expect(err).To(BeNil())
	}

	loadSpecs := func(dir, name string) []string {
		pf, err := promptManager.Load(ctx, filepath.Join(dir, name))
		Expect(err).To(BeNil())
		return []string(pf.Frontmatter.Specs)
	}

	Describe("MigrateDirs", func() {
		Context("bare number ref with matching spec file", func() {
			It("replaces bare ref with full slug", func() {
				writeSpec(specsDir, "036-full-slug-spec-references.md")
				writePrompt(promptsDir, "100-my-prompt.md", []string{"036"})

				err := migrator.MigrateDirs(ctx, []string{promptsDir})
				Expect(err).To(BeNil())

				specs := loadSpecs(promptsDir, "100-my-prompt.md")
				Expect(specs).To(ConsistOf("036-full-slug-spec-references"))
			})
		})

		Context("file already using full slug", func() {
			It("leaves the ref unchanged (idempotent)", func() {
				writeSpec(specsDir, "036-full-slug-spec-references.md")
				writePrompt(
					promptsDir,
					"100-my-prompt.md",
					[]string{"036-full-slug-spec-references"},
				)

				err := migrator.MigrateDirs(ctx, []string{promptsDir})
				Expect(err).To(BeNil())

				specs := loadSpecs(promptsDir, "100-my-prompt.md")
				Expect(specs).To(ConsistOf("036-full-slug-spec-references"))
			})
		})

		Context("file with no spec field", func() {
			It("leaves the file unchanged with no error", func() {
				writePrompt(promptsDir, "100-my-prompt.md", []string{})

				err := migrator.MigrateDirs(ctx, []string{promptsDir})
				Expect(err).To(BeNil())

				specs := loadSpecs(promptsDir, "100-my-prompt.md")
				Expect(specs).To(BeEmpty())
			})
		})

		Context("bare number with no matching spec file", func() {
			It("leaves the ref unchanged with no error", func() {
				writePrompt(promptsDir, "100-my-prompt.md", []string{"099"})

				err := migrator.MigrateDirs(ctx, []string{promptsDir})
				Expect(err).To(BeNil())

				specs := loadSpecs(promptsDir, "100-my-prompt.md")
				Expect(specs).To(ConsistOf("099"))
			})
		})

		Context("two spec files with the same number (ambiguous)", func() {
			It("leaves bare ref unchanged with no error", func() {
				extraSpecsDir, err := os.MkdirTemp("", "slugmigrator-specs2-*")
				Expect(err).To(BeNil())
				defer func() { _ = os.RemoveAll(extraSpecsDir) }()

				writeSpec(specsDir, "036-version-one.md")
				writeSpec(extraSpecsDir, "036-version-two.md")

				ambiguousMover := &mocks.FileMover{}
				ambiguousMover.MoveFileStub = func(ctx context.Context, src, dst string) error {
					return os.Rename(src, dst)
				}
				ambiguousManager := prompt.NewManager(
					promptsDir,
					promptsDir,
					promptsDir,
					ambiguousMover,
					currentDateTimeGetter,
				)
				ambiguousMigrator := slugmigrator.NewMigrator(
					[]string{specsDir, extraSpecsDir},
					ambiguousManager,
				)

				writePrompt(promptsDir, "100-my-prompt.md", []string{"036"})

				err = ambiguousMigrator.MigrateDirs(ctx, []string{promptsDir})
				Expect(err).To(BeNil())

				specs := loadSpecs(promptsDir, "100-my-prompt.md")
				Expect(specs).To(ConsistOf("036"))
			})
		})

		Context("empty directory", func() {
			It("returns no error", func() {
				err := migrator.MigrateDirs(ctx, []string{promptsDir})
				Expect(err).To(BeNil())
			})
		})

		Context("non-existent directory", func() {
			It("returns no error", func() {
				err := migrator.MigrateDirs(ctx, []string{"/nonexistent/path/abc123"})
				Expect(err).To(BeNil())
			})
		})
	})
})
