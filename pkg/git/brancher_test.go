// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("Brancher", func() {
	var (
		ctx         context.Context
		tempDir     string
		originalDir string
		b           git.Brancher
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		originalDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		tempDir, err = os.MkdirTemp("", "brancher-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Initialize git repo
		cmd := exec.Command("git", "init")
		cmd.Dir = tempDir
		err = cmd.Run()
		Expect(err).NotTo(HaveOccurred())

		// Configure git
		cmd = exec.Command("git", "config", "user.email", "test@example.com")
		cmd.Dir = tempDir
		err = cmd.Run()
		Expect(err).NotTo(HaveOccurred())

		cmd = exec.Command("git", "config", "user.name", "Test User")
		cmd.Dir = tempDir
		err = cmd.Run()
		Expect(err).NotTo(HaveOccurred())

		// Create initial commit
		err = os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0600)
		Expect(err).NotTo(HaveOccurred())

		cmd = exec.Command("git", "add", ".")
		cmd.Dir = tempDir
		err = cmd.Run()
		Expect(err).NotTo(HaveOccurred())

		cmd = exec.Command("git", "commit", "-m", "initial")
		cmd.Dir = tempDir
		err = cmd.Run()
		Expect(err).NotTo(HaveOccurred())

		// Change to temp directory
		err = os.Chdir(tempDir)
		Expect(err).NotTo(HaveOccurred())

		b = git.NewBrancher()
	})

	AfterEach(func() {
		if originalDir != "" {
			err := os.Chdir(originalDir)
			Expect(err).NotTo(HaveOccurred())
		}

		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Describe("NewBrancher", func() {
		It("creates Brancher", func() {
			brancher := git.NewBrancher()
			Expect(brancher).NotTo(BeNil())
		})

		It("creates Brancher with WithDefaultBranch option", func() {
			brancher := git.NewBrancher(git.WithDefaultBranch("main"))
			Expect(brancher).NotTo(BeNil())
		})

		It("WithDefaultBranch empty string is a no-op", func() {
			brancher := git.NewBrancher(git.WithDefaultBranch(""))
			Expect(brancher).NotTo(BeNil())
		})
	})

	Describe("CurrentBranch", func() {
		It("returns the current branch name", func() {
			branch, err := b.CurrentBranch(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(branch).To(Or(Equal("master"), Equal("main")))
		})
	})

	Describe("CreateAndSwitch", func() {
		It("creates a new branch and switches to it", func() {
			err := b.CreateAndSwitch(ctx, "feature-branch")
			Expect(err).NotTo(HaveOccurred())

			// Verify we're on the new branch
			branch, err := b.CurrentBranch(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(branch).To(Equal("feature-branch"))
		})

		It("returns error when branch already exists", func() {
			// Create branch first
			err := b.CreateAndSwitch(ctx, "existing-branch")
			Expect(err).NotTo(HaveOccurred())

			// Switch back to master
			err = b.Switch(ctx, "master")
			if err != nil {
				// Try main if master doesn't exist
				err = b.Switch(ctx, "main")
			}
			Expect(err).NotTo(HaveOccurred())

			// Try to create same branch again
			err = b.CreateAndSwitch(ctx, "existing-branch")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Switch", func() {
		It("switches to an existing branch", func() {
			// Create and switch to new branch
			err := b.CreateAndSwitch(ctx, "test-branch")
			Expect(err).NotTo(HaveOccurred())

			// Switch back to master/main
			err = b.Switch(ctx, "master")
			if err != nil {
				err = b.Switch(ctx, "main")
			}
			Expect(err).NotTo(HaveOccurred())

			// Verify we're on master/main
			branch, err := b.CurrentBranch(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(branch).To(Or(Equal("master"), Equal("main")))
		})

		It("returns error when branch does not exist", func() {
			err := b.Switch(ctx, "nonexistent-branch")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Push", func() {
		It("returns error when no remote is configured", func() {
			// Create and switch to new branch
			err := b.CreateAndSwitch(ctx, "push-test-branch")
			Expect(err).NotTo(HaveOccurred())

			// Try to push (should fail since no remote)
			err = b.Push(ctx, "push-test-branch")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Fetch", func() {
		It("returns error when no remote is configured", func() {
			// Try to fetch (should fail since no remote)
			err := b.Fetch(ctx)
			Expect(err).To(HaveOccurred())
		})

		It("succeeds when remote is configured", func() {
			// Configure a remote (using the same repo as remote for test purposes)
			cmd := exec.Command("git", "remote", "add", "origin", tempDir)
			cmd.Dir = tempDir
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Fetch should succeed now
			err = b.Fetch(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("FetchAndVerifyBranch", func() {
		It("returns error when no remote is configured", func() {
			err := b.FetchAndVerifyBranch(ctx, "some-branch")
			Expect(err).To(HaveOccurred())
		})

		It("returns error containing branch name when branch does not exist remotely", func() {
			// Configure a remote (using the same repo as remote)
			cmd := exec.Command("git", "remote", "add", "origin", tempDir)
			cmd.Dir = tempDir
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			err = b.FetchAndVerifyBranch(ctx, "nonexistent-branch")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent-branch"))
		})

		It("returns nil when branch exists remotely", func() {
			// Set up a bare clone to act as origin
			bareDir, err := os.MkdirTemp("", "brancher-bare-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(bareDir) }()

			// Clone the tempDir as a bare repo
			cmd := exec.Command("git", "clone", "--bare", tempDir, bareDir)
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Add the bare clone as origin
			cmd = exec.Command("git", "remote", "add", "origin", bareDir)
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Get default branch name
			branch, err := b.CurrentBranch(ctx)
			Expect(err).NotTo(HaveOccurred())

			err = b.FetchAndVerifyBranch(ctx, branch)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("DefaultBranch", func() {
		It("returns error when not in a GitHub repository", func() {
			// DefaultBranch requires gh CLI and a GitHub repository
			_, err := b.DefaultBranch(ctx)
			Expect(err).To(HaveOccurred())
		})

		It("returns configured branch without calling gh", func() {
			configured := git.NewBrancher(git.WithDefaultBranch("main"))
			branch, err := configured.DefaultBranch(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(branch).To(Equal("main"))
		})

		It("returns configured branch master without calling gh", func() {
			configured := git.NewBrancher(git.WithDefaultBranch("master"))
			branch, err := configured.DefaultBranch(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(branch).To(Equal("master"))
		})

		It("falls back to git symbolic-ref when gh is unavailable", func() {
			// Create a bare repo to act as origin
			bareDir, err := os.MkdirTemp("", "brancher-bare-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(bareDir) }()

			cmd := exec.Command("git", "init", "--bare", bareDir)
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Add bare repo as remote origin
			cmd = exec.Command("git", "remote", "add", "origin", bareDir)
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Push current branch to bare repo
			cmd = exec.Command("git", "push", "origin", "HEAD:master")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Fetch to populate refs/remotes/origin/
			cmd = exec.Command("git", "fetch", "origin")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Set the symbolic ref so git knows the default branch
			cmd = exec.Command("git", "remote", "set-head", "origin", "--auto")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			branch, err := b.DefaultBranch(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(branch).To(Equal("master"))
		})

		It("returns error when both gh and git symbolic-ref fail", func() {
			// No remote configured, so git symbolic-ref also fails
			_, err := b.DefaultBranch(ctx)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Pull", func() {
		It("returns error when no remote is configured", func() {
			// Try to pull (should fail since no remote)
			err := b.Pull(ctx)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("IsClean", func() {
		It("returns true when working tree is clean", func() {
			clean, err := b.IsClean(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(clean).To(BeTrue())
		})

		It("returns false when working tree has uncommitted changes", func() {
			// Create a new file that is untracked (makes tree dirty)
			err := os.WriteFile(filepath.Join(tempDir, "dirty.txt"), []byte("dirty"), 0600)
			Expect(err).NotTo(HaveOccurred())

			clean, err := b.IsClean(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(clean).To(BeFalse())
		})
	})

	Describe("IsCleanIgnoring", func() {
		It("returns nil when repo is clean", func() {
			dirtyPaths, err := b.IsCleanIgnoring(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(dirtyPaths).To(BeNil())
		})

		It("returns the dirty path when file is outside all ignore prefixes", func() {
			// Create an untracked file at root — git reports "?? user-code.go"
			Expect(
				os.WriteFile(filepath.Join(tempDir, "user-code.go"), []byte("package main"), 0600),
			).To(Succeed())

			dirtyPaths, err := b.IsCleanIgnoring(ctx, []string{"prompts/in-progress"})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(dirtyPaths)).To(Equal(1))
			Expect(dirtyPaths[0]).To(Equal("user-code.go"))
		})

		It("returns nil when staged dirty file matches ignore prefix", func() {
			// Stage a new file so git reports the individual path "A  prompts/in-progress/001-test.md"
			// rather than the parent directory "?? prompts/" (untracked dirs are collapsed by git).
			Expect(os.MkdirAll(filepath.Join(tempDir, "prompts/in-progress"), 0750)).To(Succeed())
			Expect(
				os.WriteFile(
					filepath.Join(tempDir, "prompts/in-progress/001-test.md"),
					[]byte("---\nstatus: in-progress\n---\n"),
					0600,
				),
			).To(Succeed())
			addCmd := exec.Command("git", "add", "prompts/in-progress/001-test.md")
			Expect(addCmd.Run()).To(Succeed())

			dirtyPaths, err := b.IsCleanIgnoring(ctx, []string{"prompts/in-progress"})
			Expect(err).NotTo(HaveOccurred())
			Expect(dirtyPaths).To(BeNil())
		})

		It("returns only non-ignored dirty path when mixed dirty files exist", func() {
			// Stage the prompt file so it appears with a full individual path in git status.
			Expect(os.MkdirAll(filepath.Join(tempDir, "prompts/in-progress"), 0750)).To(Succeed())
			Expect(
				os.WriteFile(
					filepath.Join(tempDir, "prompts/in-progress/001-test.md"),
					[]byte("---\nstatus: in-progress\n---\n"),
					0600,
				),
			).To(Succeed())
			addCmd := exec.Command("git", "add", "prompts/in-progress/001-test.md")
			Expect(addCmd.Run()).To(Succeed())
			// Also create an untracked user file at root
			Expect(
				os.WriteFile(filepath.Join(tempDir, "user-code.go"), []byte("package main"), 0600),
			).To(Succeed())

			dirtyPaths, err := b.IsCleanIgnoring(ctx, []string{"prompts/in-progress"})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(dirtyPaths)).To(Equal(1))
			Expect(dirtyPaths[0]).NotTo(ContainSubstring("prompts"))
			Expect(dirtyPaths[0]).To(Equal("user-code.go"))
		})

		It("returns dirty paths when prefix list is empty and repo is dirty", func() {
			// Create an untracked file — with empty prefixes, all dirty paths are returned.
			Expect(
				os.WriteFile(filepath.Join(tempDir, "user-code.go"), []byte("package main"), 0600),
			).To(Succeed())

			dirtyPaths, err := b.IsCleanIgnoring(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(dirtyPaths)).To(BeNumerically(">=", 1))
		})
	})

	Describe("DiscardUncommittedInPaths", func() {
		BeforeEach(func() {
			// Create and commit a file under prompts/in-progress/ so it is tracked at HEAD.
			Expect(
				os.MkdirAll(filepath.Join(tempDir, "prompts", "in-progress"), 0750),
			).To(Succeed())
			Expect(
				os.WriteFile(
					filepath.Join(tempDir, "prompts", "in-progress", "001-test.md"),
					[]byte("status: queued"),
					0600,
				),
			).To(Succeed())
			addCmd := exec.Command("git", "add", ".")
			addCmd.Dir = tempDir
			Expect(addCmd.Run()).To(Succeed())
			commitCmd := exec.Command("git", "commit", "-m", "add prompt")
			commitCmd.Dir = tempDir
			Expect(commitCmd.Run()).To(Succeed())
		})

		It("restores a dirty tracked file to HEAD state", func() {
			// Dirty the file without committing.
			Expect(
				os.WriteFile(
					filepath.Join(tempDir, "prompts", "in-progress", "001-test.md"),
					[]byte("status: failed"),
					0600,
				),
			).To(Succeed())

			err := b.DiscardUncommittedInPaths(ctx, []string{"prompts/"})
			Expect(err).NotTo(HaveOccurred())

			// File must be back to HEAD state.
			content, readErr := os.ReadFile(
				filepath.Join(tempDir, "prompts", "in-progress", "001-test.md"),
			)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal("status: queued"))

			// git status must show no changes for that file.
			statusCmd := exec.Command("git", "status", "--porcelain")
			statusCmd.Dir = tempDir
			output, runErr := statusCmd.Output()
			Expect(runErr).NotTo(HaveOccurred())
			Expect(string(output)).NotTo(ContainSubstring("001-test.md"))
		})

		It("empty prefixes slice is a no-op — returns nil, leaves dirty file untouched", func() {
			// Dirty the file.
			Expect(
				os.WriteFile(
					filepath.Join(tempDir, "prompts", "in-progress", "001-test.md"),
					[]byte("status: failed"),
					0600,
				),
			).To(Succeed())

			err := b.DiscardUncommittedInPaths(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())

			// File must still be dirty.
			statusCmd := exec.Command("git", "status", "--porcelain")
			statusCmd.Dir = tempDir
			output, runErr := statusCmd.Output()
			Expect(runErr).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("001-test.md"))
		})

		It("prefix with no tracked files is silently skipped — returns nil", func() {
			// A non-existent directory forces git to emit "did not match any file(s) known to git".
			err := b.DiscardUncommittedInPaths(ctx, []string{"completely-missing-dir-xyz/"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("prefix outside the dirty path does not affect the dirty file", func() {
			// Dirty the prompts file.
			Expect(
				os.WriteFile(
					filepath.Join(tempDir, "prompts", "in-progress", "001-test.md"),
					[]byte("status: failed"),
					0600,
				),
			).To(Succeed())

			// Discard using "specs/" — should not touch "prompts/".
			err := b.DiscardUncommittedInPaths(ctx, []string{"specs/"})
			Expect(err).NotTo(HaveOccurred())

			// File under prompts/ must still be dirty.
			statusCmd := exec.Command("git", "status", "--porcelain")
			statusCmd.Dir = tempDir
			output, runErr := statusCmd.Output()
			Expect(runErr).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("001-test.md"))
		})
	})

	Describe("MergeOriginDefault", func() {
		It("skips merge when default branch cannot be determined", func() {
			// When DefaultBranch fails (no GitHub remote, no config), MergeOriginDefault
			// logs a warning and returns nil instead of aborting the prompt execution.
			err := b.MergeOriginDefault(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("with configured default branch skips gh CLI and attempts git merge", func() {
			configured := git.NewBrancher(git.WithDefaultBranch("main"))
			// No remote configured, so git merge will fail — but the error comes from
			// git merge, not from gh CLI, proving the configured branch path is used.
			err := configured.MergeOriginDefault(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("merge origin/main"))
		})
	})

	Describe("FetchBranch", func() {
		It(
			"returns nil when origin does not have the branch (swallows couldn't find remote ref)",
			func() {
				// Set up a bare "origin" without the feature branch pushed to it.
				bareDir := GinkgoT().TempDir()
				cmd := exec.Command("git", "init", "--bare", bareDir)
				Expect(cmd.Run()).To(Succeed())
				cmd = exec.Command("git", "remote", "add", "origin", bareDir)
				cmd.Dir = tempDir
				Expect(cmd.Run()).To(Succeed())

				err := b.FetchBranch(ctx, "dark-factory/not-yet-pushed")
				Expect(err).NotTo(HaveOccurred())
			},
		)

		It("creates a local branch when origin has the branch", func() {
			// Set up a bare "origin".
			bareDir := GinkgoT().TempDir()
			cmd := exec.Command("git", "init", "--bare", bareDir)
			Expect(cmd.Run()).To(Succeed())
			// Add origin remote.
			cmd = exec.Command("git", "remote", "add", "origin", bareDir)
			cmd.Dir = tempDir
			Expect(cmd.Run()).To(Succeed())
			// Push the initial commit to origin.
			cmd = exec.Command("git", "push", "origin", "HEAD:master")
			cmd.Dir = tempDir
			Expect(cmd.Run()).To(Succeed())
			// Create and push a feature branch to origin.
			cmd = exec.Command("git", "checkout", "-b", "dark-factory/feature-foo")
			cmd.Dir = tempDir
			Expect(cmd.Run()).To(Succeed())
			cmd = exec.Command("git", "push", "origin", "dark-factory/feature-foo")
			cmd.Dir = tempDir
			Expect(cmd.Run()).To(Succeed())
			// Switch back to master so dark-factory/feature-foo is not the current branch.
			cmd = exec.Command("git", "checkout", "master")
			cmd.Dir = tempDir
			Expect(cmd.Run()).To(Succeed())
			// Delete the local branch to simulate parent-repo state.
			cmd = exec.Command("git", "branch", "-D", "dark-factory/feature-foo")
			cmd.Dir = tempDir
			Expect(cmd.Run()).To(Succeed())

			// FetchBranch should recreate the local branch from origin.
			err := b.FetchBranch(ctx, "dark-factory/feature-foo")
			Expect(err).NotTo(HaveOccurred())

			// Verify the local branch now exists.
			cmd = exec.Command("git", "rev-parse", "--verify", "dark-factory/feature-foo")
			cmd.Dir = tempDir
			Expect(cmd.Run()).To(Succeed(), "local branch should have been created by FetchBranch")
		})

		It("returns error for invalid branch name", func() {
			err := b.FetchBranch(ctx, "--injected-flag")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("validate branch name"))
		})
	})
})
