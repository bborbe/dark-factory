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

var _ = Describe("ParseErrors", func() {
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

	It("emits a finding for a spec file with invalid YAML frontmatter", func() {
		path := filepath.Join(specsDir, "inbox", "001-feature.md")
		err := os.WriteFile(path, []byte("---\nstatus: [invalid\n---\n# Spec"), 0600)
		Expect(err).NotTo(HaveOccurred())

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

		var parseFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryParseError {
				parseFindings = append(parseFindings, f)
			}
		}
		Expect(parseFindings).To(HaveLen(1))
		Expect(parseFindings[0].Category).To(Equal(doctor.CategoryParseError))
		Expect(parseFindings[0].FixCommand).To(ContainSubstring("Fix the YAML by hand"))
	})

	It("continues scanning after a parse error and emits findings for other files", func() {
		// One bad spec, one good spec.
		badPath := filepath.Join(specsDir, "inbox", "001-bad.md")
		err := os.WriteFile(badPath, []byte("---\nstatus: [invalid\n---\n# Spec"), 0600)
		Expect(err).NotTo(HaveOccurred())
		createSpecFile(filepath.Join(specsDir, "inbox"), "002-good.md", "idea")

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

		var parseFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryParseError {
				parseFindings = append(parseFindings, f)
			}
		}
		// Only the bad file should be reported.
		Expect(parseFindings).To(HaveLen(1))
		Expect(parseFindings[0].TargetPaths[0]).To(ContainSubstring("001-bad.md"))
	})

	It("emits a finding for a prompt file with invalid YAML frontmatter", func() {
		path := filepath.Join(promptsDir, "inbox", "001-prompt.md")
		err := os.WriteFile(path, []byte("---\nstatus: [invalid\n---\n# Prompt"), 0600)
		Expect(err).NotTo(HaveOccurred())

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

		var parseFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryParseError {
				parseFindings = append(parseFindings, f)
			}
		}
		Expect(parseFindings).To(HaveLen(1))
		Expect(parseFindings[0].TargetPaths[0]).To(ContainSubstring("001-prompt.md"))
	})
})
