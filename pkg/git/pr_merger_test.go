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

var _ = Describe("PRMerger", func() {
	var ctx context.Context
	var merger git.PRMerger

	BeforeEach(func() {
		ctx = context.Background()
		merger = git.NewPRMerger("")
	})

	Describe("WaitAndMerge", func() {
		It("returns error when context is cancelled", func() {
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()

			err := merger.WaitAndMerge(cancelCtx, "https://github.com/owner/repo/pull/1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context cancelled"))
		})

		It("returns error for conflicting PR", func() {
			// This test would require mocking gh CLI output or using a real conflicting PR
			// For now, we'll skip it as it requires external dependencies
			Skip("Requires gh CLI mocking or integration test setup")
		})
	})
})
