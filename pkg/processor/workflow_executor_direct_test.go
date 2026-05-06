// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

// osFileMover is a simple FileMover for tests that uses os.Rename (no git).
type osFileMover struct{}

func (m *osFileMover) MoveFile(_ context.Context, oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

var _ = Describe("directWorkflowExecutor completeCommit autoRelease/CHANGELOG matrix", func() {
	type matrixCase struct {
		autoRelease          bool
		hasChangelog         bool
		wantPushBranch       int
		wantCommitAndRelease int
		wantCommitOnly       int
	}

	cases := []matrixCase{
		{
			autoRelease:          false,
			hasChangelog:         false,
			wantPushBranch:       0,
			wantCommitAndRelease: 0,
			wantCommitOnly:       1,
		},
		{
			autoRelease:          false,
			hasChangelog:         true,
			wantPushBranch:       0,
			wantCommitAndRelease: 0,
			wantCommitOnly:       1,
		},
		{
			autoRelease:          true,
			hasChangelog:         false,
			wantPushBranch:       1,
			wantCommitAndRelease: 0,
			wantCommitOnly:       1,
		},
		{
			autoRelease:          true,
			hasChangelog:         true,
			wantPushBranch:       1,
			wantCommitAndRelease: 1,
			wantCommitOnly:       0,
		},
	}

	for _, tc := range cases {
		tc := tc // capture range var
		desc := func() string {
			ar := "autoRelease=false"
			if tc.autoRelease {
				ar = "autoRelease=true"
			}
			cl := "no-changelog"
			if tc.hasChangelog {
				cl = "changelog"
			}
			return ar + " + " + cl
		}()
		It(desc, func() {
			ctx := context.Background()
			tempDir := GinkgoT().TempDir()
			queueDir := filepath.Join(tempDir, "in-progress")
			completedDirPath := filepath.Join(tempDir, "completed")
			Expect(os.MkdirAll(queueDir, 0750)).To(Succeed())
			Expect(os.MkdirAll(completedDirPath, 0750)).To(Succeed())

			promptPath := filepath.Join(queueDir, "001-test.md")
			Expect(
				os.WriteFile(promptPath, []byte("---\nstatus: committing\n---\n# Test\n"), 0600),
			).To(Succeed())
			completedPath := filepath.Join(completedDirPath, "001-test.md")

			promptMgr := prompt.NewManager(
				filepath.Join(tempDir, "inbox"),
				queueDir,
				completedDirPath,
				"",
				&osFileMover{},
				libtime.NewCurrentDateTime(),
			)
			rel := &stubWorkflowReleaser{hasChangelog: tc.hasChangelog}
			executor := NewDirectWorkflowExecutor(WorkflowDeps{
				PromptManager: promptMgr,
				AutoCompleter: &stubAutoCompleter{},
				Releaser:      rel,
				AutoRelease:   tc.autoRelease,
			})

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "committing"},
				[]byte("# Test\n"),
				libtime.NewCurrentDateTime(),
			)

			err := executor.Complete(ctx, ctx, pf, "test title", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(rel.pushBranchCount).To(Equal(tc.wantPushBranch), "PushBranch call count")
			Expect(
				rel.commitAndReleaseCount,
			).To(Equal(tc.wantCommitAndRelease), "CommitAndRelease call count")
			Expect(rel.commitOnlyCount).To(Equal(tc.wantCommitOnly), "CommitOnly call count")
		})
	}
})

// Regression: agent reports success but produces no diff — direct workflow must not crash.
var _ = Describe("directWorkflowExecutor no-diff success (regression)", func() {
	It("returns nil and moves prompt to completed when CommitOnly is a no-op", func() {
		ctx := context.Background()
		tempDir := GinkgoT().TempDir()
		queueDir := filepath.Join(tempDir, "in-progress")
		completedDirPath := filepath.Join(tempDir, "completed")
		Expect(os.MkdirAll(queueDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(completedDirPath, 0750)).To(Succeed())

		promptPath := filepath.Join(queueDir, "001-noop.md")
		Expect(
			os.WriteFile(promptPath, []byte("---\nstatus: committing\n---\n# Noop\n"), 0600),
		).To(Succeed())
		completedPath := filepath.Join(completedDirPath, "001-noop.md")

		promptMgr := prompt.NewManager(
			filepath.Join(tempDir, "inbox"),
			queueDir,
			completedDirPath,
			"",
			&osFileMover{},
			libtime.NewCurrentDateTime(),
		)
		// CommitOnly no-ops (returns nil) — simulates agent reporting success with no diff.
		rel := &stubWorkflowReleaser{commitOnlyErr: nil}
		executor := NewDirectWorkflowExecutor(WorkflowDeps{
			PromptManager: promptMgr,
			AutoCompleter: &stubAutoCompleter{},
			Releaser:      rel,
		})

		pf := prompt.NewPromptFile(
			promptPath,
			prompt.Frontmatter{Status: "committing"},
			[]byte("# Noop\n"),
			libtime.NewCurrentDateTime(),
		)

		err := executor.Complete(ctx, ctx, pf, "noop title", promptPath, completedPath)
		Expect(err).NotTo(HaveOccurred())
		// Prompt must have moved to completed/ — no crash despite no diff.
		Expect(completedPath).To(BeAnExistingFile())
		Expect(rel.commitOnlyCount).To(Equal(1))
		Expect(rel.commitAndReleaseCount).To(Equal(0))
	})
})

var _ = Describe("directWorkflowExecutor order-of-operations", func() {
	It(
		"transitions linked spec to verifying after the last prompt completes (regression: order-of-operations bug)",
		func() {
			ctx := context.Background()
			tempDir := GinkgoT().TempDir()

			// Set up directories
			queueDir := filepath.Join(tempDir, "prompts", "in-progress")
			completedDir := filepath.Join(tempDir, "prompts", "completed")
			specsInboxDir := filepath.Join(tempDir, "specs", "inbox")
			specsInProgressDir := filepath.Join(tempDir, "specs", "in-progress")
			specsCompletedDir := filepath.Join(tempDir, "specs", "completed")

			for _, dir := range []string{queueDir, completedDir, specsInboxDir, specsInProgressDir, specsCompletedDir} {
				Expect(os.MkdirAll(dir, 0750)).To(Succeed())
			}

			// Write a prompt file in in-progress/ with spec reference
			promptPath := filepath.Join(queueDir, "058-fix-spec.md")
			Expect(
				os.WriteFile(
					promptPath,
					[]byte("---\nstatus: committing\nspec: spec-058\n---\n# Fix spec\n"),
					0600,
				),
			).To(Succeed())

			// Write a spec file in specs/in-progress/ with status: prompted
			specPath := filepath.Join(specsInProgressDir, "spec-058.md")
			Expect(
				os.WriteFile(specPath, []byte("---\nstatus: prompted\n---\n# Spec 058\n"), 0600),
			).To(Succeed())

			// Build real PromptManager (uses real filesystem move)
			promptMgr := prompt.NewManager(
				filepath.Join(tempDir, "prompts", "inbox"),
				queueDir,
				completedDir,
				"",
				&osFileMover{},
				libtime.NewCurrentDateTime(),
			)

			// Build real AutoCompleter (reads the filesystem to check spec status)
			autoCompleter := spec.NewAutoCompleter(
				queueDir,
				completedDir,
				specsInboxDir,
				specsInProgressDir,
				specsCompletedDir,
				libtime.NewCurrentDateTime(),
				"",
				notifier.NewMultiNotifier(),
				prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime()),
			)

			// Build directWorkflowExecutor with real promptMgr and autoCompleter;
			// stub only the git/release deps.
			rel := &stubWorkflowReleaser{}
			executor := NewDirectWorkflowExecutor(WorkflowDeps{
				PromptManager: promptMgr,
				AutoCompleter: autoCompleter,
				Releaser:      rel,
			})

			// Build PromptFile matching the file on disk
			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "committing", Specs: prompt.SpecList{"spec-058"}},
				[]byte("# Fix spec\n"),
				libtime.NewCurrentDateTime(),
			)

			completedPath := filepath.Join(completedDir, "058-fix-spec.md")

			// Call Complete — this exercises the full completeCommit path including
			// the order-of-operations: MoveToCompleted must run before CheckAndComplete.
			err := executor.Complete(ctx, ctx, pf, "fix: spec-058", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred())

			// Prompt must now be in prompts/completed/
			Expect(completedPath).To(BeAnExistingFile())

			// Spec must have transitioned from "prompted" to "verifying".
			// Before the fix, CheckAndComplete ran before MoveToCompleted so it saw the
			// prompt still in in-progress and left the spec in "prompted".
			sf, loadErr := spec.Load(ctx, specPath, libtime.NewCurrentDateTime())
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(sf.Frontmatter.Status).To(Equal("verifying"),
				"spec should transition to verifying immediately after last prompt completes")
		},
	)
})
