// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/doctor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

var _ = Describe("VerifyingStale", func() {
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
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	It("returns a finding when verifying timestamp is empty", func() {
		createSpecFileWithVerifying(
			filepath.Join(specsDir, "inbox"),
			"001-feature.md",
			"verifying",
			"",
		)

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

		var staleFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryVerifyingStale {
				staleFindings = append(staleFindings, f)
			}
		}
		Expect(staleFindings).To(HaveLen(1))
		Expect(staleFindings[0].Detail).To(ContainSubstring("Verifying timestamp is empty"))
	})

	It("returns a finding when verifying timestamp is older than threshold", func() {
		oldTime := time.Now().Add(-48 * time.Hour).Format(time.RFC3339)
		createSpecFileWithVerifying(
			filepath.Join(specsDir, "inbox"),
			"001-feature.md",
			"verifying",
			oldTime,
		)

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

		var staleFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryVerifyingStale {
				staleFindings = append(staleFindings, f)
			}
		}
		Expect(staleFindings).To(HaveLen(1))
		Expect(staleFindings[0].FixCommand).To(ContainSubstring("dark-factory spec verify"))
	})

	It("returns no finding when verifying timestamp is recent", func() {
		recentTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
		createSpecFileWithVerifying(
			filepath.Join(specsDir, "inbox"),
			"001-feature.md",
			"verifying",
			recentTime,
		)

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

		var staleFindings []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryVerifyingStale {
				staleFindings = append(staleFindings, f)
			}
		}
		Expect(staleFindings).To(BeEmpty())
	})
})

func createSpecFileWithVerifying(dir, filename, status, verifying string) {
	path := filepath.Join(dir, filename)
	var content string
	if verifying != "" {
		content = "---\nstatus: " + status + "\nverifying: " + verifying + "\n---\n# Spec"
	} else {
		content = "---\nstatus: " + status + "\n---\n# Spec"
	}
	err := os.WriteFile(path, []byte(content), 0600)
	if err != nil {
		panic(err)
	}
}
