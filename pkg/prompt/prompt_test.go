// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt_test

import (
	"context"
	"os"
	"path/filepath"

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

	Describe("ListQueued", func() {
		Context("with queued prompts", func() {
			BeforeEach(func() {
				// Create test files
				createPromptFile(tempDir, "001-first.md", "queued")
				createPromptFile(tempDir, "002-second.md", "queued")
				createPromptFile(tempDir, "003-third.md", "completed")
				createPromptFile(tempDir, "004-fourth.md", "")
			})

			It("returns only queued prompts sorted alphabetically", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(2))
				Expect(filepath.Base(prompts[0].Path)).To(Equal("001-first.md"))
				Expect(filepath.Base(prompts[1].Path)).To(Equal("002-second.md"))
			})
		})

		Context("with no queued prompts", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-first.md", "completed")
			})

			It("returns empty list", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(0))
			})
		})

		Context("with non-markdown files", func() {
			BeforeEach(func() {
				// Create a non-markdown file
				err := os.WriteFile(filepath.Join(tempDir, "readme.txt"), []byte("test"), 0600)
				Expect(err).To(BeNil())
				createPromptFile(tempDir, "001-first.md", "queued")
			})

			It("ignores non-markdown files", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(1))
			})
		})
	})

	Describe("SetStatus", func() {
		var path string

		BeforeEach(func() {
			path = createPromptFile(tempDir, "001-test.md", "queued")
		})

		It("updates status field", func() {
			err := prompt.SetStatus(ctx, path, "executing")
			Expect(err).To(BeNil())

			content, err := os.ReadFile(path)
			Expect(err).To(BeNil())
			Expect(string(content)).To(ContainSubstring("status: executing"))
		})
	})

	Describe("Title", func() {
		var path string

		BeforeEach(func() {
			content := `---
status: queued
---

# Implement Feature X

This is the content.
`
			path = filepath.Join(tempDir, "001-test.md")
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())
		})

		It("extracts first heading", func() {
			title, err := prompt.Title(ctx, path)
			Expect(err).To(BeNil())
			Expect(title).To(Equal("Implement Feature X"))
		})
	})

	Describe("Content", func() {
		var path string

		BeforeEach(func() {
			path = createPromptFile(tempDir, "001-test.md", "queued")
		})

		It("returns full file content", func() {
			content, err := prompt.Content(ctx, path)
			Expect(err).To(BeNil())
			Expect(content).To(ContainSubstring("status: queued"))
			Expect(content).To(ContainSubstring("# Test Prompt"))
		})
	})

	Describe("MoveToCompleted", func() {
		var path string

		BeforeEach(func() {
			path = createPromptFile(tempDir, "001-test.md", "queued")
		})

		It("moves file to completed subdirectory", func() {
			err := prompt.MoveToCompleted(ctx, path)
			Expect(err).To(BeNil())

			// Original file should not exist
			_, err = os.Stat(path)
			Expect(os.IsNotExist(err)).To(BeTrue())

			// File should exist in completed/
			completedPath := filepath.Join(tempDir, "completed", "001-test.md")
			_, err = os.Stat(completedPath)
			Expect(err).To(BeNil())
		})
	})
})

// Helper function to create a prompt file with given status
func createPromptFile(dir, filename, status string) string {
	content := "---\n"
	if status != "" {
		content += "status: " + status + "\n"
	}
	content += "---\n\n# Test Prompt\n\nContent here.\n"

	path := filepath.Join(dir, filename)
	err := os.WriteFile(path, []byte(content), 0600)
	if err != nil {
		panic(err)
	}
	return path
}
