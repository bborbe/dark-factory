// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
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
		It("returns error when command fails", func() {
			fakeRunner := &mocks.SubprocRunner{}
			fakeRunner.RunWithWarnAndTimeoutEnvReturns(nil, errCommand("command failed"))
			p := git.NewPRCreatorWithRunner("", fakeRunner)
			_, err := p.Create(ctx, "Test PR", "Test body", "dark-factory/test-branch")
			Expect(err).To(HaveOccurred())
		})

		It("returns PR URL on success", func() {
			fakeRunner := &mocks.SubprocRunner{}
			fakeRunner.RunWithWarnAndTimeoutEnvReturns(
				[]byte("https://github.com/owner/repo/pull/1\n"),
				nil,
			)
			p := git.NewPRCreatorWithRunner("", fakeRunner)
			url, err := p.Create(ctx, "Test PR", "Test body", "dark-factory/test-branch")
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("https://github.com/owner/repo/pull/1"))
		})

		It("returns error when title starts with a dash", func() {
			p := git.NewPRCreator("")
			_, err := p.Create(ctx, "--title-injection", "body", "dark-factory/test-branch")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid PR title"))
		})

		It("returns error when title starts with single dash", func() {
			p := git.NewPRCreator("")
			_, err := p.Create(ctx, "-bad-title", "body", "dark-factory/test-branch")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid PR title"))
		})

		It("allows title that does not start with a dash", func() {
			fakeRunner := &mocks.SubprocRunner{}
			fakeRunner.RunWithWarnAndTimeoutEnvReturns(
				[]byte("https://github.com/owner/repo/pull/1\n"),
				nil,
			)
			p := git.NewPRCreatorWithRunner("", fakeRunner)
			_, err := p.Create(ctx, "Valid title", "body", "dark-factory/test-branch")
			Expect(err).NotTo(HaveOccurred())
		})

		It("sets GH_TOKEN env when token provided", func() {
			fakeRunner := &mocks.SubprocRunner{}
			fakeRunner.RunWithWarnAndTimeoutEnvReturns(
				[]byte("https://github.com/owner/repo/pull/1\n"),
				nil,
			)
			p := git.NewPRCreatorWithRunner("my-token", fakeRunner)
			_, err := p.Create(ctx, "Test PR", "body", "dark-factory/test-branch")
			Expect(err).NotTo(HaveOccurred())
			_, _, _, extraEnv, _, _ := fakeRunner.RunWithWarnAndTimeoutEnvArgsForCall(0)
			Expect(extraEnv).To(ContainElement("GH_TOKEN=my-token"))
		})

		It("passes --head branch to gh pr create", func() {
			fakeRunner := &mocks.SubprocRunner{}
			fakeRunner.RunWithWarnAndTimeoutEnvReturns(
				[]byte("https://github.com/owner/repo/pull/1\n"),
				nil,
			)
			p := git.NewPRCreatorWithRunner("", fakeRunner)
			_, err := p.Create(ctx, "Test PR", "Test body", "dark-factory/my-branch")
			Expect(err).NotTo(HaveOccurred())
			_, _, _, _, _, args := fakeRunner.RunWithWarnAndTimeoutEnvArgsForCall(0)
			Expect(args).To(ContainElements("--head", "dark-factory/my-branch"))
		})
	})

	Describe("FindOpenPR", func() {
		It("returns empty string when no open PR exists", func() {
			fakeRunner := &mocks.SubprocRunner{}
			fakeRunner.RunWithWarnAndTimeoutEnvReturns([]byte(""), nil)
			p := git.NewPRCreatorWithRunner("", fakeRunner)
			url, err := p.FindOpenPR(ctx, "feature/nonexistent-branch")
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(BeEmpty())
		})

		It("returns PR URL when open PR exists", func() {
			fakeRunner := &mocks.SubprocRunner{}
			fakeRunner.RunWithWarnAndTimeoutEnvReturns(
				[]byte("https://github.com/owner/repo/pull/42\n"),
				nil,
			)
			p := git.NewPRCreatorWithRunner("", fakeRunner)
			url, err := p.FindOpenPR(ctx, "feature/my-branch")
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("https://github.com/owner/repo/pull/42"))
		})

		It("returns error when command fails", func() {
			fakeRunner := &mocks.SubprocRunner{}
			fakeRunner.RunWithWarnAndTimeoutEnvReturns(nil, errCommand("gh auth required"))
			p := git.NewPRCreatorWithRunner("test-token", fakeRunner)
			_, err := p.FindOpenPR(ctx, "feature/test-branch")
			Expect(err).To(HaveOccurred())
		})

		It("sets GH_TOKEN env when token provided", func() {
			fakeRunner := &mocks.SubprocRunner{}
			fakeRunner.RunWithWarnAndTimeoutEnvReturns([]byte(""), nil)
			p := git.NewPRCreatorWithRunner("my-token", fakeRunner)
			_, err := p.FindOpenPR(ctx, "feature/branch")
			Expect(err).NotTo(HaveOccurred())
			_, _, _, extraEnv, _, _ := fakeRunner.RunWithWarnAndTimeoutEnvArgsForCall(0)
			Expect(extraEnv).To(ContainElement("GH_TOKEN=my-token"))
		})
	})
})
