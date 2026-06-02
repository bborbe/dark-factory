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

var _ = Describe("DuplicateSpecNumbers", func() {
	var (
		tempDir     string
		specsInbox  string
		ctx         context.Context
		fakeMover   *mocks.FileMover
		pm          *prompt.Manager
		currentTime = libtime.NewCurrentDateTime()
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		specsInbox = filepath.Join(tempDir, "specs", "inbox")
		err := os.MkdirAll(specsInbox, 0750)
		Expect(err).NotTo(HaveOccurred())

		ctx = context.Background()
		fakeMover = &mocks.FileMover{}
		fakeMover.MoveFileReturns(nil)

		pm = prompt.NewManager(
			filepath.Join(tempDir, "prompts", "inbox"),
			filepath.Join(tempDir, "prompts", "in-progress"),
			filepath.Join(tempDir, "prompts", "completed"),
			filepath.Join(tempDir, "prompts", "cancelled"),
			fakeMover,
			currentTime,
		)
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	It("returns no findings when there are no duplicate spec numbers", func() {
		// Create two specs with different numbers.
		createSpecFile(specsInbox, "001-first.md", "idea")
		createSpecFile(specsInbox, "002-second.md", "idea")

		deps := doctor.Deps{
			SpecsInboxDir:         specsInbox,
			SpecsInProgressDir:    filepath.Join(tempDir, "specs", "in-progress"),
			SpecsCompletedDir:     filepath.Join(tempDir, "specs", "completed"),
			SpecsRejectedDir:      filepath.Join(tempDir, "specs", "rejected"),
			PromptsInboxDir:       filepath.Join(tempDir, "prompts", "inbox"),
			PromptsInProgressDir:  filepath.Join(tempDir, "prompts", "in-progress"),
			PromptsCompletedDir:   filepath.Join(tempDir, "prompts", "completed"),
			PromptsCancelledDir:   filepath.Join(tempDir, "prompts", "cancelled"),
			SpecLister:            spec.NewLister(currentTime, specsInbox),
			PromptManager:         pm,
			CurrentDateTimeGetter: currentTime,
			VerifyingStaleHours:   24,
		}
		// Ensure spec dirs exist.
		os.MkdirAll(filepath.Join(tempDir, "specs", "in-progress"), 0750)

		checker := doctor.NewChecker(deps)
		findings, err := checker.Check(ctx)
		Expect(err).NotTo(HaveOccurred())

		var dupFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryDuplicateSpecNumbers {
				dupFindings = append(dupFindings, f)
			}
		}
		Expect(dupFindings).To(BeEmpty())
	})

	It("returns a finding when two spec files share the same numeric prefix", func() {
		// Create two specs with the same number 001.
		createSpecFile(specsInbox, "001-feature.md", "idea")
		createSpecFile(specsInbox, "001-other.md", "draft")

		deps := doctor.Deps{
			SpecsInboxDir:         specsInbox,
			SpecsInProgressDir:    filepath.Join(tempDir, "specs", "in-progress"),
			SpecsCompletedDir:     filepath.Join(tempDir, "specs", "completed"),
			SpecsRejectedDir:      filepath.Join(tempDir, "specs", "rejected"),
			PromptsInboxDir:       filepath.Join(tempDir, "prompts", "inbox"),
			PromptsInProgressDir:  filepath.Join(tempDir, "prompts", "in-progress"),
			PromptsCompletedDir:   filepath.Join(tempDir, "prompts", "completed"),
			PromptsCancelledDir:   filepath.Join(tempDir, "prompts", "cancelled"),
			SpecLister:            spec.NewLister(currentTime, specsInbox),
			PromptManager:         pm,
			CurrentDateTimeGetter: currentTime,
			VerifyingStaleHours:   24,
		}
		os.MkdirAll(filepath.Join(tempDir, "specs", "in-progress"), 0750)

		checker := doctor.NewChecker(deps)
		findings, err := checker.Check(ctx)
		Expect(err).NotTo(HaveOccurred())

		var dupFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryDuplicateSpecNumbers {
				dupFindings = append(dupFindings, f)
			}
		}
		Expect(dupFindings).To(HaveLen(1))
		Expect(dupFindings[0].Category).To(Equal(doctor.CategoryDuplicateSpecNumbers))
		Expect(dupFindings[0].FixCommand).To(ContainSubstring("dark-factory spec renumber"))
	})

	It("returns a finding for three colliding files with the later one as id-to-move", func() {
		createSpecFile(specsInbox, "001-first.md", "idea")
		createSpecFile(specsInbox, "001-second.md", "draft")
		createSpecFile(specsInbox, "001-third.md", "approved")

		deps := doctor.Deps{
			SpecsInboxDir:         specsInbox,
			SpecsInProgressDir:    filepath.Join(tempDir, "specs", "in-progress"),
			SpecsCompletedDir:     filepath.Join(tempDir, "specs", "completed"),
			SpecsRejectedDir:      filepath.Join(tempDir, "specs", "rejected"),
			PromptsInboxDir:       filepath.Join(tempDir, "prompts", "inbox"),
			PromptsInProgressDir:  filepath.Join(tempDir, "prompts", "in-progress"),
			PromptsCompletedDir:   filepath.Join(tempDir, "prompts", "completed"),
			PromptsCancelledDir:   filepath.Join(tempDir, "prompts", "cancelled"),
			SpecLister:            spec.NewLister(currentTime, specsInbox),
			PromptManager:         pm,
			CurrentDateTimeGetter: currentTime,
			VerifyingStaleHours:   24,
		}
		os.MkdirAll(filepath.Join(tempDir, "specs", "in-progress"), 0750)

		checker := doctor.NewChecker(deps)
		findings, err := checker.Check(ctx)
		Expect(err).NotTo(HaveOccurred())

		var dupFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryDuplicateSpecNumbers {
				dupFindings = append(dupFindings, f)
			}
		}
		Expect(dupFindings).To(HaveLen(1))
		// "001-third.md" is lex last → it's the one to renumber.
		Expect(dupFindings[0].FixCommand).To(ContainSubstring("001-third"))
	})
})

func createSpecFile(dir, filename, status string) {
	path := filepath.Join(dir, filename)
	content := "---\nstatus: " + status + "\n---\n# Spec"
	err := os.WriteFile(path, []byte(content), 0600)
	if err != nil {
		panic(err)
	}
}
