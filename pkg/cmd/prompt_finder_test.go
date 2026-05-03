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
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("returns error for empty directory", func() {
		_, err := cmd.FindPromptFile(ctx, dir, "122")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
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
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("finds prompt by unpadded number", func() {
		writePrompt("063-fix-bug.md")
		result, err := cmd.FindPromptFile(ctx, dir, "63")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(filepath.Join(dir, "063-fix-bug.md")))
	})

	It("finds prompt by padded number matching zero-padded file", func() {
		writePrompt("063-fix-bug.md")
		result, err := cmd.FindPromptFile(ctx, dir, "063")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(filepath.Join(dir, "063-fix-bug.md")))
	})

	It("does not match 010-bar.md when input is 1", func() {
		writePrompt("001-foo.md")
		writePrompt("010-bar.md")
		result, err := cmd.FindPromptFile(ctx, dir, "1")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(filepath.Join(dir, "001-foo.md")))
	})

	It("returns ambiguity error when two prompts share the same numeric prefix", func() {
		writePrompt("001-foo.md")
		writePrompt("001-bar.md")
		_, err := cmd.FindPromptFile(ctx, dir, "001")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ambiguous prompt id 001"))
		Expect(err.Error()).To(ContainSubstring("001-foo.md"))
		Expect(err.Error()).To(ContainSubstring("001-bar.md"))
	})
})

var _ = Describe("FindPromptFileInDirs", func() {
	var (
		dir1 string
		dir2 string
		ctx  context.Context
	)

	BeforeEach(func() {
		var err error
		dir1, err = os.MkdirTemp("", "prompt-finder-dirs1-*")
		Expect(err).NotTo(HaveOccurred())
		dir2, err = os.MkdirTemp("", "prompt-finder-dirs2-*")
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(dir1)
		_ = os.RemoveAll(dir2)
	})

	writePromptIn := func(directory, name string) string {
		path := filepath.Join(directory, name)
		Expect(os.WriteFile(path, []byte("---\nstatus: draft\n---\n"), 0600)).To(Succeed())
		return path
	}

	It("finds prompt in first dir by basename", func() {
		path := writePromptIn(dir1, "122-fix-bug.md")
		result, err := cmd.FindPromptFileInDirs(ctx, "122-fix-bug.md", dir1, dir2)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(path))
	})

	It("finds prompt in second dir when not in first", func() {
		path := writePromptIn(dir2, "122-fix-bug.md")
		result, err := cmd.FindPromptFileInDirs(ctx, "122-fix-bug.md", dir1, dir2)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(path))
	})

	It("finds prompt by unpadded number across dirs", func() {
		path := writePromptIn(dir2, "063-bug-foo.md")
		result, err := cmd.FindPromptFileInDirs(ctx, "63", dir1, dir2)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(path))
	})

	It("returns error when not found in any dir", func() {
		_, err := cmd.FindPromptFileInDirs(ctx, "999-missing.md", dir1, dir2)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("prompt not found"))
	})

	It("returns ambiguity error when same numeric prefix exists in different dirs", func() {
		writePromptIn(dir1, "001-foo.md")
		writePromptIn(dir2, "001-bar.md")
		_, err := cmd.FindPromptFileInDirs(ctx, "1", dir1, dir2)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ambiguous prompt id 1"))
	})

	It("skips missing dirs silently", func() {
		path := writePromptIn(dir2, "003-spec.md")
		result, err := cmd.FindPromptFileInDirs(ctx, "003-spec.md", "/nonexistent/dir", dir2)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(path))
	})
})
