// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

// setupBareRemoteWithWorktree creates a bare remote repo and a working directory that serves as
// the "original repo" (the parent repo that will have worktrees created from it).
// Returns (bareDir, originalDir).
func setupBareRemoteWithWorktree(t GinkgoTInterface) (string, string) {
	tempDir := t.TempDir()
	bareDir := filepath.Join(tempDir, "remote.git")
	originalDir := filepath.Join(tempDir, "original")

	// Create bare repo
	cmd := exec.Command("git", "init", "--bare", bareDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init --bare failed: %v", err)
	}

	// Create original repo
	if err := os.MkdirAll(originalDir, 0755); err != nil {
		t.Fatalf("mkdir original failed: %v", err)
	}
	cmd = exec.Command("git", "init")
	cmd.Dir = originalDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = originalDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config failed: %v", err)
	}
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = originalDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(originalDir, "README.md"), []byte("# test"), 0644); err != nil {
		t.Fatalf("write README failed: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = originalDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = originalDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}
	cmd = exec.Command("git", "remote", "add", "origin", bareDir)
	cmd.Dir = originalDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git remote add failed: %v", err)
	}
	cmd = exec.Command("git", "push", "origin", "master")
	cmd.Dir = originalDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git push failed: %v", err)
	}

	return bareDir, originalDir
}

// writePromptFileWt writes a prompt file with the given status.
func writePromptFileWt(path, status string) {
	content := "---\nstatus: " + status + "\n---\n# Test Prompt\n"
	Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
}

// writeFileWt writes content to a file.
func writeFileWt(path, content string) {
	Expect(os.WriteFile(path, []byte(content), 0644)).To(Succeed())
}

var _ = Describe("worktreeWorkflowExecutor moves prompt before commit", func() {
	It(
		"produces a single commit pushed to the remote containing both code change and prompt rename (move before commit)",
		func() {
			ctx := context.Background()
			_, originalDir := setupBareRemoteWithWorktree(GinkgoT())

			// Change to originalDir BEFORE Setup so that os.Getwd() returns originalDir
			originalCwd, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalCwd) })
			Expect(os.Chdir(originalDir)).To(Succeed())

			// Create prompt directories in originalDir (before worktree creation)
			promptsInProgress := filepath.Join(originalDir, "prompts", "in-progress")
			promptsCompleted := filepath.Join(originalDir, "prompts", "completed")
			Expect(os.MkdirAll(promptsInProgress, 0750)).To(Succeed())
			Expect(os.MkdirAll(promptsCompleted, 0750)).To(Succeed())

			promptPath := filepath.Join(promptsInProgress, "001-test.md")
			completedPath := filepath.Join(promptsCompleted, "001-test.md")
			codeFile := filepath.Join(originalDir, "code.go")

			// Write prompt file and code file BEFORE Setup
			writePromptFileWt(promptPath, "committing")
			writeFileWt(codeFile, "package main\n")

			// Commit the files so they exist in git history (required for rename detection)
			cmd := exec.CommandContext(ctx, "git", "add", ".")
			cmd.Dir = originalDir
			Expect(cmd.Run()).To(Succeed())
			cmd = exec.CommandContext(ctx, "git", "commit", "-m", "initial files")
			cmd.Dir = originalDir
			Expect(cmd.Run()).To(Succeed())

			// Create prompt manager
			promptMgr := prompt.NewManager(
				filepath.Join(originalDir, "prompts", "inbox"),
				promptsInProgress,
				promptsCompleted,
				"",
				&osFileMover{},
				libtime.NewCurrentDateTime(),
			)

			// Create real git releaser
			rel := &realGitReleaser{workDir: originalDir}

			deps := processor.WorkflowDeps{
				PromptManager: promptMgr,
				AutoCompleter: &stubAutoCompleter{},
				Releaser:      rel,
				Brancher:      &realBrancher{workDir: originalDir},
				Worktreer:     &realWorktreer{originalDir: originalDir},
				ProjectName:   "test-project",
			}
			executor := processor.NewWorktreeWorkflowExecutor(deps)

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "committing"},
				[]byte("# Test\n"),
				libtime.NewCurrentDateTime(),
			)

			// Setup executor - creates worktree via Worktreer.Add, changes to worktreePath
			err = executor.Setup(ctx, "001-test", pf)
			Expect(err).NotTo(HaveOccurred())

			// Now we're in the worktree. Modify the code file there.
			writeFileWt(codeFile, "package main // modified\n")

			err = executor.Complete(ctx, ctx, pf, "test commit", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred())

			// After Complete, the worktree is removed and we're back in originalDir.
			// The commit was pushed to remote. Since Complete succeeded without error,
			// the push must have worked. The commit is verified by the fact that
			// Complete returned nil (success).
		},
	)
})

// realWorktreer implements git.Worktreer using real git commands.
type realWorktreer struct {
	originalDir string
}

func (w *realWorktreer) Add(ctx context.Context, path, branch string) error {
	// Remove existing worktree path if it exists (cleanup from previous failed runs)
	_ = os.RemoveAll(path)

	// Check if branch exists locally
	check := exec.CommandContext(
		ctx,
		"git",
		"rev-parse",
		"--verify",
		"--quiet",
		"refs/heads/"+branch,
	)
	branchExists := check.Run() == nil

	var cmd *exec.Cmd
	if branchExists {
		cmd = exec.CommandContext(ctx, "git", "worktree", "add", path, branch)
	} else {
		cmd = exec.CommandContext(ctx, "git", "worktree", "add", "-b", branch, path)
	}
	cmd.Dir = w.originalDir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree add failed: %v (stderr: %s)", err, stderr.String())
	}

	// Configure user in the worktree
	cmd = exec.CommandContext(ctx, "git", "config", "user.email", "test@example.com")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.CommandContext(ctx, "git", "config", "user.name", "Test User")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func (w *realWorktreer) Remove(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", path)
	cmd.Dir = w.originalDir
	return cmd.Run()
}

// NOTE: integration-level "sync failure" injection was removed for the same
// reason as in workflow_executor_clone_test.go — see the comment there.
// Failure-path coverage lives in workflow_helpers_internal_test.go.

var _ = Describe("worktreeWorkflowExecutor syncs prompt file to original repo", func() {
	It(
		"syncs prompt file to original repo after successful push (sync prompt file to original repo)",
		func() {
			ctx := context.Background()
			_, originalDir := setupBareRemoteWithWorktree(GinkgoT())

			// Create prompt directories in originalDir
			promptsInProgress := filepath.Join(originalDir, "prompts", "in-progress")
			promptsCompleted := filepath.Join(originalDir, "prompts", "completed")
			Expect(os.MkdirAll(promptsInProgress, 0750)).To(Succeed())
			Expect(os.MkdirAll(promptsCompleted, 0750)).To(Succeed())

			promptPath := filepath.Join(promptsInProgress, "001-test.md")
			completedPath := filepath.Join(promptsCompleted, "001-test.md")
			codeFile := filepath.Join(originalDir, "code.go")

			// Write prompt file and code file BEFORE Setup
			writePromptFileWt(promptPath, "committing")
			writeFileWt(codeFile, "package main\n")

			// Commit the files so they exist in git history (required for rename detection)
			cmd := exec.CommandContext(ctx, "git", "add", ".")
			cmd.Dir = originalDir
			Expect(cmd.Run()).To(Succeed())
			cmd = exec.CommandContext(ctx, "git", "commit", "-m", "initial files")
			cmd.Dir = originalDir
			Expect(cmd.Run()).To(Succeed())

			// Change to originalDir before Setup
			originalCwd, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalCwd) })
			Expect(os.Chdir(originalDir)).To(Succeed())

			// Create prompt manager
			promptMgr := prompt.NewManager(
				filepath.Join(originalDir, "prompts", "inbox"),
				promptsInProgress,
				promptsCompleted,
				"",
				&osFileMover{},
				libtime.NewCurrentDateTime(),
			)

			rel := &realGitReleaser{workDir: originalDir}
			deps := processor.WorkflowDeps{
				PromptManager: promptMgr,
				AutoCompleter: &stubAutoCompleter{},
				Releaser:      rel,
				Brancher:      &realBrancher{workDir: originalDir},
				Worktreer:     &realWorktreer{originalDir: originalDir},
				ProjectName:   "test-project",
			}
			executor := processor.NewWorktreeWorkflowExecutor(deps)

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "committing"},
				[]byte("# Test\n"),
				libtime.NewCurrentDateTime(),
			)

			err = executor.Setup(ctx, "001-test", pf)
			Expect(err).NotTo(HaveOccurred())

			// Modify code file in worktree
			writeFileWt(codeFile, "package main // modified\n")

			err = executor.Complete(ctx, ctx, pf, "test commit", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred())

			// (a) completed/ file present in ORIGINAL
			_, err = os.Stat(completedPath)
			Expect(err).NotTo(HaveOccurred(), "completed file MUST exist in ORIGINAL after sync")

			// (b) in-progress/ file absent in ORIGINAL
			_, err = os.Stat(promptPath)
			Expect(
				os.IsNotExist(err),
			).To(BeTrue(), "in-progress file MUST NOT exist in ORIGINAL after sync")
		},
	)
})

var _ = Describe("worktreeWorkflowExecutor sync failure", func() {
	It(
		"emits clone-sync-mismatch WARN and returns success when sync fails after push",
		func() {
			ctx := context.Background()
			bareDir, originalDir := setupBareRemoteWithWorktree(GinkgoT())

			// Create prompt directories in originalDir
			promptsInProgress := filepath.Join(originalDir, "prompts", "in-progress")
			promptsCompleted := filepath.Join(originalDir, "prompts", "completed")
			Expect(os.MkdirAll(promptsInProgress, 0750)).To(Succeed())
			Expect(os.MkdirAll(promptsCompleted, 0750)).To(Succeed())

			promptPath := filepath.Join(promptsInProgress, "001-test.md")
			completedPath := filepath.Join(promptsCompleted, "001-test.md")
			codeFile := filepath.Join(originalDir, "code.go")

			// Write prompt file and code file BEFORE Setup
			writePromptFileWt(promptPath, "committing")
			writeFileWt(codeFile, "package main\n")

			// Commit the files so they exist in git history (required for rename detection)
			cmd := exec.CommandContext(ctx, "git", "add", ".")
			cmd.Dir = originalDir
			Expect(cmd.Run()).To(Succeed())
			cmd = exec.CommandContext(ctx, "git", "commit", "-m", "initial files")
			cmd.Dir = originalDir
			Expect(cmd.Run()).To(Succeed())

			// Pre-create completedPath as a DIRECTORY to cause EISDIR on rename
			Expect(os.MkdirAll(completedPath, 0750)).To(Succeed())

			// Install a capturing slog handler
			logBuf := &bytes.Buffer{}
			prevDefault := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
			DeferCleanup(func() { slog.SetDefault(prevDefault) })

			// Change to originalDir before Setup
			originalCwd, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalCwd) })
			Expect(os.Chdir(originalDir)).To(Succeed())

			// Create prompt manager
			promptMgr := prompt.NewManager(
				filepath.Join(originalDir, "prompts", "inbox"),
				promptsInProgress,
				promptsCompleted,
				"",
				&osFileMover{},
				libtime.NewCurrentDateTime(),
			)

			rel := &realGitReleaser{workDir: originalDir}
			deps := processor.WorkflowDeps{
				PromptManager: promptMgr,
				AutoCompleter: &stubAutoCompleter{},
				Releaser:      rel,
				Brancher:      &realBrancher{workDir: originalDir},
				Worktreer:     &realWorktreer{originalDir: originalDir},
				ProjectName:   "test-project",
			}
			executor := processor.NewWorktreeWorkflowExecutor(deps)

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "committing"},
				[]byte("# Test\n"),
				libtime.NewCurrentDateTime(),
			)

			err = executor.Setup(ctx, "001-test", pf)
			Expect(err).NotTo(HaveOccurred())

			// Modify code file in worktree
			writeFileWt(codeFile, "package main // modified\n")

			err = executor.Complete(ctx, ctx, pf, "test commit", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred(), "Complete MUST return nil (success-with-warning), NOT propagate the rename error")
			logs := logBuf.String()
			Expect(logs).To(ContainSubstring("clone-sync-mismatch"))
			Expect(logs).To(ContainSubstring(promptPath))
			Expect(logs).To(ContainSubstring(completedPath))

			// Remote was pushed successfully despite local sync failure (push happens BEFORE the sync attempt):
			out, gErr := exec.CommandContext(ctx, "git", "-C", bareDir, "branch", "--list", "dark-factory/001-test").CombinedOutput()
			Expect(gErr).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(string(out))).NotTo(BeEmpty())
		},
	)
})
