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
	})

	Describe("Pull", func() {
		It("returns error when no remote is configured", func() {
			// Try to pull (should fail since no remote)
			err := b.Pull(ctx)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("MergeOriginDefault", func() {
		It("returns error when not in a GitHub repository", func() {
			// MergeOriginDefault requires DefaultBranch which needs gh CLI
			err := b.MergeOriginDefault(ctx)
			Expect(err).To(HaveOccurred())
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
})
