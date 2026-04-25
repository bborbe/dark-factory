// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt_test

import (
	"context"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("Prompt", func() {
	var (
		ctx     context.Context
		tempDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "prompt-test-*")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("Title", func() {
		Context("with frontmatter", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved
---

# Implement Feature X

This is the content.
`
				path = filepath.Join(tempDir, "001-test.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("extracts first heading", func() {
				title, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					Title(ctx, path)
				Expect(err).To(BeNil())
				Expect(title).To(Equal("Implement Feature X"))
			})
		})

		Context("without frontmatter", func() {
			var path string

			BeforeEach(func() {
				content := `# Implement Feature Y

This is the content.
`
				path = filepath.Join(tempDir, "001-plain.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("extracts first heading from start of file", func() {
				title, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					Title(ctx, path)
				Expect(err).To(BeNil())
				Expect(title).To(Equal("Implement Feature Y"))
			})
		})

		Context("without heading", func() {
			var path string

			BeforeEach(func() {
				content := "just some plain text without heading\n"
				path = filepath.Join(tempDir, "004-test.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("returns filename without extension", func() {
				title, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					Title(ctx, path)
				Expect(err).To(BeNil())
				Expect(title).To(Equal("004-test"))
			})
		})
	})

	Describe("Content", func() {
		Context("with content", func() {
			var path string

			BeforeEach(func() {
				path = createPromptFile(tempDir, "001-test.md", "approved")
			})

			It("returns content without frontmatter", func() {
				content, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					Content(ctx, path)
				Expect(err).To(BeNil())
				Expect(content).NotTo(ContainSubstring("status: approved"))
				Expect(content).To(ContainSubstring("# Test Prompt"))
			})
		})

		Context("with empty file", func() {
			var path string

			BeforeEach(func() {
				path = filepath.Join(tempDir, "empty.md")
				err := os.WriteFile(path, []byte(""), 0600)
				Expect(err).To(BeNil())
			})

			It("returns ErrEmptyPrompt", func() {
				_, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					Content(ctx, path)
				Expect(err).To(Equal(prompt.ErrEmptyPrompt))
			})
		})

		Context("with whitespace-only file", func() {
			var path string

			BeforeEach(func() {
				path = filepath.Join(tempDir, "whitespace.md")
				err := os.WriteFile(path, []byte("   \n\t\n  \n"), 0600)
				Expect(err).To(BeNil())
			})

			It("returns ErrEmptyPrompt", func() {
				_, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					Content(ctx, path)
				Expect(err).To(Equal(prompt.ErrEmptyPrompt))
			})
		})

		Context("with duplicate empty frontmatter", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved
---



---
---

# Actual prompt title

Prompt content here.
`
				path = filepath.Join(tempDir, "001-duplicate.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("strips the empty frontmatter block from content", func() {
				content, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					Content(ctx, path)
				Expect(err).To(BeNil())
				Expect(content).NotTo(ContainSubstring("status: approved"))
				Expect(content).NotTo(HavePrefix("---"))
				Expect(content).To(ContainSubstring("# Actual prompt title"))
				Expect(content).To(ContainSubstring("Prompt content here."))
			})
		})

		Context("with empty frontmatter block containing only whitespace", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved
---

---


---

# Test Prompt

Content here.
`
				path = filepath.Join(tempDir, "001-whitespace-fm.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("strips the whitespace-only frontmatter block", func() {
				content, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					Content(ctx, path)
				Expect(err).To(BeNil())
				Expect(content).NotTo(ContainSubstring("status: approved"))
				Expect(content).NotTo(HavePrefix("---"))
				Expect(content).To(ContainSubstring("# Test Prompt"))
			})
		})

		Context("with multiple consecutive empty frontmatter blocks", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved
---

---
---

---
---

# Test Prompt

Content here.
`
				path = filepath.Join(tempDir, "001-multiple.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("strips all empty frontmatter blocks", func() {
				content, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					Content(ctx, path)
				Expect(err).To(BeNil())
				Expect(content).NotTo(ContainSubstring("status: approved"))
				Expect(content).NotTo(HavePrefix("---"))
				Expect(content).To(ContainSubstring("# Test Prompt"))
			})
		})

		Context("with non-empty duplicate frontmatter", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved
---

---
title: Some Title
---

# Test Prompt

Content here.
`
				path = filepath.Join(tempDir, "001-nonempty.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("preserves non-empty frontmatter in content", func() {
				content, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					Content(ctx, path)
				Expect(err).To(BeNil())
				Expect(content).NotTo(ContainSubstring("status: approved"))
				Expect(content).To(ContainSubstring("---"))
				Expect(content).To(ContainSubstring("title: Some Title"))
				Expect(content).To(ContainSubstring("# Test Prompt"))
			})
		})

		Context("with empty frontmatter at EOF", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved
---

---
---`
				path = filepath.Join(tempDir, "001-eof.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("returns body as-is (body is preserved exactly as loaded)", func() {
				content, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					Content(ctx, path)
				Expect(err).To(BeNil())
				// Body contains the text after frontmatter, including the "---\n---"
				Expect(content).To(ContainSubstring("---"))
			})
		})
	})

})
