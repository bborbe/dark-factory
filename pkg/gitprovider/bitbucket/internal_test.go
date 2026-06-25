// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bitbucket

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("parsePRID", func() {
	DescribeTable(
		"valid URLs",
		func(prURL string, expectedID int) {
			id, err := parsePRID(context.Background(), prURL)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(expectedID))
		},
		Entry(
			"standard URL",
			"https://bitbucket.example.com/projects/BRO/repos/sentinel/pull-requests/42",
			42,
		),
		Entry(
			"URL with trailing slash",
			"https://bitbucket.example.com/projects/BRO/repos/sentinel/pull-requests/42/",
			42,
		),
		Entry(
			"URL with /overview suffix",
			"https://bitbucket.example.com/projects/BRO/repos/sentinel/pull-requests/42/overview",
			42,
		),
		Entry(
			"PR ID 1",
			"https://bitbucket.example.com/projects/BRO/repos/sentinel/pull-requests/1",
			1,
		),
	)

	DescribeTable(
		"invalid URLs return error",
		func(prURL string) {
			_, err := parsePRID(context.Background(), prURL)
			Expect(err).To(HaveOccurred())
		},
		Entry("GitHub PR URL", "https://github.com/owner/repo/pull/42"),
		Entry(
			"no pull-requests segment",
			"https://bitbucket.example.com/projects/BRO/repos/sentinel",
		),
		Entry(
			"non-numeric ID",
			"https://bitbucket.example.com/projects/BRO/repos/sentinel/pull-requests/abc",
		),
		Entry("empty string", ""),
	)
})

var _ = Describe("redactToken", func() {
	It("replaces token occurrences with [REDACTED]", func() {
		result := redactToken("error: Bearer mysecrettoken in response", "mysecrettoken")
		Expect(result).NotTo(ContainSubstring("mysecrettoken"))
		Expect(result).To(ContainSubstring("[REDACTED]"))
	})

	It("is a no-op when token is empty", func() {
		result := redactToken("some error message", "")
		Expect(result).To(Equal("some error message"))
	})

	It("is a no-op when token does not appear in string", func() {
		result := redactToken("some error message", "notpresent")
		Expect(result).To(Equal("some error message"))
	})
})
