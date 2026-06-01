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

var _ = Describe("StatusDirMismatch", func() {
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

	It("returns a finding when a completed spec is in in-progress dir", func() {
		createSpecFile(filepath.Join(specsDir, "in-progress"), "001-feature.md", "completed")

		deps := doctor.Deps{
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
			),
			PromptManager:         pm,
			CurrentDateTimeGetter: libtime.NewCurrentDateTime(),
			VerifyingStaleHours:   24,
		}

		checker := doctor.NewChecker(deps)
		findings, err := checker.Check(ctx)
		Expect(err).NotTo(HaveOccurred())

		var mismatchFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryStatusDirMismatch {
				mismatchFindings = append(mismatchFindings, f)
			}
		}
		Expect(mismatchFindings).To(HaveLen(1))
		Expect(mismatchFindings[0].FixCommand).To(ContainSubstring("dark-factory spec move"))
	})

	It("returns a finding when a rejected spec is in completed dir", func() {
		createSpecFile(filepath.Join(specsDir, "completed"), "001-feature.md", "rejected")

		deps := doctor.Deps{
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
				filepath.Join(specsDir, "completed"),
			),
			PromptManager:         pm,
			CurrentDateTimeGetter: libtime.NewCurrentDateTime(),
			VerifyingStaleHours:   24,
		}

		checker := doctor.NewChecker(deps)
		findings, err := checker.Check(ctx)
		Expect(err).NotTo(HaveOccurred())

		var mismatchFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryStatusDirMismatch {
				mismatchFindings = append(mismatchFindings, f)
			}
		}
		Expect(mismatchFindings).To(HaveLen(1))
	})

	It("returns no finding when specs are in correct directories", func() {
		createSpecFile(filepath.Join(specsDir, "in-progress"), "001-feature.md", "idea")
		createSpecFile(filepath.Join(specsDir, "completed"), "002-feature.md", "completed")
		createSpecFile(filepath.Join(specsDir, "rejected"), "003-feature.md", "rejected")

		deps := doctor.Deps{
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
				filepath.Join(specsDir, "rejected"),
			),
			PromptManager:         pm,
			CurrentDateTimeGetter: libtime.NewCurrentDateTime(),
			VerifyingStaleHours:   24,
		}

		checker := doctor.NewChecker(deps)
		findings, err := checker.Check(ctx)
		Expect(err).NotTo(HaveOccurred())

		var mismatchFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryStatusDirMismatch {
				mismatchFindings = append(mismatchFindings, f)
			}
		}
		Expect(mismatchFindings).To(BeEmpty())
	})

	It("returns a finding when a prompt in completed/ has non-completed status", func() {
		createPromptFile(filepath.Join(promptsDir, "completed"), "001-prompt.md", "approved", "")

		deps := doctor.Deps{
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
				filepath.Join(specsDir, "inbox"),
			),
			PromptManager:         pm,
			CurrentDateTimeGetter: libtime.NewCurrentDateTime(),
			VerifyingStaleHours:   24,
		}

		checker := doctor.NewChecker(deps)
		findings, err := checker.Check(ctx)
		Expect(err).NotTo(HaveOccurred())

		var mismatchFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryStatusDirMismatch {
				mismatchFindings = append(mismatchFindings, f)
			}
		}
		Expect(mismatchFindings).To(HaveLen(1))
		Expect(mismatchFindings[0].FixCommand).To(ContainSubstring("dark-factory prompt move"))
	})

	It("returns a finding when a prompt in cancelled/ has non-cancelled status", func() {
		createPromptFile(filepath.Join(promptsDir, "cancelled"), "001-prompt.md", "idea", "")

		deps := doctor.Deps{
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
				filepath.Join(specsDir, "inbox"),
			),
			PromptManager:         pm,
			CurrentDateTimeGetter: libtime.NewCurrentDateTime(),
			VerifyingStaleHours:   24,
		}

		checker := doctor.NewChecker(deps)
		findings, err := checker.Check(ctx)
		Expect(err).NotTo(HaveOccurred())

		var mismatchFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryStatusDirMismatch {
				mismatchFindings = append(mismatchFindings, f)
			}
		}
		Expect(mismatchFindings).To(HaveLen(1))
	})
})
