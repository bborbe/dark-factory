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

var _ = Describe("ValidateBranchName", func() {
	var ctx context.Context
	BeforeEach(func() {
		ctx = context.Background()
	})
	DescribeTable("valid branch names",
		func(name string) {
			Expect(git.ValidateBranchName(ctx, name)).To(Succeed())
		},
		Entry("simple alphanumeric", "feature"),
		Entry("with slash", "dark-factory/feature"),
		Entry("with dash", "feature-branch"),
		Entry("with dot", "release-1.0.0"),
		Entry("with underscore", "my_branch"),
		Entry("numeric start", "1.0.0"),
		Entry("full feature branch", "dark-factory/123-my-feature"),
	)

	DescribeTable("invalid branch names",
		func(name string) {
			Expect(git.ValidateBranchName(ctx, name)).To(HaveOccurred())
		},
		Entry("empty string", ""),
		Entry("leading dash", "-feature"),
		Entry("double dash flag", "--orphan"),
		Entry("semicolon injection", "; rm -rf"),
		Entry("command substitution", "$(evil)"),
		Entry("space in name", "feature branch"),
		Entry("newline", "feature\nbranch"),
		Entry("ampersand", "feature&evil"),
	)
})

var _ = Describe("ValidatePRTitle", func() {
	var ctx context.Context
	BeforeEach(func() {
		ctx = context.Background()
	})
	DescribeTable("valid PR titles",
		func(title string) {
			Expect(git.ValidatePRTitle(ctx, title)).To(Succeed())
		},
		Entry("normal title", "Add feature X"),
		Entry("title with numbers", "Fix issue #123"),
		Entry("title with colon", "feat: add validation"),
	)

	DescribeTable("invalid PR titles",
		func(title string) {
			Expect(git.ValidatePRTitle(ctx, title)).To(HaveOccurred())
		},
		Entry("empty title", ""),
		Entry("leading single dash", "-bad-title"),
		Entry("leading double dash", "--title-injection"),
	)
})
