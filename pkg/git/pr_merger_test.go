// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("decideMergeAction", func() {
	DescribeTable("maps mergeStateStatus to action",
		func(status string, wantMerge bool, wantErr bool) {
			shouldMerge, err := git.DecideMergeActionForTest(status)
			if wantErr {
				Expect(err).To(HaveOccurred())
				Expect(shouldMerge).To(BeFalse())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(shouldMerge).To(Equal(wantMerge))
			}
		},
		Entry("CLEAN → merge", "CLEAN", true, false),
		Entry("DIRTY → conflict error", "DIRTY", false, true),
		Entry("BLOCKED → keep polling", "BLOCKED", false, false),
		Entry("BEHIND → keep polling", "BEHIND", false, false),
		Entry("UNKNOWN → keep polling", "UNKNOWN", false, false),
		Entry("UNSTABLE → keep polling", "UNSTABLE", false, false),
		Entry("HAS_HOOKS → keep polling", "HAS_HOOKS", false, false),
		Entry("empty string → keep polling", "", false, false),
		Entry("MERGEABLE (old wrong value) → keep polling", "MERGEABLE", false, false),
		Entry("CONFLICTING (old wrong value) → keep polling", "CONFLICTING", false, false),
	)

	It("DIRTY error message mentions conflicts", func() {
		_, err := git.DecideMergeActionForTest("DIRTY")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("conflict"))
	})
})

var _ = Describe("PRMerger", func() {
	var ctx context.Context
	var merger git.PRMerger

	BeforeEach(func() {
		ctx = context.Background()
		merger = git.NewPRMerger("", libtime.NewCurrentDateTime())
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
