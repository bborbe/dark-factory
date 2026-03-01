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
})
