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

var _ = Describe("Check", func() {
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

	It("returns an empty slice (not nil) when there are no anomalies", func() {
		createSpecFile(filepath.Join(specsDir, "in-progress"), "001-feature.md", "idea")
		createSpecFile(filepath.Join(specsDir, "completed"), "002-feature.md", "completed")
		createPromptFile(filepath.Join(promptsDir, "in-progress"), "001-first.md", "approved", "")

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
			),
			PromptManager:         pm,
			CurrentDateTimeGetter: libtime.NewCurrentDateTime(),
			VerifyingStaleHours:   24,
		}

		checker := doctor.NewChecker(deps)
		findings, err := checker.Check(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(findings).NotTo(BeNil())
		Expect(findings).To(BeEmpty())
	})

	It("returns no findings and no error when no pipeline directories exist", func() {
		// A bare directory with a .dark-factory.yaml but no pipeline tree.
		// Check is a clean/dirty oracle: it must not hard-error here.
		bare := filepath.Join(tempDir, "bare")
		Expect(os.MkdirAll(bare, 0750)).To(Succeed())

		deps := doctor.Deps{
			SpecsInboxDir:        filepath.Join(bare, "specs", "inbox"),
			SpecsInProgressDir:   filepath.Join(bare, "specs", "in-progress"),
			SpecsCompletedDir:    filepath.Join(bare, "specs", "completed"),
			SpecsRejectedDir:     filepath.Join(bare, "specs", "rejected"),
			PromptsInboxDir:      filepath.Join(bare, "prompts", "inbox"),
			PromptsInProgressDir: filepath.Join(bare, "prompts", "in-progress"),
			PromptsCompletedDir:  filepath.Join(bare, "prompts", "completed"),
			PromptsCancelledDir:  filepath.Join(bare, "prompts", "cancelled"),
			SpecLister: spec.NewLister(
				libtime.NewCurrentDateTime(),
				filepath.Join(bare, "specs", "in-progress"),
				filepath.Join(bare, "specs", "completed"),
			),
			PromptManager:         pm,
			CurrentDateTimeGetter: libtime.NewCurrentDateTime(),
			VerifyingStaleHours:   24,
		}

		checker := doctor.NewChecker(deps)
		findings, err := checker.Check(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(findings).To(BeEmpty())
	})
})
