// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("truncateStderr", func() {
	It("returns short input unchanged (modulo trailing newline stripping)", func() {
		input := "short error message"
		result := git.TruncateStderrForTest(input)
		Expect(result).To(Equal(input))
	})

	It("strips trailing newlines from short input", func() {
		result := git.TruncateStderrForTest("error\n")
		Expect(result).To(Equal("error"))
	})

	It("returns input exactly 8192 bytes unchanged", func() {
		input := strings.Repeat("X", 8192)
		result := git.TruncateStderrForTest(input)
		Expect(result).To(Equal(input))
		Expect(len(result)).To(Equal(8192))
	})

	It(
		"truncates input exceeding 8192 bytes and appends (truncated), bounding output size",
		func() {
			input := strings.Repeat("X", 64*1024)
			result := git.TruncateStderrForTest(input)
			Expect(result).To(HaveSuffix(" (truncated)"))
			Expect(len(result)).To(BeNumerically("<", len(input)))
		},
	)
})
