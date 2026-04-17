// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("Worktreer", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("NewWorktreer", func() {
		It("creates a Worktreer", func() {
			worktreer := git.NewWorktreer()
			Expect(worktreer).NotTo(BeNil())
		})
	})

	Describe("Add", func() {
		It("returns non-nil error when CWD is not a git repo", func() {
			worktreer := git.NewWorktreer()
			tmpPath := GinkgoT().TempDir()
			worktreePath := tmpPath + "/worktree"

			// Add runs git from CWD — chdir to non-repo dir so git fails.
			origDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Chdir(tmpPath)).To(Succeed())
			DeferCleanup(func() { _ = os.Chdir(origDir) })

			err = worktreer.Add(ctx, worktreePath, "some-branch")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("worktree add"))
		})
	})

	Describe("Remove", func() {
		It("returns nil even when path does not exist", func() {
			worktreer := git.NewWorktreer()
			// Non-existent path — Remove must always return nil per contract
			err := worktreer.Remove(ctx, "/tmp/nonexistent-worktree-path-xyz-abc")
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
