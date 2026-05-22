// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

// osFileMover is a simple FileMover for tests that uses os.Rename (no git).
type osFileMover struct{}

func (m *osFileMover) MoveFile(_ context.Context, oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// realGitReleaser implements git.Releaser using real git commands in a working directory.
type realGitReleaser struct {
	workDir      string
	pushErr      error // optional error to return on Push
	commitErr    error // optional error to return on CommitOnly
	hasChangelog bool
}

func (r *realGitReleaser) GetNextVersion(_ context.Context, _ git.VersionBump) (string, error) {
	return "v0.0.0", nil
}

func (r *realGitReleaser) CommitAndRelease(_ context.Context, _ git.VersionBump) error {
	if r.commitErr != nil {
		return r.commitErr
	}
	if err := runGitDirect(r.workDir, "add", "-A"); err != nil {
		return err
	}
	return runGitDirect(r.workDir, "commit", "-m", "release")
}

func (r *realGitReleaser) CommitCompletedFile(_ context.Context, _ string) error {
	return nil
}

func (r *realGitReleaser) CommitOnly(_ context.Context, title string) error {
	if r.commitErr != nil {
		return r.commitErr
	}
	if err := runGitDirect(r.workDir, "add", "-A"); err != nil {
		return err
	}
	return runGitDirect(r.workDir, "commit", "-m", title)
}

func (r *realGitReleaser) HasChangelog(_ context.Context) bool {
	return r.hasChangelog
}

func (r *realGitReleaser) MoveFile(_ context.Context, _, _ string) error {
	return nil
}

func (r *realGitReleaser) PushBranch(_ context.Context) error {
	if r.pushErr != nil {
		return r.pushErr
	}
	return runGitDirect(r.workDir, "push", "-u", "origin", "HEAD")
}

func (r *realGitReleaser) Push(_ context.Context, branch string) error {
	if r.pushErr != nil {
		return r.pushErr
	}
	return runGitDirect(r.workDir, "push", "origin", branch)
}

func runGitDirect(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// writePromptFile writes a prompt file with the given status.
func writePromptFile(path, status string) {
	content := "---\nstatus: " + status + "\n---\n# Test Prompt\n"
	Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
}

// writeFile writes content to a file.
func writeFile(path, content string) {
	Expect(os.WriteFile(path, []byte(content), 0644)).To(Succeed())
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readPromptStatus reads the status from a prompt file's frontmatter.
func readPromptStatus(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "status:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "status:"))
		}
	}
	return ""
}

// setupRealGitRepo creates a real git repo in a temp directory with an initial commit.
func setupRealGitRepo(t GinkgoTInterface) string {
	tempDir := t.TempDir()

	cmd := exec.Command("git", "init")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config user.email failed: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config user.name failed: %v", err)
	}

	if err := os.WriteFile(tempDir+"/.gitkeep", []byte("keep"), 0644); err != nil {
		t.Fatalf("write .gitkeep failed: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	return tempDir
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

var _ = Describe("directWorkflowExecutor moves prompt before commit", func() {
	It(
		"produces a single commit containing both code change and prompt rename in direct mode (move before commit)",
		func() {
			ctx := context.Background()
			repoDir := setupRealGitRepo(GinkgoT())

			// Create prompt and code directories
			promptsInProgress := repoDir + "/prompts/in-progress"
			promptsCompleted := repoDir + "/prompts/completed"
			Expect(os.MkdirAll(promptsInProgress, 0750)).To(Succeed())
			Expect(os.MkdirAll(promptsCompleted, 0750)).To(Succeed())

			promptPath := promptsInProgress + "/001-test.md"
			completedPath := promptsCompleted + "/001-test.md"
			codeFile := repoDir + "/code.go"

			// Write prompt file with "committing" status
			writePromptFile(promptPath, "committing")
			// Write code file
			writeFile(codeFile, "package main\n")

			// Commit the initial state (prompt at in-progress for rename detection)
			cmd := exec.CommandContext(ctx, "git", "add", ".")
			cmd.Dir = repoDir
			Expect(cmd.Run()).To(Succeed())
			cmd = exec.CommandContext(ctx, "git", "commit", "-m", "initial files")
			cmd.Dir = repoDir
			Expect(cmd.Run()).To(Succeed())

			// Create prompt manager with osFileMover
			promptMgr := prompt.NewManager(
				repoDir+"/prompts/inbox",
				promptsInProgress,
				promptsCompleted,
				"",
				&osFileMover{},
				libtime.NewCurrentDateTime(),
			)

			deps := WorkflowDeps{
				PromptManager: promptMgr,
				AutoCompleter: &stubAutoCompleter{},
				Releaser:      &realGitReleaser{workDir: repoDir},
				FileMover:     &osFileMover{},
				AutoRelease:   false,
			}
			executor := NewDirectWorkflowExecutor(deps)

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "committing"},
				[]byte("# Test\n"),
				libtime.NewCurrentDateTime(),
			)

			// Modify code file before complete
			writeFile(codeFile, "package main // modified\n")

			err := executor.Complete(ctx, ctx, pf, "test commit", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred())

			// Verify: prompt is at completed path, not at in-progress path.
			// `git ls-tree HEAD -- <path>` exits 0 with empty stdout when path doesn't match.
			cmd = exec.CommandContext(
				ctx,
				"git",
				"ls-tree",
				"HEAD",
				"--",
				"prompts/in-progress/001-test.md",
			)
			cmd.Dir = repoDir
			lsOut, err := cmd.CombinedOutput()
			Expect(err).To(BeNil())
			Expect(
				strings.TrimSpace(string(lsOut)),
			).To(BeEmpty(), "prompt should NOT be at in-progress path in HEAD")

			cmd = exec.CommandContext(
				ctx,
				"git",
				"ls-tree",
				"HEAD",
				"--",
				"prompts/completed/001-test.md",
			)
			cmd.Dir = repoDir
			lsOut, err = cmd.CombinedOutput()
			Expect(err).To(BeNil())
			Expect(
				strings.TrimSpace(string(lsOut)),
			).NotTo(BeEmpty(), "prompt should be at completed path in HEAD")

			// Verify: code.go was modified in the same commit
			cmd = exec.CommandContext(ctx, "git", "log", "-1", "--name-status", "--format=", "HEAD")
			cmd.Dir = repoDir
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("M\tcode.go"))
		},
	)
})

var _ = Describe("directWorkflowExecutor rollback on commit failure", func() {
	It(
		"rolls back the prompt to in-progress with status committing when the work commit fails after move",
		func() {
			ctx := context.Background()
			repoDir := setupRealGitRepo(GinkgoT())

			// Create prompt and code directories
			promptsInProgress := repoDir + "/prompts/in-progress"
			promptsCompleted := repoDir + "/prompts/completed"
			Expect(os.MkdirAll(promptsInProgress, 0750)).To(Succeed())
			Expect(os.MkdirAll(promptsCompleted, 0750)).To(Succeed())

			promptPath := promptsInProgress + "/001-test.md"
			completedPath := promptsCompleted + "/001-test.md"
			codeFile := repoDir + "/code.go"

			// Write prompt file
			writePromptFile(promptPath, "committing")
			// Write code file with invalid content to cause commit to fail
			writeFile(codeFile, "invalid go code {")

			// Create prompt manager with osFileMover
			promptMgr := prompt.NewManager(
				repoDir+"/prompts/inbox",
				promptsInProgress,
				promptsCompleted,
				"",
				&osFileMover{},
				libtime.NewCurrentDateTime(),
			)

			// Simulated commit failure
			rel := &realGitReleaser{
				workDir:   repoDir,
				commitErr: errors.New("simulated commit failure"),
			}

			deps := WorkflowDeps{
				PromptManager: promptMgr,
				AutoCompleter: &stubAutoCompleter{},
				Releaser:      rel,
				FileMover:     &osFileMover{},
				AutoRelease:   false,
			}
			executor := NewDirectWorkflowExecutor(deps)

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "committing"},
				[]byte("# Test\n"),
				libtime.NewCurrentDateTime(),
			)

			// Complete returns nil even on failure (by design - to avoid daemon crashes)
			// The rollback happens internally
			err := executor.Complete(ctx, ctx, pf, "test commit", promptPath, completedPath)
			Expect(err).To(BeNil())

			// Assert: file at completedPath does NOT exist (rollback worked)
			Expect(fileExists(completedPath)).To(BeFalse())
			// Assert: file at promptPath DOES exist (rollback worked)
			Expect(fileExists(promptPath)).To(BeTrue())
			// Assert: frontmatter status is "committing"
			Expect(readPromptStatus(promptPath)).To(Equal("committing"))
		},
	)
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
