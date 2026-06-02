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

var _ = Describe("PromptedNotSwept", func() {
	var (
		tempDir     string
		specsDir    string
		promptsDir  string
		ctx         context.Context
		fakeMover   *mocks.FileMover
		pm          *prompt.Manager
		currentTime = libtime.NewCurrentDateTime()
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
			currentTime,
		)
		os.MkdirAll(filepath.Join(specsDir, "inbox"), 0750)
		os.MkdirAll(filepath.Join(specsDir, "in-progress"), 0750)
		os.MkdirAll(filepath.Join(specsDir, "completed"), 0750)
		os.MkdirAll(filepath.Join(specsDir, "rejected"), 0750)
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	It("returns a finding when a spec in prompted status has all linked prompts completed", func() {
		// Create spec in prompted status.
		createSpecFile(filepath.Join(specsDir, "inbox"), "001-feature.md", "prompted")
		// Create a completed prompt linked to this spec.
		createPromptFile(filepath.Join(promptsDir, "completed"), "001-first.md", "completed", "001")

		deps := doctor.Deps{
			SpecsInboxDir:         filepath.Join(specsDir, "inbox"),
			SpecsInProgressDir:    filepath.Join(specsDir, "in-progress"),
			SpecsCompletedDir:     filepath.Join(specsDir, "completed"),
			SpecsRejectedDir:      filepath.Join(specsDir, "rejected"),
			PromptsInboxDir:       filepath.Join(promptsDir, "inbox"),
			PromptsInProgressDir:  filepath.Join(promptsDir, "in-progress"),
			PromptsCompletedDir:   filepath.Join(promptsDir, "completed"),
			PromptsCancelledDir:   filepath.Join(promptsDir, "cancelled"),
			SpecLister:            spec.NewLister(currentTime, filepath.Join(specsDir, "inbox")),
			PromptManager:         pm,
			CurrentDateTimeGetter: currentTime,
			VerifyingStaleHours:   24,
		}

		checker := doctor.NewChecker(deps)
		findings, err := checker.Check(ctx)
		Expect(err).NotTo(HaveOccurred())

		var sweptFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryPromptedNotSwept {
				sweptFindings = append(sweptFindings, f)
			}
		}
		Expect(sweptFindings).To(HaveLen(1))
		Expect(sweptFindings[0].Category).To(Equal(doctor.CategoryPromptedNotSwept))
		Expect(sweptFindings[0].FixCommand).To(ContainSubstring("dark-factory spec sweep"))
	})

	It("returns no finding when a spec in prompted status has a non-terminal prompt", func() {
		createSpecFile(filepath.Join(specsDir, "inbox"), "001-feature.md", "prompted")
		// Create a draft (non-terminal) prompt.
		createPromptFile(filepath.Join(promptsDir, "in-progress"), "001-first.md", "draft", "001")

		deps := doctor.Deps{
			SpecsInboxDir:         filepath.Join(specsDir, "inbox"),
			SpecsInProgressDir:    filepath.Join(specsDir, "in-progress"),
			SpecsCompletedDir:     filepath.Join(specsDir, "completed"),
			SpecsRejectedDir:      filepath.Join(specsDir, "rejected"),
			PromptsInboxDir:       filepath.Join(promptsDir, "inbox"),
			PromptsInProgressDir:  filepath.Join(promptsDir, "in-progress"),
			PromptsCompletedDir:   filepath.Join(promptsDir, "completed"),
			PromptsCancelledDir:   filepath.Join(promptsDir, "cancelled"),
			SpecLister:            spec.NewLister(currentTime, filepath.Join(specsDir, "inbox")),
			PromptManager:         pm,
			CurrentDateTimeGetter: currentTime,
			VerifyingStaleHours:   24,
		}

		checker := doctor.NewChecker(deps)
		findings, err := checker.Check(ctx)
		Expect(err).NotTo(HaveOccurred())

		var sweptFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryPromptedNotSwept {
				sweptFindings = append(sweptFindings, f)
			}
		}
		Expect(sweptFindings).To(BeEmpty())
	})

	It("does not fire on a prompted spec with zero linked prompts", func() {
		createSpecFile(filepath.Join(specsDir, "inbox"), "001-feature.md", "prompted")

		deps := doctor.Deps{
			SpecsInboxDir:         filepath.Join(specsDir, "inbox"),
			SpecsInProgressDir:    filepath.Join(specsDir, "in-progress"),
			SpecsCompletedDir:     filepath.Join(specsDir, "completed"),
			SpecsRejectedDir:      filepath.Join(specsDir, "rejected"),
			PromptsInboxDir:       filepath.Join(promptsDir, "inbox"),
			PromptsInProgressDir:  filepath.Join(promptsDir, "in-progress"),
			PromptsCompletedDir:   filepath.Join(promptsDir, "completed"),
			PromptsCancelledDir:   filepath.Join(promptsDir, "cancelled"),
			SpecLister:            spec.NewLister(currentTime, filepath.Join(specsDir, "inbox")),
			PromptManager:         pm,
			CurrentDateTimeGetter: currentTime,
			VerifyingStaleHours:   24,
		}

		checker := doctor.NewChecker(deps)
		findings, err := checker.Check(ctx)
		Expect(err).NotTo(HaveOccurred())

		var sweptFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryPromptedNotSwept {
				sweptFindings = append(sweptFindings, f)
			}
		}
		Expect(sweptFindings).To(BeEmpty())
	})

	It("returns no finding when a spec is not in prompted status", func() {
		createSpecFile(filepath.Join(specsDir, "inbox"), "001-feature.md", "idea")
		createPromptFile(filepath.Join(promptsDir, "completed"), "001-first.md", "completed", "001")

		deps := doctor.Deps{
			SpecsInboxDir:         filepath.Join(specsDir, "inbox"),
			SpecsInProgressDir:    filepath.Join(specsDir, "in-progress"),
			SpecsCompletedDir:     filepath.Join(specsDir, "completed"),
			SpecsRejectedDir:      filepath.Join(specsDir, "rejected"),
			PromptsInboxDir:       filepath.Join(promptsDir, "inbox"),
			PromptsInProgressDir:  filepath.Join(promptsDir, "in-progress"),
			PromptsCompletedDir:   filepath.Join(promptsDir, "completed"),
			PromptsCancelledDir:   filepath.Join(promptsDir, "cancelled"),
			SpecLister:            spec.NewLister(currentTime, filepath.Join(specsDir, "inbox")),
			PromptManager:         pm,
			CurrentDateTimeGetter: currentTime,
			VerifyingStaleHours:   24,
		}

		checker := doctor.NewChecker(deps)
		findings, err := checker.Check(ctx)
		Expect(err).NotTo(HaveOccurred())

		var sweptFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryPromptedNotSwept {
				sweptFindings = append(sweptFindings, f)
			}
		}
		Expect(sweptFindings).To(BeEmpty())
	})
})

func createPromptFile(dir, filename, status, specRef string) {
	path := filepath.Join(dir, filename)
	content := "---\nstatus: " + status + "\nspec: " + specRef + "\n---\n# Prompt"
	err := os.MkdirAll(dir, 0750)
	Expect(err).NotTo(HaveOccurred())
	err = os.WriteFile(path, []byte(content), 0600)
	if err != nil {
		panic(err)
	}
}
