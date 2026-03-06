// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/cmd"
)

var _ = Describe("findSpecFile", func() {
	var (
		specsDir string
		ctx      context.Context
	)

	BeforeEach(func() {
		var err error
		specsDir, err = os.MkdirTemp("", "spec-finder-test-*")
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(specsDir)
	})

	writeSpec := func(name string) string {
		path := filepath.Join(specsDir, name)
		Expect(os.WriteFile(path, []byte("---\nstatus: draft\n---\n"), 0600)).To(Succeed())
		return path
	}

	It("finds spec by absolute path", func() {
		absPath := writeSpec("020-my-spec.md")
		result, err := cmd.FindSpecFile(ctx, specsDir, absPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(absPath))
	})

	It("finds spec by relative path with directory component", func() {
		absPath := writeSpec("020-my-spec.md")
		rel := filepath.Join(specsDir, "020-my-spec.md")
		result, err := cmd.FindSpecFile(ctx, specsDir, rel)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(absPath))
	})

	It("finds spec by exact filename with .md extension", func() {
		writeSpec("020-my-spec.md")
		result, err := cmd.FindSpecFile(ctx, specsDir, "020-my-spec.md")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(filepath.Join(specsDir, "020-my-spec.md")))
	})

	It("finds spec by filename without .md extension", func() {
		writeSpec("020-my-spec.md")
		result, err := cmd.FindSpecFile(ctx, specsDir, "020-my-spec")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(filepath.Join(specsDir, "020-my-spec.md")))
	})

	It("finds spec by numeric prefix", func() {
		writeSpec("020-my-spec.md")
		result, err := cmd.FindSpecFile(ctx, specsDir, "020")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(filepath.Join(specsDir, "020-my-spec.md")))
	})

	It("returns error when spec not found", func() {
		_, err := cmd.FindSpecFile(ctx, specsDir, "999")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("spec not found"))
	})

	It("returns error when absolute path does not exist", func() {
		_, err := cmd.FindSpecFile(ctx, specsDir, "/nonexistent/path/spec.md")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("spec not found"))
	})
})

var _ = Describe("FindSpecFileInDirs", func() {
	var (
		dir1 string
		dir2 string
		ctx  context.Context
	)

	BeforeEach(func() {
		var err error
		dir1, err = os.MkdirTemp("", "spec-finder-dirs1-*")
		Expect(err).NotTo(HaveOccurred())
		dir2, err = os.MkdirTemp("", "spec-finder-dirs2-*")
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(dir1)
		_ = os.RemoveAll(dir2)
	})

	It("finds spec in first dir", func() {
		path := filepath.Join(dir1, "001-spec.md")
		Expect(os.WriteFile(path, []byte("---\nstatus: draft\n---\n"), 0600)).To(Succeed())

		result, err := cmd.FindSpecFileInDirs(ctx, "001-spec.md", dir1, dir2)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(path))
	})

	It("finds spec in second dir when not in first", func() {
		path := filepath.Join(dir2, "002-spec.md")
		Expect(os.WriteFile(path, []byte("---\nstatus: draft\n---\n"), 0600)).To(Succeed())

		result, err := cmd.FindSpecFileInDirs(ctx, "002-spec.md", dir1, dir2)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(path))
	})

	It("returns error when not found in any dir", func() {
		_, err := cmd.FindSpecFileInDirs(ctx, "999-missing.md", dir1, dir2)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("spec not found"))
	})

	It("finds spec by numeric prefix in second dir", func() {
		path := filepath.Join(dir2, "042-my-spec.md")
		Expect(os.WriteFile(path, []byte("---\nstatus: draft\n---\n"), 0600)).To(Succeed())

		result, err := cmd.FindSpecFileInDirs(ctx, "042", dir1, dir2)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(path))
	})

	It("skips missing dirs silently", func() {
		path := filepath.Join(dir2, "003-spec.md")
		Expect(os.WriteFile(path, []byte("---\nstatus: draft\n---\n"), 0600)).To(Succeed())

		result, err := cmd.FindSpecFileInDirs(ctx, "003-spec.md", "/nonexistent/dir", dir2)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(path))
	})
})
