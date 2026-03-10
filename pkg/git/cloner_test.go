// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("Cloner", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("NewCloner", func() {
		It("creates a Cloner", func() {
			cloner := git.NewCloner()
			Expect(cloner).NotTo(BeNil())
		})
	})

	Describe("Remove", func() {
		It("removes an existing directory", func() {
			cloner := git.NewCloner()
			dir, err := os.MkdirTemp("", "cloner-remove-*")
			Expect(err).NotTo(HaveOccurred())

			err = cloner.Remove(ctx, dir)
			Expect(err).NotTo(HaveOccurred())

			_, statErr := os.Stat(dir)
			Expect(os.IsNotExist(statErr)).To(BeTrue())
		})

		It("succeeds for non-existent path", func() {
			cloner := git.NewCloner()
			err := cloner.Remove(ctx, "/tmp/nonexistent-cloner-dir-xyz")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Clone", func() {
		var (
			srcDir  string
			destDir string
		)

		BeforeEach(func() {
			var err error
			srcDir, err = os.MkdirTemp("", "cloner-src-*")
			Expect(err).NotTo(HaveOccurred())

			destDir, err = os.MkdirTemp("", "cloner-dest-*")
			Expect(err).NotTo(HaveOccurred())
			// Remove destDir so git clone can create it
			err = os.RemoveAll(destDir)
			Expect(err).NotTo(HaveOccurred())

			// Initialize bare git repo as source
			cmd := exec.Command("git", "init", "--bare", srcDir)
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			_ = os.RemoveAll(srcDir)
			_ = os.RemoveAll(destDir)
		})

		It("returns error when source has no remote URL", func() {
			cloner := git.NewCloner()
			// srcDir is a bare repo with no remote; Clone succeeds up to get-url which fails
			err := cloner.Clone(ctx, srcDir, destDir, "feature-branch")
			Expect(err).To(HaveOccurred())
		})

		It("returns error when source path does not exist", func() {
			cloner := git.NewCloner()
			err := cloner.Clone(ctx, "/nonexistent/source/path", destDir, "feature-branch")
			Expect(err).To(HaveOccurred())
		})

		It("removes stale destDir before cloning", func() {
			cloner := git.NewCloner()

			// Create a non-empty destDir to simulate stale clone
			err := os.MkdirAll(destDir, 0750)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(destDir+"/stale-file.txt", []byte("stale"), 0600)
			Expect(err).NotTo(HaveOccurred())

			// Clone should remove the stale dir and proceed (will still fail at get-url, but not at clone)
			err = cloner.Clone(ctx, srcDir, destDir, "feature-branch")
			// Error expected because bare repo has no remote, but it should NOT be "already exists"
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("already exists"))
		})

		Context("with a fully configured source repo and remote", func() {
			var (
				remoteDir string
				sourceDir string
			)

			// initSourceWithRemote sets up a source repo (non-bare) with a real remote.
			initSourceWithRemote := func() {
				var err error
				remoteDir, err = os.MkdirTemp("", "cloner-remote-*")
				Expect(err).NotTo(HaveOccurred())

				sourceDir, err = os.MkdirTemp("", "cloner-source-*")
				Expect(err).NotTo(HaveOccurred())

				// Initialize bare remote
				cmd := exec.Command("git", "init", "--bare", remoteDir)
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Init source repo
				cmd = exec.Command("git", "init", sourceDir)
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "config", "user.email", "test@example.com")
				cmd.Dir = sourceDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "config", "user.name", "Test User")
				cmd.Dir = sourceDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Add initial commit
				err = os.WriteFile(filepath.Join(sourceDir, "README.md"), []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "add", ".")
				cmd.Dir = sourceDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "initial")
				cmd.Dir = sourceDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Set remote to bare repo
				cmd = exec.Command("git", "remote", "add", "origin", remoteDir)
				cmd.Dir = sourceDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Push to remote
				cmd = exec.Command("git", "push", "origin", "HEAD")
				cmd.Dir = sourceDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())
			}

			AfterEach(func() {
				_ = os.RemoveAll(remoteDir)
				_ = os.RemoveAll(sourceDir)
			})

			It(
				"creates fresh branch with checkout -b when branch does not exist on remote",
				func() {
					initSourceWithRemote()
					cloner := git.NewCloner()
					err := cloner.Clone(ctx, sourceDir, destDir, "new-feature-branch")
					Expect(err).NotTo(HaveOccurred())

					// Verify we're on the new branch
					cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
					cmd.Dir = destDir
					output, err := cmd.Output()
					Expect(err).NotTo(HaveOccurred())
					Expect(strings.TrimSpace(string(output))).To(Equal("new-feature-branch"))
				},
			)

			It(
				"tracks existing remote branch with checkout --track when branch exists on remote",
				func() {
					initSourceWithRemote()

					// Push a feature branch to the remote
					cmd := exec.Command("git", "checkout", "-b", "existing-remote-branch")
					cmd.Dir = sourceDir
					err := cmd.Run()
					Expect(err).NotTo(HaveOccurred())

					cmd = exec.Command("git", "push", "origin", "existing-remote-branch")
					cmd.Dir = sourceDir
					err = cmd.Run()
					Expect(err).NotTo(HaveOccurred())

					// Switch back to default branch in source
					cmd = exec.Command("git", "checkout", "-")
					cmd.Dir = sourceDir
					err = cmd.Run()
					Expect(err).NotTo(HaveOccurred())

					// Clone should track existing remote branch
					cloner := git.NewCloner()
					err = cloner.Clone(ctx, sourceDir, destDir, "existing-remote-branch")
					Expect(err).NotTo(HaveOccurred())

					// Verify we're on the tracked branch
					cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
					cmd.Dir = destDir
					output, err := cmd.Output()
					Expect(err).NotTo(HaveOccurred())
					Expect(strings.TrimSpace(string(output))).To(Equal("existing-remote-branch"))

					// Verify it's tracking the remote
					cmd = exec.Command(
						"git",
						"rev-parse",
						"--abbrev-ref",
						"--symbolic-full-name",
						"@{u}",
					)
					cmd.Dir = destDir
					output, err = cmd.Output()
					Expect(err).NotTo(HaveOccurred())
					Expect(
						strings.TrimSpace(string(output)),
					).To(Equal("origin/existing-remote-branch"))
				},
			)
		})
	})
})
