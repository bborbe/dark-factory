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

	Describe("MergeOriginMaster", func() {
		It("returns error when no remote tracking branch exists", func() {
			// Try to merge origin/master (should fail since no remote tracking branch)
			err := b.MergeOriginMaster(ctx)
			Expect(err).To(HaveOccurred())
		})

		It("succeeds when no conflicts exist", func() {
			// Set up a remote tracking branch
			cmd := exec.Command("git", "remote", "add", "origin", tempDir)
			cmd.Dir = tempDir
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Fetch to establish tracking
			err = b.Fetch(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Merge should succeed (no changes to merge, but command should succeed)
			err = b.MergeOriginMaster(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error when conflicts exist", func() {
			// Create a second repo to simulate a remote with conflicts
			remoteDir, err := os.MkdirTemp("", "brancher-remote-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(remoteDir) }()

			// Initialize remote repo
			cmd := exec.Command("git", "init", "--bare")
			cmd.Dir = remoteDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Add remote
			cmd = exec.Command("git", "remote", "add", "origin", remoteDir)
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Push to remote
			cmd = exec.Command("git", "push", "-u", "origin", "master")
			cmd.Dir = tempDir
			err = cmd.Run()
			// Ignore error if branch name is main instead of master
			if err != nil {
				cmd = exec.Command("git", "push", "-u", "origin", "main")
				cmd.Dir = tempDir
				err = cmd.Run()
			}
			Expect(err).NotTo(HaveOccurred())

			// Create conflicting changes locally
			err = os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("local change"), 0600)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "commit", "-am", "local change")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Create conflicting changes in a clone (simulating remote changes)
			cloneDir, err := os.MkdirTemp("", "brancher-clone-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(cloneDir) }()

			cmd = exec.Command("git", "clone", remoteDir, cloneDir)
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Configure git in clone
			cmd = exec.Command("git", "config", "user.email", "test@example.com")
			cmd.Dir = cloneDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "config", "user.name", "Test User")
			cmd.Dir = cloneDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Make conflicting change in clone
			err = os.WriteFile(filepath.Join(cloneDir, "test.txt"), []byte("remote change"), 0600)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "commit", "-am", "remote change")
			cmd.Dir = cloneDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "push")
			cmd.Dir = cloneDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Fetch the conflicting changes
			err = b.Fetch(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Try to merge - should fail due to conflict
			err = b.MergeOriginMaster(ctx)
			Expect(err).To(HaveOccurred())
		})
	})
})
