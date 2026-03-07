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

var _ = Describe("FindPromptFile", func() {
	var (
		dir string
		ctx context.Context
	)

	BeforeEach(func() {
		var err error
		dir, err = os.MkdirTemp("", "prompt-finder-test-*")
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(dir)
	})

	writePrompt := func(name string) string {
		path := filepath.Join(dir, name)
		Expect(os.WriteFile(path, []byte("---\nstatus: draft\n---\n"), 0600)).To(Succeed())
		return path
	}

	It("finds prompt by exact filename with .md extension", func() {
		writePrompt("122-fix-bug.md")
		result, err := cmd.FindPromptFile(ctx, dir, "122-fix-bug.md")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(filepath.Join(dir, "122-fix-bug.md")))
	})

	It("finds prompt by filename without .md extension", func() {
		writePrompt("122-fix-bug.md")
		result, err := cmd.FindPromptFile(ctx, dir, "122-fix-bug")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(filepath.Join(dir, "122-fix-bug.md")))
	})

	It("finds prompt by numeric prefix only", func() {
		writePrompt("122-fix-bug.md")
		result, err := cmd.FindPromptFile(ctx, dir, "122")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(filepath.Join(dir, "122-fix-bug.md")))
	})

	It("returns error when no match found", func() {
		writePrompt("122-fix-bug.md")
		_, err := cmd.FindPromptFile(ctx, dir, "999")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("file not found"))
	})

	It("returns error for empty directory", func() {
		_, err := cmd.FindPromptFile(ctx, dir, "122")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("file not found"))
	})

	It("finds prompt by absolute path", func() {
		absPath := writePrompt("122-fix-bug.md")
		result, err := cmd.FindPromptFile(ctx, dir, absPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(absPath))
	})

	It("returns error for nonexistent absolute path", func() {
		_, err := cmd.FindPromptFile(ctx, dir, "/nonexistent/path/prompt.md")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("file not found"))
	})
})
