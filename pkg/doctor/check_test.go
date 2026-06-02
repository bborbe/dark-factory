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

	It("returns an error when specs/in-progress/ does not exist", func() {
		// Don't create specs/in-progress/ — it won't exist.
		deps := doctor.Deps{
			SpecsInboxDir:        filepath.Join(specsDir, "inbox"),
			SpecsInProgressDir:   filepath.Join(tempDir, "does-not-exist"),
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
		_, err := checker.Check(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not a dark-factory project"))
		Expect(err.Error()).To(ContainSubstring("missing"))
	})
})
