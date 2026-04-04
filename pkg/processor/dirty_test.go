// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor_test

import (
	"context"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/processor"
)

var _ = Describe("gitDirtyFileChecker", func() {
	var (
		ctx     context.Context
		tempDir string
		checker processor.DirtyFileChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "dirty-checker-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Initialize a git repo in tempDir
		cmd := exec.Command("git", "init")
		cmd.Dir = tempDir
		Expect(cmd.Run()).To(Succeed())

		cmd = exec.Command("git", "config", "user.email", "test@example.com")
		cmd.Dir = tempDir
		Expect(cmd.Run()).To(Succeed())

		cmd = exec.Command("git", "config", "user.name", "Test")
		cmd.Dir = tempDir
		Expect(cmd.Run()).To(Succeed())

		checker = processor.NewDirtyFileChecker(tempDir)
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It("returns 0 for an empty repo", func() {
		count, err := checker.CountDirtyFiles(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(0))
	})

	It("returns correct count for untracked files", func() {
		Expect(os.WriteFile(tempDir+"/a.txt", []byte("a"), 0600)).To(Succeed())
		Expect(os.WriteFile(tempDir+"/b.txt", []byte("b"), 0600)).To(Succeed())

		count, err := checker.CountDirtyFiles(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(2))
	})

	It("returns correct count for modified tracked files", func() {
		// Create and commit a file
		Expect(os.WriteFile(tempDir+"/tracked.txt", []byte("original"), 0600)).To(Succeed())
		cmd := exec.Command("git", "add", "tracked.txt")
		cmd.Dir = tempDir
		Expect(cmd.Run()).To(Succeed())
		cmd = exec.Command("git", "commit", "-m", "initial")
		cmd.Dir = tempDir
		Expect(cmd.Run()).To(Succeed())

		// Modify the tracked file
		Expect(os.WriteFile(tempDir+"/tracked.txt", []byte("modified"), 0600)).To(Succeed())

		count, err := checker.CountDirtyFiles(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(1))
	})
})
