// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bitbucket_test

import (
	"context"
	stderrors "errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/gitprovider/bitbucket"
)

var _ = Describe("ParseRemoteURL", func() {
	var ctx context.Context
	BeforeEach(func() {
		ctx = context.Background()
	})

	DescribeTable(
		"SSH format",
		func(url, expectedProject, expectedRepo string) {
			coords, err := bitbucket.ParseRemoteURL(ctx, url)
			Expect(err).NotTo(HaveOccurred())
			Expect(coords.Project).To(Equal(expectedProject))
			Expect(coords.Repo).To(Equal(expectedRepo))
		},
		Entry(
			"lowercase project and repo",
			"ssh://bitbucket.example.com:7999/bro/sentinel.git",
			"BRO",
			"sentinel",
		),
		Entry(
			"uppercase project key",
			"ssh://bitbucket.example.com:7999/BRO/sentinel.git",
			"BRO",
			"sentinel",
		),
		Entry(
			"without .git suffix",
			"ssh://bitbucket.example.com:7999/bro/sentinel",
			"BRO",
			"sentinel",
		),
		Entry(
			"mixed case repo slug is lowercased",
			"ssh://bitbucket.example.com:7999/bro/MyRepo.git",
			"BRO",
			"myrepo",
		),
	)

	DescribeTable(
		"HTTPS format",
		func(url, expectedProject, expectedRepo string) {
			coords, err := bitbucket.ParseRemoteURL(ctx, url)
			Expect(err).NotTo(HaveOccurred())
			Expect(coords.Project).To(Equal(expectedProject))
			Expect(coords.Repo).To(Equal(expectedRepo))
		},
		Entry(
			"standard HTTPS with /scm/ path",
			"https://bitbucket.example.com/scm/bro/sentinel.git",
			"BRO",
			"sentinel",
		),
		Entry(
			"http (not https) also accepted",
			"http://bitbucket.example.com/scm/bro/sentinel.git",
			"BRO",
			"sentinel",
		),
		Entry(
			"without .git suffix",
			"https://bitbucket.example.com/scm/bro/sentinel",
			"BRO",
			"sentinel",
		),
	)

	DescribeTable("invalid formats return error",
		func(url string) {
			_, err := bitbucket.ParseRemoteURL(ctx, url)
			Expect(err).To(HaveOccurred())
		},
		Entry("github SSH URL", "git@github.com:owner/repo.git"),
		Entry("github HTTPS URL", "https://github.com/owner/repo.git"),
		Entry("empty string", ""),
		Entry("plain host only", "ssh://bitbucket.example.com:7999"),
		Entry("https without /scm/ segment", "https://bitbucket.example.com/bro/sentinel.git"),
	)
})

var _ = Describe("ParseRemoteFromGit", func() {
	var ctx context.Context
	BeforeEach(func() {
		ctx = context.Background()
	})

	It("uses the injected runner to fetch the git remote URL", func() {
		fakeRunner := &mocks.SubprocRunner{}
		fakeRunner.RunWithWarnAndTimeoutReturns(
			[]byte("ssh://bitbucket.example.com:7999/bro/sentinel.git\n"),
			nil,
		)
		coords, err := bitbucket.ParseRemoteFromGit(ctx, fakeRunner, "origin")
		Expect(err).NotTo(HaveOccurred())
		Expect(coords.Project).To(Equal("BRO"))
		Expect(coords.Repo).To(Equal("sentinel"))
		Expect(fakeRunner.RunWithWarnAndTimeoutCallCount()).To(Equal(1))
	})

	It("returns error when the runner fails", func() {
		fakeRunner := &mocks.SubprocRunner{}
		fakeRunner.RunWithWarnAndTimeoutReturns(nil, stderrors.New("git not found"))
		_, err := bitbucket.ParseRemoteFromGit(ctx, fakeRunner, "origin")
		Expect(err).To(HaveOccurred())
	})

	It("returns error when URL does not match Bitbucket Server format", func() {
		fakeRunner := &mocks.SubprocRunner{}
		fakeRunner.RunWithWarnAndTimeoutReturns([]byte("https://github.com/owner/repo.git\n"), nil)
		_, err := bitbucket.ParseRemoteFromGit(ctx, fakeRunner, "origin")
		Expect(err).To(HaveOccurred())
	})
})
