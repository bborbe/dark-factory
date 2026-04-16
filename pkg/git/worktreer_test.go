// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"

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
		It("returns non-nil error when path is not a git repo", func() {
			worktreer := git.NewWorktreer()
			// Use a temp path that is not a git repo — Add must fail
			tmpPath := GinkgoT().TempDir()
			worktreePath := tmpPath + "/worktree"
			err := worktreer.Add(ctx, worktreePath, "some-branch")
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
