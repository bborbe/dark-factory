// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor_test

import (
	"context"
	stderrors "errors"
	"os"
	"os/exec"
	"strings"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("branchWorkflowExecutor setupInPlaceBranch", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("when FetchAndVerifyBranch succeeds (branch exists)", func() {
		It("calls DiscardUncommittedInPaths before Switch", func() {
			fakeBrancher := &mocks.Brancher{}
			fakeBrancher.IsCleanIgnoringReturns([]string{}, nil)
			fakeBrancher.DefaultBranchReturns("master", nil)
			fakeBrancher.FetchAndVerifyBranchReturns(nil)
			fakeBrancher.SwitchReturns(nil)
			fakeBrancher.DiscardUncommittedInPathsReturns(nil)

			prefixes := []string{"prompts/in-progress/", "prompts/completed/"}
			deps := processor.WorkflowDeps{
				Brancher:           fakeBrancher,
				IgnorePathPrefixes: prefixes,
			}
			err := processor.SetupInPlaceBranchForTest(deps, ctx, "dark-factory/test-prompt")
			Expect(err).NotTo(HaveOccurred())

			// DiscardUncommittedInPaths must be called with the prefix list.
			Expect(fakeBrancher.DiscardUncommittedInPathsCallCount()).To(Equal(1))
			_, gotPrefixes := fakeBrancher.DiscardUncommittedInPathsArgsForCall(0)
			Expect(gotPrefixes).To(Equal(prefixes))

			// Switch must be called (branch existed); CreateAndSwitch must not.
			Expect(fakeBrancher.SwitchCallCount()).To(Equal(1))
			Expect(fakeBrancher.CreateAndSwitchCallCount()).To(Equal(0))
		})
	})

	Describe("when FetchAndVerifyBranch fails (branch does not exist)", func() {
		It("calls DiscardUncommittedInPaths and CreateAndSwitch", func() {
			fakeBrancher := &mocks.Brancher{}
			fakeBrancher.IsCleanIgnoringReturns([]string{}, nil)
			fakeBrancher.DefaultBranchReturns("master", nil)
			fakeBrancher.FetchAndVerifyBranchReturns(stderrors.New("not found"))
			fakeBrancher.CreateAndSwitchReturns(nil)
			fakeBrancher.DiscardUncommittedInPathsReturns(nil)

			deps := processor.WorkflowDeps{
				Brancher:           fakeBrancher,
				IgnorePathPrefixes: []string{"prompts/in-progress/"},
			}
			err := processor.SetupInPlaceBranchForTest(deps, ctx, "dark-factory/new-prompt")
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeBrancher.DiscardUncommittedInPathsCallCount()).To(Equal(1))
			Expect(fakeBrancher.SwitchCallCount()).To(Equal(0))
			Expect(fakeBrancher.CreateAndSwitchCallCount()).To(Equal(1))
		})
	})

	It("returns error when DiscardUncommittedInPaths fails", func() {
		fakeBrancher := &mocks.Brancher{}
		fakeBrancher.IsCleanIgnoringReturns([]string{}, nil)
		fakeBrancher.DefaultBranchReturns("master", nil)
		fakeBrancher.DiscardUncommittedInPathsReturns(stderrors.New("git error"))

		deps := processor.WorkflowDeps{
			Brancher:           fakeBrancher,
			IgnorePathPrefixes: []string{"prompts/"},
		}
		err := processor.SetupInPlaceBranchForTest(deps, ctx, "dark-factory/broken-prompt")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("discard bookkeeping dirt before branch switch"))

		// Neither Switch nor CreateAndSwitch must be called.
		Expect(fakeBrancher.SwitchCallCount()).To(Equal(0))
		Expect(fakeBrancher.CreateAndSwitchCallCount()).To(Equal(0))
	})

	It("aborts before discard when IsCleanIgnoring finds user-source dirt", func() {
		fakeBrancher := &mocks.Brancher{}
		fakeBrancher.IsCleanIgnoringReturns([]string{"pkg/handler/list-sprints.go"}, nil)

		deps := processor.WorkflowDeps{
			Brancher:           fakeBrancher,
			IgnorePathPrefixes: []string{"prompts/"},
		}
		err := processor.SetupInPlaceBranchForTest(deps, ctx, "dark-factory/some-prompt")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("working tree is not clean"))

		// Discard must NOT be called — IsCleanIgnoring gate runs first.
		Expect(fakeBrancher.DiscardUncommittedInPathsCallCount()).To(Equal(0))
	})
})

// Helper functions and test helpers

// setupRealGitRepo creates a real git repo in a temp directory with an initial commit.
func setupRealGitRepoBranch(t GinkgoTInterface) string {
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

// setupBareRemoteWithCloneBranch creates a bare remote repo and a clone from it.
// Returns (bareDir, cloneDir).
func setupBareRemoteWithCloneBranch(t GinkgoTInterface) (string, string) {
	bareDir := t.TempDir() + "/remote.git"
	cloneDir := t.TempDir() + "/clone"

	// Create bare repo
	cmd := exec.Command("git", "init", "--bare", bareDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init --bare failed: %v", err)
	}

	// Create a temp repo to push to bare
	tempRepo := t.TempDir()
	cmd = exec.Command("git", "init")
	cmd.Dir = tempRepo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tempRepo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config failed: %v", err)
	}
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tempRepo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config failed: %v", err)
	}
	if err := os.WriteFile(tempRepo+"/README.md", []byte("# test"), 0644); err != nil {
		t.Fatalf("write README failed: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tempRepo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tempRepo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}
	cmd = exec.Command("git", "remote", "add", "origin", bareDir)
	cmd.Dir = tempRepo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git remote add failed: %v", err)
	}
	cmd = exec.Command("git", "push", "origin", "master")
	cmd.Dir = tempRepo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git push failed: %v", err)
	}

	// Clone from bare
	cmd = exec.Command("git", "clone", bareDir, cloneDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git clone failed: %v", err)
	}
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = cloneDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config failed: %v", err)
	}
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = cloneDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config failed: %v", err)
	}

	return bareDir, cloneDir
}

// writePromptFileBranch writes a prompt file with the given status.
func writePromptFileBranch(path, status string) {
	content := "---\nstatus: " + status + "\n---\n# Test Prompt\n"
	Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
}

// writeFileBranch writes content to a file.
func writeFileBranch(path, content string) {
	Expect(os.WriteFile(path, []byte(content), 0644)).To(Succeed())
}

var _ = Describe("branchWorkflowExecutor moves prompt before commit", func() {
	It(
		"produces a single commit on the feature branch containing both code change and prompt rename (move before commit)",
		func() {
			ctx := context.Background()
			repoDir := setupRealGitRepoBranch(GinkgoT())

			// Configure a remote (using the repo itself as origin for branch push testing)
			cmd := exec.Command("git", "remote", "add", "origin", repoDir)
			cmd.Dir = repoDir
			if err := cmd.Run(); err != nil {
				Fail("git remote add failed: " + err.Error())
			}

			// Create prompt and code directories
			promptsInProgress := repoDir + "/prompts/in-progress"
			promptsCompleted := repoDir + "/prompts/completed"
			Expect(os.MkdirAll(promptsInProgress, 0750)).To(Succeed())
			Expect(os.MkdirAll(promptsCompleted, 0750)).To(Succeed())

			promptPath := promptsInProgress + "/001-test.md"
			completedPath := promptsCompleted + "/001-test.md"
			codeFile := repoDir + "/code.go"

			// Write prompt file with "committing" status
			writePromptFileBranch(promptPath, "committing")
			// Write code file
			writeFileBranch(codeFile, "package main\n")

			// Commit the initial state (prompt at in-progress for rename detection)
			cmd = exec.CommandContext(ctx, "git", "add", ".")
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

			deps := processor.WorkflowDeps{
				PromptManager:      promptMgr,
				AutoCompleter:      &stubAutoCompleter{},
				Releaser:           &realGitReleaser{workDir: repoDir},
				Brancher:           &realBrancher{workDir: repoDir},
				IgnorePathPrefixes: []string{"prompts/in-progress/", "prompts/completed/"},
			}
			executor := processor.NewBranchWorkflowExecutor(deps)

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "committing"},
				[]byte("# Test\n"),
				libtime.NewCurrentDateTime(),
			)

			// Modify code file before complete
			writeFileBranch(codeFile, "package main // modified\n")

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

var _ = Describe("BRO-20203 regression: lib-crypto-divergence on branch workflow", func() {
	It(
		"after prompt PR merge, origin/master shows prompt only at completed/ not in-progress/ (bro-20203 lib-crypto-divergence)",
		func() {
			ctx := context.Background()
			bareDir, cloneDir := setupBareRemoteWithCloneBranch(GinkgoT())

			// Create prompt directories in the clone
			promptsInProgress := cloneDir + "/prompts/in-progress"
			promptsCompleted := cloneDir + "/prompts/completed"
			Expect(os.MkdirAll(promptsInProgress, 0750)).To(Succeed())
			Expect(os.MkdirAll(promptsCompleted, 0750)).To(Succeed())

			promptPath := promptsInProgress + "/001-test.md"
			completedPath := promptsCompleted + "/001-test.md"
			codeFile := cloneDir + "/code.go"

			// Write prompt file
			writePromptFileBranch(promptPath, "committing")
			// Write code file
			writeFileBranch(codeFile, "package main\n")

			// Commit initial files so the working tree is clean before Setup
			cmd := exec.CommandContext(ctx, "git", "add", ".")
			cmd.Dir = cloneDir
			Expect(cmd.Run()).To(Succeed())
			cmd = exec.CommandContext(ctx, "git", "commit", "-m", "initial files")
			cmd.Dir = cloneDir
			Expect(cmd.Run()).To(Succeed())

			// Create prompt manager with osFileMover
			promptMgr := prompt.NewManager(
				cloneDir+"/prompts/inbox",
				promptsInProgress,
				promptsCompleted,
				"",
				&osFileMover{},
				libtime.NewCurrentDateTime(),
			)

			deps := processor.WorkflowDeps{
				PromptManager:      promptMgr,
				AutoCompleter:      &stubAutoCompleter{},
				Releaser:           &realGitReleaser{workDir: cloneDir},
				Brancher:           &realBrancher{workDir: cloneDir},
				IgnorePathPrefixes: []string{"prompts/in-progress/", "prompts/completed/"},
			}
			executor := processor.NewBranchWorkflowExecutor(deps)

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "committing"},
				[]byte("# Test\n"),
				libtime.NewCurrentDateTime(),
			)

			// Setup executor
			err := executor.Setup(ctx, "001-test", pf)
			Expect(err).NotTo(HaveOccurred())

			// Complete the workflow
			err = executor.Complete(ctx, ctx, pf, "test commit", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred())

			// Get the feature branch name
			featureBranch := pf.Branch()
			Expect(featureBranch).NotTo(BeEmpty())

			// Simulate PR merge: get the current HEAD (the merge commit) after MergeToDefault
			output, err := exec.CommandContext(ctx, "git", "-C", cloneDir, "rev-parse", "HEAD").
				CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			mergeSHA := strings.TrimSpace(string(output))

			// Update the bare remote's master branch to point to the merge commit
			cmd = exec.Command("git", "-C", bareDir, "branch", "-f", "master", mergeSHA)
			if err := cmd.Run(); err != nil {
				Fail("git branch -f master failed: " + err.Error())
			}

			// Verify: prompts/in-progress/ does NOT contain the prompt file.
			// `git ls-tree master -- <path>` exits 0 with empty stdout when path doesn't match.
			output, err = exec.CommandContext(ctx, "git", "-C", bareDir, "ls-tree", "master", "--", "prompts/in-progress/001-test.md").
				CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(
				strings.TrimSpace(string(output)),
			).To(BeEmpty(), "prompts/in-progress/001-test.md should NOT exist on master after merge")

			// Verify: prompts/completed/ DOES contain the prompt
			output, err = exec.CommandContext(ctx, "git", "-C", bareDir, "ls-tree", "master", "--", "prompts/completed/001-test.md").
				CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("prompts/completed/001-test.md"))
		},
	)
})
