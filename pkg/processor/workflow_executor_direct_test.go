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
