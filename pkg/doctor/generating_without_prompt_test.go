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

var _ = Describe("GeneratingWithoutPrompt", func() {
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
		Expect(os.MkdirAll(filepath.Join(specsDir, "inbox"), 0750)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(specsDir, "in-progress"), 0750)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(specsDir, "completed"), 0750)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(specsDir, "rejected"), 0750)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(promptsDir, "inbox"), 0750)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(promptsDir, "in-progress"), 0750)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(promptsDir, "completed"), 0750)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(promptsDir, "cancelled"), 0750)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(promptsDir, "rejected"), 0750)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(tempDir)).To(Succeed())
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
			PromptsRejectedDir:   filepath.Join(promptsDir, "rejected"),
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

	generatingFindings := func() []doctor.Finding {
		findings, err := checkerFor().Check(ctx)
		Expect(err).NotTo(HaveOccurred())
		var out []doctor.Finding
		for _, f := range findings {
			if f.Category == doctor.CategoryGeneratingWithoutPrompt {
				out = append(out, f)
			}
		}
		return out
	}

	It("exposes the category constant as generating-without-prompt", func() {
		Expect(
			string(doctor.CategoryGeneratingWithoutPrompt),
		).To(Equal("generating-without-prompt"))
	})

	// The motivating shape: generation died, leaving the spec parked with
	// nothing on disk. Observed live when prompts were approved out from under a
	// still-running generator, which logged "generation produced no prompt
	// files" and rolled the spec back.
	It("reports a generating spec with no prompt anywhere", func() {
		createSpecFile(filepath.Join(specsDir, "in-progress"), "041-orphan.md", "generating")

		findings := generatingFindings()
		Expect(findings).To(HaveLen(1))
		Expect(findings[0].SpecID).To(Equal("041-orphan"))
		Expect(findings[0].Detail).To(ContainSubstring("no prompt references it"))
	})

	// The counterweight to the test above: without this, a detector that
	// reported EVERY generating spec would pass that one just as happily.
	It("does not report a generating spec that has a prompt", func() {
		createSpecFile(filepath.Join(specsDir, "in-progress"), "042-linked.md", "generating")
		createPromptFile(
			filepath.Join(promptsDir, "in-progress"),
			"500-work.md",
			"approved",
			"042-linked",
		)

		Expect(generatingFindings()).To(BeEmpty())
	})

	// Prompts that already ran still count as evidence generation succeeded —
	// scanning in-progress/ alone would resurrect a finished spec as a finding.
	It("does not report when the linked prompt already completed", func() {
		createSpecFile(filepath.Join(specsDir, "in-progress"), "043-done.md", "generating")
		createPromptFile(
			filepath.Join(promptsDir, "completed"),
			"501-shipped.md",
			"completed",
			"043-done",
		)

		Expect(generatingFindings()).To(BeEmpty())
	})

	// Every other status in specs/in-progress/ is legal and belongs to other
	// detectors; this one must claim only generating.
	It("ignores specs in other statuses", func() {
		dir := filepath.Join(specsDir, "in-progress")
		createSpecFile(dir, "044-approved.md", "approved")
		createSpecFile(dir, "045-prompted.md", "prompted")
		createSpecFile(dir, "046-draft.md", "draft")

		Expect(generatingFindings()).To(BeEmpty())
	})

	// The fix command must name commands that exist. This package has shipped
	// fix commands for `prompt unlink` and `spec renumber`, neither of which is
	// a real subcommand — a second dead end for an already-stuck operator.
	It("emits a fix command naming only real subcommands", func() {
		createSpecFile(filepath.Join(specsDir, "in-progress"), "047-orphan.md", "generating")

		findings := generatingFindings()
		Expect(findings).To(HaveLen(1))
		fix := findings[0].FixCommand
		Expect(fix).To(ContainSubstring("dark-factory spec unapprove 047-orphan"))
		Expect(fix).To(ContainSubstring("dark-factory spec approve 047-orphan"))
		Expect(fix).To(ContainSubstring("dark-factory spec mark-prompted 047-orphan"))
		Expect(fix).NotTo(ContainSubstring("spec renumber"))
		Expect(fix).NotTo(ContainSubstring("prompt unlink"))
	})

	// A clean tree must stay silent, or the daemon gate this feeds would refuse
	// every startup.
	It("reports nothing on a tree with no specs", func() {
		Expect(generatingFindings()).To(BeEmpty())
	})

	// The scan walks every spec and every prompt, so a cancelled context must
	// stop it rather than run the directories out. Asserting the error also
	// pins that cancellation surfaces instead of being swallowed into an empty
	// finding list, which would read to a caller as "pipeline clean".
	It("stops and returns the error when the context is cancelled", func() {
		createSpecFile(filepath.Join(specsDir, "in-progress"), "048-orphan.md", "generating")
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		findings, err := checkerFor().Check(cancelledCtx)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(context.Canceled))
		Expect(findings).To(BeEmpty())
	})
})
