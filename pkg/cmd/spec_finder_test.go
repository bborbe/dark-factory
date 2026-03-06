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
