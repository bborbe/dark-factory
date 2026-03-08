// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package specnum_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/specnum"
)

var _ = Describe("Parse", func() {
	DescribeTable("returns correct numeric prefix",
		func(input string, expected int) {
			Expect(specnum.Parse(input)).To(Equal(expected))
		},
		Entry("bare number", "019", 19),
		Entry("padded number", "0019", 19),
		Entry("full spec name", "019-review-fix-loop", 19),
		Entry("zero prefix", "001-something", 1),
		Entry("no prefix returns -1", "no-number-here", -1),
		Entry("empty string returns -1", "", -1),
	)
})
