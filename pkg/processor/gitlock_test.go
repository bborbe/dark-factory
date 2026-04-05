// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/processor"
)

var _ = Describe("gitLockChecker", func() {
	var (
		tempDir string
		checker processor.GitLockChecker
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "gitlock-checker-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Create a .git directory to simulate a repo
		Expect(os.MkdirAll(filepath.Join(tempDir, ".git"), 0750)).To(Succeed())

		checker = processor.NewGitLockChecker(tempDir)
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It("returns false when no lock file exists", func() {
		Expect(checker.Exists()).To(BeFalse())
	})

	It("returns true when .git/index.lock is present", func() {
		lockPath := filepath.Join(tempDir, ".git", "index.lock")
		Expect(os.WriteFile(lockPath, []byte(""), 0600)).To(Succeed())

		Expect(checker.Exists()).To(BeTrue())
	})
})
