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

var _ = Describe("Worktree", func() {
	var (
		ctx         context.Context
		tempDir     string
		originalDir string
		w           git.Worktree
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		originalDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		tempDir, err = os.MkdirTemp("", "worktree-test-*")
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

		w = git.NewWorktree()
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

	Describe("Add", func() {
		It("creates a worktree at the given path with the given branch", func() {
			worktreePath := filepath.Join(tempDir, "worktree-1")

			err := w.Add(ctx, worktreePath, "feature-branch")
			Expect(err).NotTo(HaveOccurred())

			// Verify worktree directory exists
			info, err := os.Stat(worktreePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.IsDir()).To(BeTrue())

			// Verify branch exists in worktree
			cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
			cmd.Dir = worktreePath
			output, err := cmd.Output()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("feature-branch"))
		})

		It("returns error for branch that already exists", func() {
			worktreePath1 := filepath.Join(tempDir, "worktree-1")
			worktreePath2 := filepath.Join(tempDir, "worktree-2")

			// Create first worktree with the branch
			err := w.Add(ctx, worktreePath1, "existing-branch")
			Expect(err).NotTo(HaveOccurred())

			// Try to create second worktree with same branch name
			err = w.Add(ctx, worktreePath2, "existing-branch")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Remove", func() {
		It("removes an existing worktree", func() {
			worktreePath := filepath.Join(tempDir, "worktree-remove")

			// Create worktree first
			err := w.Add(ctx, worktreePath, "remove-test-branch")
			Expect(err).NotTo(HaveOccurred())

			// Verify it exists
			_, err = os.Stat(worktreePath)
			Expect(err).NotTo(HaveOccurred())

			// Remove it
			err = w.Remove(ctx, worktreePath)
			Expect(err).NotTo(HaveOccurred())

			// Verify it's gone
			_, err = os.Stat(worktreePath)
			Expect(err).To(HaveOccurred())
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("returns error for non-existent worktree", func() {
			nonExistentPath := filepath.Join(tempDir, "does-not-exist")

			err := w.Remove(ctx, nonExistentPath)
			Expect(err).To(HaveOccurred())
		})
	})
})
