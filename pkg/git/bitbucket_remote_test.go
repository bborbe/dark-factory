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

var _ = Describe("ParseBitbucketRemoteURL", func() {
	var ctx context.Context
	BeforeEach(func() {
		ctx = context.Background()
	})

	DescribeTable(
		"SSH format",
		func(url, expectedProject, expectedRepo string) {
			coords, err := git.ParseBitbucketRemoteURL(ctx, url)
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
			coords, err := git.ParseBitbucketRemoteURL(ctx, url)
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
			_, err := git.ParseBitbucketRemoteURL(ctx, url)
			Expect(err).To(HaveOccurred())
		},
		Entry("github SSH URL", "git@github.com:owner/repo.git"),
		Entry("github HTTPS URL", "https://github.com/owner/repo.git"),
		Entry("empty string", ""),
		Entry("plain host only", "ssh://bitbucket.example.com:7999"),
		Entry("https without /scm/ segment", "https://bitbucket.example.com/bro/sentinel.git"),
	)
})
