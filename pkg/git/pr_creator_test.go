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
	var (
		ctx context.Context
		p   git.PRCreator
	)

	BeforeEach(func() {
		ctx = context.Background()
		p = git.NewPRCreator()
	})

	Describe("Create", func() {
		It("returns error when gh CLI fails", func() {
			// This test will fail when not in a git repo with remote configured
			// We're testing that the error is propagated correctly
			_, err := p.Create(ctx, "Test PR", "Test body")
			Expect(err).To(HaveOccurred())
		})
	})
})
