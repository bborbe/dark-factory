// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor_test

import (
	"context"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/doctor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

var _ = Describe("MissingCompletedPrompt", func() {
	var (
		tempDir    string
		specsDir   string
		promptsDir string
		ctx        context.Context
		fakeMover  *mocks.FileMover
		pm         *prompt.Manager
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		specsDir = filepath.Join(tempDir, "specs")
		promptsDir = filepath.Join(tempDir, "prompts")
		ctx = context.Background()
		fakeMover = &mocks.FileMover{}
		fakeMover.MoveFileReturns(nil)
		pm = prompt.NewManager(
			filepath.Join(promptsDir, "inbox"),
			filepath.Join(promptsDir, "in-progress"),
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "cancelled"),
			fakeMover,
			libtime.NewCurrentDateTime(),
		)
		os.MkdirAll(filepath.Join(specsDir, "inbox"), 0750)
		os.MkdirAll(filepath.Join(specsDir, "in-progress"), 0750)
		os.MkdirAll(filepath.Join(specsDir, "completed"), 0750)
		os.MkdirAll(filepath.Join(specsDir, "rejected"), 0750)
		os.MkdirAll(filepath.Join(promptsDir, "inbox"), 0750)
		os.MkdirAll(filepath.Join(promptsDir, "in-progress"), 0750)
		os.MkdirAll(filepath.Join(promptsDir, "completed"), 0750)
		os.MkdirAll(filepath.Join(promptsDir, "cancelled"), 0750)
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	checkerFor := func() doctor.Checker {
		return doctor.NewChecker(doctor.Deps{
			SpecsInboxDir:        filepath.Join(specsDir, "inbox"),
			SpecsInProgressDir:   filepath.Join(specsDir, "in-progress"),
			SpecsCompletedDir:    filepath.Join(specsDir, "completed"),
			SpecsRejectedDir:     filepath.Join(specsDir, "rejected"),
			PromptsInboxDir:      filepath.Join(promptsDir, "inbox"),
			PromptsInProgressDir: filepath.Join(promptsDir, "in-progress"),
			PromptsCompletedDir:  filepath.Join(promptsDir, "completed"),
			PromptsCancelledDir:  filepath.Join(promptsDir, "cancelled"),
			SpecLister: spec.NewLister(
				libtime.NewCurrentDateTime(),
				filepath.Join(specsDir, "in-progress"),
				filepath.Join(specsDir, "completed"),
			),
			PromptManager:         pm,
			CurrentDateTimeGetter: libtime.NewCurrentDateTime(),
			VerifyingStaleHours:   24,
		})
	}

	// AC1: the constant exists and is what Check surfaces.
	It("exposes the category constant as missing-completed-prompt", func() {
		Expect(string(doctor.CategoryMissingCompletedPrompt)).To(Equal("missing-completed-prompt"))
	})

	// AC2: a number absent from completed/ while present in in-progress/.
	It("reports numbers present in in-progress but missing from completed", func() {
		completedDir := filepath.Join(promptsDir, "completed")
		inProgressDir := filepath.Join(promptsDir, "in-progress")
		createPromptFile(completedDir, "185-blocked.md", "completed", "")
		createPromptFile(completedDir, "188-later.md", "completed", "")
		createPromptFile(inProgressDir, "186-stuck.md", "failed", "")
		createPromptFile(inProgressDir, "187-queued.md", "approved", "")

		findings, err := checkerFor().Check(ctx)
		Expect(err).NotTo(HaveOccurred())

		categories := map[doctor.Category]bool{}
		for _, f := range findings {
			categories[f.Category] = true
		}
		Expect(categories).To(Equal(map[doctor.Category]bool{
			doctor.CategoryMissingCompletedPrompt: true,
		}))

		var missing []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryMissingCompletedPrompt {
				missing = append(missing, f)
			}
		}
		// Exactly two: 186 and 187. Numbers below the lowest number present
		// (1…184) are NOT reported.
		Expect(missing).To(HaveLen(2))
		Expect(missing[0].Detail).To(ContainSubstring("186"))
		Expect(missing[1].Detail).To(ContainSubstring("187"))
		Expect(missing[0].FixCommand).To(Equal("dark-factory prompt requeue 186"))
		Expect(missing[1].FixCommand).To(Equal("dark-factory prompt requeue 187"))
	})

	// AC3: a number absent from BOTH trees — the never-created gap.
	It("reports a number present in neither tree", func() {
		completedDir := filepath.Join(promptsDir, "completed")
		createPromptFile(completedDir, "185-blocked.md", "completed", "")
		createPromptFile(completedDir, "187-queued.md", "completed", "")
		createPromptFile(completedDir, "188-later.md", "completed", "")

		findings, err := checkerFor().Check(ctx)
		Expect(err).NotTo(HaveOccurred())

		categories := map[doctor.Category]bool{}
		for _, f := range findings {
			categories[f.Category] = true
		}
		Expect(categories).To(Equal(map[doctor.Category]bool{
			doctor.CategoryMissingCompletedPrompt: true,
		}))

		var missing []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryMissingCompletedPrompt {
				missing = append(missing, f)
			}
		}
		Expect(missing).To(HaveLen(1))
		Expect(missing[0].Detail).To(ContainSubstring("186"))
		Expect(missing[0].TargetPaths).To(BeEmpty())
		Expect(missing[0].FixCommand).To(ContainSubstring("no file for prompt number 186"))
	})

	// AC4: a contiguous tree produces no finding at all.
	It("reports nothing for a contiguous completed tree", func() {
		completedDir := filepath.Join(promptsDir, "completed")
		for _, name := range []string{
			"001-prompt.md", "002-prompt.md", "003-prompt.md", "004-prompt.md",
			"005-prompt.md", "006-prompt.md", "007-prompt.md", "008-prompt.md",
			"009-prompt.md", "010-prompt.md",
		} {
			createPromptFile(completedDir, name, "completed", "")
		}

		findings, err := checkerFor().Check(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(findings).To(BeEmpty())
	})

	It("does not report the highest number present, which is in flight", func() {
		createPromptFile(filepath.Join(promptsDir, "completed"), "001-prompt.md", "completed", "")
		createPromptFile(filepath.Join(promptsDir, "completed"), "002-prompt.md", "completed", "")
		createPromptFile(
			filepath.Join(promptsDir, "in-progress"),
			"003-running.md",
			"executing",
			"",
		)

		findings, err := checkerFor().Check(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(findings).To(BeEmpty())
	})

	It("reports nothing when every prompt directory is empty", func() {
		findings, err := checkerFor().Check(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(findings).To(BeEmpty())
	})
})
