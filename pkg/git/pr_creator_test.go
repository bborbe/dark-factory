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

var _ = Describe("PRCreator", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("NewPRCreator", func() {
		It("creates PRCreator without token", func() {
			p := git.NewPRCreator("")
			Expect(p).NotTo(BeNil())
		})

		It("creates PRCreator with token", func() {
			p := git.NewPRCreator("test-token")
			Expect(p).NotTo(BeNil())
		})
	})

	Describe("Create", func() {
		It("returns error when gh CLI fails (no token)", func() {
			p := git.NewPRCreator("")
			// This test will fail when not in a git repo with remote configured
			// We're testing that the error is propagated correctly
			_, err := p.Create(ctx, "Test PR", "Test body")
			Expect(err).To(HaveOccurred())
		})

		It("returns error when gh CLI fails (with token)", func() {
			p := git.NewPRCreator("test-token")
			// This test will fail when not in a git repo with remote configured
			// We're testing that the error is propagated correctly
			_, err := p.Create(ctx, "Test PR", "Test body")
			Expect(err).To(HaveOccurred())
		})

		It("returns error when title starts with a dash", func() {
			p := git.NewPRCreator("")
			_, err := p.Create(ctx, "--title-injection", "body")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid PR title"))
		})

		It("returns error when title starts with single dash", func() {
			p := git.NewPRCreator("")
			_, err := p.Create(ctx, "-bad-title", "body")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid PR title"))
		})

		It("allows title that does not start with a dash", func() {
			p := git.NewPRCreator("")
			// Will fail due to no git remote, but not due to title validation
			_, err := p.Create(ctx, "Valid title", "body")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("invalid PR title"))
		})
	})

	Describe("FindOpenPR", func() {
		It("returns empty string when no open PR exists", func() {
			p := git.NewPRCreator("")
			url, err := p.FindOpenPR(ctx, "feature/nonexistent-branch")
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(BeEmpty())
		})

		It("returns error when gh CLI fails with token", func() {
			p := git.NewPRCreator("test-token")
			_, err := p.FindOpenPR(ctx, "feature/test-branch")
			Expect(err).To(HaveOccurred())
		})
	})
})
