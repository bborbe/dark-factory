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

var _ = Describe("Brancher ForcePush", func() {
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

		tempDir, err = os.MkdirTemp("", "forcepush-test-*")
		Expect(err).NotTo(HaveOccurred())

		cmd := exec.Command("git", "init")
		cmd.Dir = tempDir
		Expect(cmd.Run()).NotTo(HaveOccurred())

		cmd = exec.Command("git", "config", "user.email", "test@example.com")
		cmd.Dir = tempDir
		Expect(cmd.Run()).NotTo(HaveOccurred())

		cmd = exec.Command("git", "config", "user.name", "Test User")
		cmd.Dir = tempDir
		Expect(cmd.Run()).NotTo(HaveOccurred())

		err = os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0600)
		Expect(err).NotTo(HaveOccurred())

		cmd = exec.Command("git", "add", ".")
		cmd.Dir = tempDir
		Expect(cmd.Run()).NotTo(HaveOccurred())

		cmd = exec.Command("git", "commit", "-m", "initial")
		cmd.Dir = tempDir
		Expect(cmd.Run()).NotTo(HaveOccurred())

		Expect(os.Chdir(tempDir)).NotTo(HaveOccurred())

		b = git.NewBrancher()
	})

	AfterEach(func() {
		if originalDir != "" {
			_ = os.Chdir(originalDir)
		}
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Describe("ForcePush", func() {
		It("returns error when no remote is configured", func() {
			err := b.ForcePush(ctx, "master")
			Expect(err).To(HaveOccurred())
		})
	})
})

var _ = Describe("AmendCommit", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns error when context is cancelled", func() {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		err := git.AmendCommit(cancelCtx, "some-path")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("Releaser", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("AmendCommit", func() {
		It("returns error when context is cancelled", func() {
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()
			r := git.NewReleaser()
			err := r.AmendCommit(cancelCtx, "some-path")
			Expect(err).To(HaveOccurred())
		})
	})
})
