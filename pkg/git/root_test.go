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

var _ = Describe("ResolveGitRoot", func() {
	var (
		ctx         context.Context
		originalDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		originalDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if originalDir != "" {
			_ = os.Chdir(originalDir)
		}
	})

	It("returns the git root when called from the repo root", func() {
		root, err := git.ResolveGitRoot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(root).NotTo(BeEmpty())

		// Verify a .git directory exists at the returned root
		_, statErr := os.Stat(root + "/.git")
		Expect(statErr).NotTo(HaveOccurred())
	})

	It("returns the git root when called from a subdirectory", func() {
		// Get the expected root first
		expectedRoot, err := git.ResolveGitRoot(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Change to a subdirectory
		subdir := expectedRoot + "/pkg/git"
		err = os.Chdir(subdir)
		Expect(err).NotTo(HaveOccurred())

		// Should still return the same root
		root, err := git.ResolveGitRoot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(root).To(Equal(expectedRoot))
	})

	It("returns an error when outside a git repository", func() {
		tmpDir, err := os.MkdirTemp("", "no-git-repo-*")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(tmpDir) }()

		err = os.Chdir(tmpDir)
		Expect(err).NotTo(HaveOccurred())

		_, err = git.ResolveGitRoot(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("git repository"))
	})

	It("returns the correct root for a git repo initialized in a temp dir", func() {
		tmpDir, err := os.MkdirTemp("", "git-root-test-*")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(tmpDir) }()

		// Resolve symlinks (macOS: /var -> /private/var)
		tmpDir, err = filepath.EvalSymlinks(tmpDir)
		Expect(err).NotTo(HaveOccurred())

		// Initialize a git repo in the temp dir
		cmd := exec.Command("git", "init")
		cmd.Dir = tmpDir
		Expect(cmd.Run()).NotTo(HaveOccurred())

		// Create a subdirectory and chdir into it
		subdir := tmpDir + "/sub/dir"
		Expect(os.MkdirAll(subdir, 0750)).NotTo(HaveOccurred())
		Expect(os.Chdir(subdir)).NotTo(HaveOccurred())

		root, err := git.ResolveGitRoot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(root).To(Equal(tmpDir))
	})
})
