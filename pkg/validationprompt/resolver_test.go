// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package validationprompt_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/validationprompt"
)

var _ = Describe("Resolver", func() {
	var (
		ctx      context.Context
		resolver validationprompt.Resolver
	)

	BeforeEach(func() {
		ctx = context.Background()
		resolver = validationprompt.NewResolver()
	})

	Describe("Resolve", func() {
		It("returns empty/false/nil for empty value", func() {
			text, ok, err := resolver.Resolve(ctx, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
			Expect(text).To(BeEmpty())
		})

		It("returns inline text for non-path value", func() {
			text, ok, err := resolver.Resolve(ctx, "# My Criteria\n- check this")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(text).To(Equal("# My Criteria\n- check this"))
		})

		It("returns file contents for existing file path", func() {
			tempDir := GinkgoT().TempDir()
			criteriaPath := filepath.Join(tempDir, "criteria.md")
			Expect(
				os.WriteFile(criteriaPath, []byte("# File Criteria\n- check this"), 0600),
			).To(Succeed())

			text, ok, err := resolver.Resolve(ctx, criteriaPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(text).To(Equal("# File Criteria\n- check this"))
		})

		It("returns false/nil for path-shaped missing file", func() {
			text, ok, err := resolver.Resolve(ctx, "/nonexistent/path/criteria.md")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
			Expect(text).To(BeEmpty())
		})

		It("returns false/nil for .md filename without separator that doesn't exist", func() {
			text, ok, err := resolver.Resolve(ctx, "criteria.md")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
			Expect(text).To(BeEmpty())
		})

		It("returns error when file exists but cannot be read", func() {
			tempDir := GinkgoT().TempDir()
			criteriaPath := filepath.Join(tempDir, "criteria.md")
			Expect(os.WriteFile(criteriaPath, []byte("content"), 0600)).To(Succeed())
			Expect(os.Chmod(criteriaPath, 0000)).To(Succeed())
			DeferCleanup(func() {
				_ = os.Chmod(criteriaPath, 0600)
			})

			_, ok, err := resolver.Resolve(ctx, criteriaPath)
			Expect(err).To(HaveOccurred())
			Expect(ok).To(BeFalse())
		})
	})
})
