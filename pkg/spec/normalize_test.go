// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package spec_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/spec"
)

var _ = Describe("NormalizeSpecFilename", func() {
	var (
		ctx     context.Context
		tempDir string
		dir1    string
		dir2    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "normalize-spec-test-*")
		Expect(err).NotTo(HaveOccurred())
		dir1 = filepath.Join(tempDir, "dir1")
		dir2 = filepath.Join(tempDir, "dir2")
		Expect(os.MkdirAll(dir1, 0750)).To(Succeed())
		Expect(os.MkdirAll(dir2, 0750)).To(Succeed())
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	It("assigns 001- prefix when dirs are empty", func() {
		result, err := spec.NormalizeSpecFilename(ctx, "my-spec.md", dir1, dir2)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("001-my-spec.md"))
	})

	It("returns name unchanged when it already has a numeric prefix", func() {
		result, err := spec.NormalizeSpecFilename(ctx, "025-my-spec.md", dir1, dir2)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("025-my-spec.md"))
	})

	It("assigns next number after highest across all dirs", func() {
		Expect(os.WriteFile(filepath.Join(dir1, "003-spec-a.md"), []byte(""), 0600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir2, "007-spec-b.md"), []byte(""), 0600)).To(Succeed())

		result, err := spec.NormalizeSpecFilename(ctx, "new-spec.md", dir1, dir2)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("008-new-spec.md"))
	})

	It("scans across multiple dirs to find highest number", func() {
		Expect(os.WriteFile(filepath.Join(dir1, "010-spec-a.md"), []byte(""), 0600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir2, "005-spec-b.md"), []byte(""), 0600)).To(Succeed())

		result, err := spec.NormalizeSpecFilename(ctx, "another.md", dir1, dir2)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("011-another.md"))
	})

	It("ignores non-.md files when scanning", func() {
		Expect(os.WriteFile(filepath.Join(dir1, "002-spec.md"), []byte(""), 0600)).To(Succeed())
		Expect(
			os.WriteFile(filepath.Join(dir1, "099-notaspec.txt"), []byte(""), 0600),
		).To(Succeed())

		result, err := spec.NormalizeSpecFilename(ctx, "new.md", dir1)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("003-new.md"))
	})

	It("skips non-existent dirs without error", func() {
		result, err := spec.NormalizeSpecFilename(ctx, "new.md", "/nonexistent/dir", dir1)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("001-new.md"))
	})

	It("works with no dirs", func() {
		result, err := spec.NormalizeSpecFilename(ctx, "new.md")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("001-new.md"))
	})
})
