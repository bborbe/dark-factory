// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"
	"os"
	"os/exec"

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
	})
})
