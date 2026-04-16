// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

// simpleMover is a test implementation that uses os.Rename
type simpleMover struct{}

func (s *simpleMover) MoveFile(ctx context.Context, oldPath string, newPath string) error {
	return os.Rename(oldPath, newPath)
}

var _ = Describe("Prompt", func() {
	var (
		ctx     context.Context
		tempDir string
		mover   prompt.FileMover
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "prompt-test-*")
		Expect(err).To(BeNil())
		mover = &simpleMover{}
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("Status.Validate", func() {
		Context("with valid statuses", func() {
			It("accepts idea", func() {
				err := prompt.IdeaPromptStatus.Validate(ctx)
				Expect(err).To(BeNil())
			})

			It("accepts queued", func() {
				err := prompt.ApprovedPromptStatus.Validate(ctx)
				Expect(err).To(BeNil())
			})

			It("accepts executing", func() {
				err := prompt.ExecutingPromptStatus.Validate(ctx)
				Expect(err).To(BeNil())
			})

			It("accepts completed", func() {
				err := prompt.CompletedPromptStatus.Validate(ctx)
				Expect(err).To(BeNil())
			})

			It("accepts failed", func() {
				err := prompt.FailedPromptStatus.Validate(ctx)
				Expect(err).To(BeNil())
			})

			It("accepts in_review", func() {
				err := prompt.InReviewPromptStatus.Validate(ctx)
				Expect(err).To(BeNil())
			})
		})

		Context("with invalid status", func() {
			It("rejects unknown status", func() {
				err := prompt.PromptStatus("invalid").Validate(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("status(invalid) is invalid"))
			})

			It("rejects empty status", func() {
				err := prompt.PromptStatus("").Validate(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("status() is invalid"))
			})
		})
	})

	Describe("Prompt.Validate", func() {
		Context("with valid prompt", func() {
			It("accepts valid prompt with numbered filename", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "001-test.md"),
					Status: prompt.ApprovedPromptStatus,
				}
				err := p.Validate(ctx)
				Expect(err).To(BeNil())
			})
		})

		Context("with invalid prompts", func() {
			It("rejects empty path", func() {
				p := prompt.Prompt{
					Path:   "",
					Status: prompt.ApprovedPromptStatus,
				}
				err := p.Validate(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("path"))
			})

			It("rejects invalid status", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "001-test.md"),
					Status: prompt.PromptStatus("invalid"),
				}
				err := p.Validate(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("status"))
			})

			It("rejects filename without number prefix", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "test.md"),
					Status: prompt.ApprovedPromptStatus,
				}
				err := p.Validate(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("missing NNN- prefix"))
			})

			It("rejects filename with wrong format (single digit)", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "1-test.md"),
					Status: prompt.ApprovedPromptStatus,
				}
				err := p.Validate(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("missing NNN- prefix"))
			})

			It("rejects filename with wrong format (two digits)", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "42-test.md"),
					Status: prompt.ApprovedPromptStatus,
				}
				err := p.Validate(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("missing NNN- prefix"))
			})
		})
	})

	Describe("Prompt.ValidateForExecution", func() {
		Context("with queued status", func() {
			It("accepts prompt ready for execution", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "001-test.md"),
					Status: prompt.ApprovedPromptStatus,
				}
				err := p.ValidateForExecution(ctx)
				Expect(err).To(BeNil())
			})
		})

		Context("with non-queued status", func() {
			It("rejects executing prompt", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "001-test.md"),
					Status: prompt.ExecutingPromptStatus,
				}
				err := p.ValidateForExecution(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("expected status approved"))
			})

			It("rejects completed prompt", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "001-test.md"),
					Status: prompt.CompletedPromptStatus,
				}
				err := p.ValidateForExecution(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("expected status approved"))
			})

			It("rejects failed prompt", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "001-test.md"),
					Status: prompt.FailedPromptStatus,
				}
				err := p.ValidateForExecution(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("expected status approved"))
			})
		})

		Context("with invalid prompt", func() {
			It("rejects prompt without number prefix", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "test.md"),
					Status: prompt.ApprovedPromptStatus,
				}
				err := p.ValidateForExecution(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("missing NNN- prefix"))
			})
		})
	})

	Describe("Prompt.Number", func() {
		It("extracts number from valid filename", func() {
			p := prompt.Prompt{
				Path: filepath.Join(tempDir, "001-test.md"),
			}
			Expect(p.Number()).To(Equal(1))
		})

		It("extracts larger number", func() {
			p := prompt.Prompt{
				Path: filepath.Join(tempDir, "042-test.md"),
			}
			Expect(p.Number()).To(Equal(42))
		})

		It("returns -1 for filename without number", func() {
			p := prompt.Prompt{
				Path: filepath.Join(tempDir, "test.md"),
			}
			Expect(p.Number()).To(Equal(-1))
		})

		It("returns -1 for invalid number format", func() {
			p := prompt.Prompt{
				Path: filepath.Join(tempDir, "1-test.md"),
			}
			Expect(p.Number()).To(Equal(-1))
		})
	})

	Describe("AllPreviousCompleted", func() {
		Context("with no previous prompts", func() {
			It("returns true for n=1", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 1)
				Expect(result).To(BeTrue())
			})
		})

		Context("with all previous prompts completed", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())
				createPromptFile(completedDir, "001-first.md", "completed")
				createPromptFile(completedDir, "002-second.md", "completed")
				createPromptFile(completedDir, "003-third.md", "completed")
			})

			It("returns true for n=4", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 4)
				Expect(result).To(BeTrue())
			})

			It("returns true for n=2", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 2)
				Expect(result).To(BeTrue())
			})
		})

		Context("with gap in completed prompts", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())
				createPromptFile(completedDir, "001-first.md", "completed")
				createPromptFile(completedDir, "003-third.md", "completed")
				// Missing 002
			})

			It("returns false for n=3", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 3)
				Expect(result).To(BeFalse())
			})

			It("returns false for n=4", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 4)
				Expect(result).To(BeFalse())
			})

			It("returns true for n=2", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 2)
				Expect(result).To(BeTrue())
			})
		})

		Context("with no completed directory", func() {
			It("returns false for n=2", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 2)
				Expect(result).To(BeFalse())
			})

			It("returns true for n=1", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 1)
				Expect(result).To(BeTrue())
			})
		})

		Context("with empty completed directory", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())
			})

			It("returns false for n=2", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 2)
				Expect(result).To(BeFalse())
			})
		})

		Context("with files lacking frontmatter", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())
				// File with no frontmatter at all
				path := filepath.Join(completedDir, "001-no-frontmatter.md")
				err = os.WriteFile(path, []byte("# Test Prompt\n\nContent here.\n"), 0600)
				Expect(err).To(BeNil())
				// Normal file with frontmatter
				createPromptFile(completedDir, "002-normal.md", "completed")
			})

			It("counts files without frontmatter as completed", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 3)
				Expect(result).To(BeTrue())
			})
		})

		Context("with files having wrong status in frontmatter", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())
				// File in completed/ with status: failed (should still count as completed)
				createPromptFile(completedDir, "001-wrong-status.md", "failed")
				createPromptFile(completedDir, "002-another.md", "approved")
			})

			It("counts all files in completed/ as completed regardless of status field", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 3)
				Expect(result).To(BeTrue())
			})
		})

		Context("with mix of frontmatter and no-frontmatter files", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())
				// Mix of different file types
				path1 := filepath.Join(completedDir, "001-no-fm.md")
				err = os.WriteFile(path1, []byte("# Test 1\n"), 0600)
				Expect(err).To(BeNil())
				createPromptFile(completedDir, "002-with-status.md", "completed")
				path3 := filepath.Join(completedDir, "003-empty-fm.md")
				err = os.WriteFile(path3, []byte("---\n---\n# Test 3\n"), 0600)
				Expect(err).To(BeNil())
				createPromptFile(completedDir, "004-normal.md", "completed")
			})

			It("counts all files as completed", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 5)
				Expect(result).To(BeTrue())
			})

			It("detects gaps correctly even with mixed file types", func() {
				// Remove file 003 to create a gap
				completedDir := filepath.Join(tempDir, "completed")
				err := os.Remove(filepath.Join(completedDir, "003-empty-fm.md"))
				Expect(err).To(BeNil())

				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 4)
				Expect(result).To(BeFalse())
			})
		})
	})

	Describe("FindMissingCompleted", func() {
		Context("with no previous prompts", func() {
			It("returns nil for n=1", func() {
				result := prompt.FindMissingCompleted(ctx, filepath.Join(tempDir, "completed"), 1)
				Expect(result).To(BeNil())
			})
		})

		Context("with all previous prompts completed", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())
				createPromptFile(completedDir, "001-first.md", "completed")
				createPromptFile(completedDir, "002-second.md", "completed")
				createPromptFile(completedDir, "003-third.md", "completed")
			})

			It("returns nil for n=4", func() {
				result := prompt.FindMissingCompleted(ctx, filepath.Join(tempDir, "completed"), 4)
				Expect(result).To(BeNil())
			})

			It("returns nil for n=2", func() {
				result := prompt.FindMissingCompleted(ctx, filepath.Join(tempDir, "completed"), 2)
				Expect(result).To(BeNil())
			})
		})

		Context("with gap in completed prompts", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())
				createPromptFile(completedDir, "001-first.md", "completed")
				// Missing 002
				createPromptFile(completedDir, "003-third.md", "completed")
			})

			It("returns missing numbers for n=4", func() {
				result := prompt.FindMissingCompleted(ctx, filepath.Join(tempDir, "completed"), 4)
				Expect(result).To(Equal([]int{2}))
			})

			It("returns sorted missing numbers when multiple are missing", func() {
				result := prompt.FindMissingCompleted(ctx, filepath.Join(tempDir, "completed"), 5)
				Expect(result).To(Equal([]int{2, 4}))
			})
		})

		Context("with empty completed directory", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())
			})

			It("returns all missing for n=3", func() {
				result := prompt.FindMissingCompleted(ctx, filepath.Join(tempDir, "completed"), 3)
				Expect(result).To(Equal([]int{1, 2}))
			})
		})

		Context("with no completed directory", func() {
			It("returns all missing for n=3", func() {
				result := prompt.FindMissingCompleted(ctx, filepath.Join(tempDir, "completed"), 3)
				Expect(result).To(Equal([]int{1, 2}))
			})
		})
	})

	Describe("FindPromptStatus", func() {
		var inProgressDir string

		BeforeEach(func() {
			inProgressDir = filepath.Join(tempDir, "in-progress")
			err := os.MkdirAll(inProgressDir, 0750)
			Expect(err).To(BeNil())
		})

		Context("when prompt is found", func() {
			BeforeEach(func() {
				createPromptFile(inProgressDir, "083-some-prompt.md", "failed")
			})

			It("returns the status", func() {
				result := prompt.FindPromptStatus(ctx, inProgressDir, 83)
				Expect(result).To(Equal("failed"))
			})
		})

		Context("when prompt has executing status", func() {
			BeforeEach(func() {
				createPromptFile(inProgressDir, "042-another-prompt.md", "executing")
			})

			It("returns executing", func() {
				result := prompt.FindPromptStatus(ctx, inProgressDir, 42)
				Expect(result).To(Equal("executing"))
			})
		})

		Context("when prompt is not found", func() {
			It("returns empty string", func() {
				result := prompt.FindPromptStatus(ctx, inProgressDir, 99)
				Expect(result).To(Equal(""))
			})
		})

		Context("when directory does not exist", func() {
			It("returns empty string", func() {
				result := prompt.FindPromptStatus(ctx, filepath.Join(tempDir, "nonexistent"), 1)
				Expect(result).To(Equal(""))
			})
		})
	})

	Describe("ListQueued", func() {
		Context("with explicit status: approved", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-first.md", "approved")
				createPromptFile(tempDir, "002-second.md", "approved")
			})

			It("returns prompts sorted alphabetically", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(2))
				Expect(filepath.Base(prompts[0].Path)).To(Equal("001-first.md"))
				Expect(filepath.Base(prompts[1].Path)).To(Equal("002-second.md"))
			})
		})

		Context("with no frontmatter at all", func() {
			BeforeEach(func() {
				// Plain markdown file with no frontmatter
				content := "# Test Prompt\n\nContent here.\n"
				path := filepath.Join(tempDir, "001-plain.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("picks up the file", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(1))
				Expect(filepath.Base(prompts[0].Path)).To(Equal("001-plain.md"))
			})
		})

		Context("with frontmatter but no status field", func() {
			BeforeEach(func() {
				// Frontmatter with other fields but no status
				content := "---\nauthor: alice\n---\n\n# Test Prompt\n\nContent here.\n"
				path := filepath.Join(tempDir, "001-no-status.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("picks up the file", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(1))
				Expect(filepath.Base(prompts[0].Path)).To(Equal("001-no-status.md"))
			})
		})

		Context("with skip statuses", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-executing.md", "executing")
				createPromptFile(tempDir, "002-completed.md", "completed")
				createPromptFile(tempDir, "003-failed.md", "failed")
			})

			It("does not return files with skip status", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(0))
			})
		})

		Context("with mixed files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				createPromptFile(tempDir, "002-completed.md", "completed")
				// Plain file with no frontmatter
				content := "# Plain Prompt\n\nContent here.\n"
				path := filepath.Join(tempDir, "003-plain.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
				createPromptFile(tempDir, "004-executing.md", "executing")
			})

			It("returns queued and plain files only", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(2))
				Expect(filepath.Base(prompts[0].Path)).To(Equal("001-queued.md"))
				Expect(filepath.Base(prompts[1].Path)).To(Equal("003-plain.md"))
			})
		})

		Context("with non-markdown files", func() {
			BeforeEach(func() {
				// Create a non-markdown file
				err := os.WriteFile(filepath.Join(tempDir, "readme.txt"), []byte("test"), 0600)
				Expect(err).To(BeNil())
				createPromptFile(tempDir, "001-first.md", "approved")
			})

			It("ignores non-markdown files", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(1))
			})
		})
	})

	Describe("SetStatus", func() {
		Context("with existing frontmatter", func() {
			var path string

			BeforeEach(func() {
				path = createPromptFile(tempDir, "001-test.md", "approved")
			})

			It("updates status field", func() {
				err := prompt.SetStatus(ctx, path, "executing", libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())

				content, err := os.ReadFile(path)
				Expect(err).To(BeNil())
				Expect(string(content)).To(ContainSubstring("status: executing"))
			})
		})

		Context("without frontmatter", func() {
			var path string

			BeforeEach(func() {
				// Plain markdown file with no frontmatter
				content := "# Test Prompt\n\nContent here.\n"
				path = filepath.Join(tempDir, "001-plain.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("adds frontmatter with status", func() {
				err := prompt.SetStatus(ctx, path, "executing", libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())

				content, err := os.ReadFile(path)
				Expect(err).To(BeNil())
				contentStr := string(content)
				Expect(contentStr).To(ContainSubstring("---\n"))
				Expect(contentStr).To(ContainSubstring("status: executing"))
				Expect(contentStr).To(ContainSubstring("# Test Prompt"))
			})
		})
	})

	Describe("SetContainer", func() {
		Context("with existing frontmatter", func() {
			var path string

			BeforeEach(func() {
				path = createPromptFile(tempDir, "001-test.md", "approved")
			})

			It("adds container field", func() {
				err := prompt.SetContainer(
					ctx,
					path,
					"dark-factory-001-test",
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())

				fm, err := prompt.ReadFrontmatter(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(fm.Container).To(Equal("dark-factory-001-test"))
				Expect(fm.Status).To(Equal("approved")) // Status should be preserved
			})
		})

		Context("without frontmatter", func() {
			var path string

			BeforeEach(func() {
				// Plain markdown file with no frontmatter
				content := "# Test Prompt\n\nContent here.\n"
				path = filepath.Join(tempDir, "001-plain.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("adds frontmatter with container field", func() {
				err := prompt.SetContainer(
					ctx,
					path,
					"dark-factory-001-plain",
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())

				fm, err := prompt.ReadFrontmatter(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(fm.Container).To(Equal("dark-factory-001-plain"))
			})
		})

		Context("with existing container field", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved
container: old-container
---

# Test Prompt

Content here.
`
				path = filepath.Join(tempDir, "001-test.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("updates container field", func() {
				err := prompt.SetContainer(ctx, path, "new-container", libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())

				fm, err := prompt.ReadFrontmatter(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(fm.Container).To(Equal("new-container"))
				Expect(fm.Status).To(Equal("approved")) // Status should be preserved
			})
		})
	})

	Describe("SetVersion", func() {
		Context("with existing frontmatter", func() {
			var path string

			BeforeEach(func() {
				path = createPromptFile(tempDir, "001-test.md", "approved")
			})

			It("adds version field", func() {
				err := prompt.SetVersion(ctx, path, "v0.2.37", libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())

				fm, err := prompt.ReadFrontmatter(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(fm.DarkFactoryVersion).To(Equal("v0.2.37"))
				Expect(fm.Status).To(Equal("approved")) // Status should be preserved
			})
		})

		Context("without frontmatter", func() {
			var path string

			BeforeEach(func() {
				// Plain markdown file with no frontmatter
				content := "# Test Prompt\n\nContent here.\n"
				path = filepath.Join(tempDir, "001-plain.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("adds frontmatter with version field", func() {
				err := prompt.SetVersion(ctx, path, "v0.1.0", libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())

				fm, err := prompt.ReadFrontmatter(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(fm.DarkFactoryVersion).To(Equal("v0.1.0"))
			})
		})

		Context("with existing version field", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved
dark-factory-version: v0.1.0
---

# Test Prompt

Content here.
`
				path = filepath.Join(tempDir, "001-test.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("updates version field", func() {
				err := prompt.SetVersion(ctx, path, "v0.2.0", libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())

				fm, err := prompt.ReadFrontmatter(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(fm.DarkFactoryVersion).To(Equal("v0.2.0"))
				Expect(fm.Status).To(Equal("approved")) // Status should be preserved
			})
		})

		Context("version persists through move to completed", func() {
			var path string

			BeforeEach(func() {
				path = createPromptFile(tempDir, "001-test.md", "approved")
			})

			It("preserves version when moved to completed", func() {
				// Set container and version
				err := prompt.SetContainer(
					ctx,
					path,
					"dark-factory-001-test",
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())

				err = prompt.SetVersion(ctx, path, "v0.5.0", libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())

				// Move to completed
				completedDir := filepath.Join(tempDir, "completed")
				err = prompt.MoveToCompleted(
					ctx,
					path,
					completedDir,
					mover,
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())

				// Verify version is preserved in completed file
				completedPath := filepath.Join(tempDir, "completed", "001-test.md")
				fm, err := prompt.ReadFrontmatter(ctx, completedPath, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("completed"))
				Expect(fm.Container).To(Equal("dark-factory-001-test"))
				Expect(fm.DarkFactoryVersion).To(Equal("v0.5.0"))
			})
		})
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
				title, err := prompt.Title(ctx, path, libtime.NewCurrentDateTime())
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
				title, err := prompt.Title(ctx, path, libtime.NewCurrentDateTime())
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
				title, err := prompt.Title(ctx, path, libtime.NewCurrentDateTime())
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
				content, err := prompt.Content(ctx, path, libtime.NewCurrentDateTime())
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
				_, err := prompt.Content(ctx, path, libtime.NewCurrentDateTime())
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
				_, err := prompt.Content(ctx, path, libtime.NewCurrentDateTime())
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
				content, err := prompt.Content(ctx, path, libtime.NewCurrentDateTime())
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
				content, err := prompt.Content(ctx, path, libtime.NewCurrentDateTime())
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
				content, err := prompt.Content(ctx, path, libtime.NewCurrentDateTime())
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
				content, err := prompt.Content(ctx, path, libtime.NewCurrentDateTime())
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
				content, err := prompt.Content(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				// Body contains the text after frontmatter, including the "---\n---"
				Expect(content).To(ContainSubstring("---"))
			})
		})
	})

	Describe("MoveToCompleted", func() {
		var path string

		BeforeEach(func() {
			path = createPromptFile(tempDir, "001-test.md", "approved")
		})

		It("moves file to completed subdirectory", func() {
			completedDir := filepath.Join(tempDir, "completed")
			err := prompt.MoveToCompleted(
				ctx,
				path,
				completedDir,
				mover,
				libtime.NewCurrentDateTime(),
			)
			Expect(err).To(BeNil())

			// Original file should not exist
			_, err = os.Stat(path)
			Expect(os.IsNotExist(err)).To(BeTrue())

			// File should exist in completed/
			completedPath := filepath.Join(tempDir, "completed", "001-test.md")
			_, err = os.Stat(completedPath)
			Expect(err).To(BeNil())
		})

		It("sets status to completed before moving", func() {
			completedDir := filepath.Join(tempDir, "completed")
			err := prompt.MoveToCompleted(
				ctx,
				path,
				completedDir,
				mover,
				libtime.NewCurrentDateTime(),
			)
			Expect(err).To(BeNil())

			// Read completed file and verify status
			completedPath := filepath.Join(tempDir, "completed", "001-test.md")
			fm, err := prompt.ReadFrontmatter(ctx, completedPath, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(fm.Status).To(Equal("completed"))
		})
	})

	Describe("HasExecuting", func() {
		Context("with executing prompt", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				createPromptFile(tempDir, "002-executing.md", "executing")
				createPromptFile(tempDir, "003-completed.md", "completed")
			})

			It("returns true", func() {
				result := prompt.HasExecuting(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(result).To(BeTrue())
			})
		})

		Context("with multiple executing prompts", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-executing.md", "executing")
				createPromptFile(tempDir, "002-executing.md", "executing")
			})

			It("returns true", func() {
				result := prompt.HasExecuting(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(result).To(BeTrue())
			})
		})

		Context("without executing prompt", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				createPromptFile(tempDir, "002-completed.md", "completed")
				createPromptFile(tempDir, "003-failed.md", "failed")
			})

			It("returns false", func() {
				result := prompt.HasExecuting(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(result).To(BeFalse())
			})
		})

		Context("with empty directory", func() {
			It("returns false", func() {
				result := prompt.HasExecuting(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(result).To(BeFalse())
			})
		})

		Context("with non-markdown files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				// Create a non-markdown file
				err := os.WriteFile(filepath.Join(tempDir, "readme.txt"), []byte("test"), 0600)
				Expect(err).To(BeNil())
			})

			It("ignores non-markdown files and returns false", func() {
				result := prompt.HasExecuting(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(result).To(BeFalse())
			})
		})

		Context("with invalid frontmatter", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				// Create file with invalid frontmatter
				invalidContent := "---\ninvalid yaml content ][[\n---\n# Test\n"
				err := os.WriteFile(
					filepath.Join(tempDir, "002-invalid.md"),
					[]byte(invalidContent),
					0600,
				)
				Expect(err).To(BeNil())
			})

			It("skips files with errors and returns false", func() {
				result := prompt.HasExecuting(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(result).To(BeFalse())
			})
		})

		Context("with nonexistent directory", func() {
			It("returns false", func() {
				result := prompt.HasExecuting(
					ctx,
					"/nonexistent/path",
					libtime.NewCurrentDateTime(),
				)
				Expect(result).To(BeFalse())
			})
		})
	})

	Describe("ResetExecuting", func() {
		Context("with mixed statuses", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				createPromptFile(tempDir, "002-executing.md", "executing")
				createPromptFile(tempDir, "003-completed.md", "completed")
				createPromptFile(tempDir, "004-executing.md", "executing")
				createPromptFile(tempDir, "005-failed.md", "failed")
			})

			It("resets only executing prompts to queued", func() {
				err := prompt.ResetExecuting(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())

				// Check that executing prompts are now queued
				fm, err := prompt.ReadFrontmatter(
					ctx,
					filepath.Join(tempDir, "002-executing.md"),
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				fm, err = prompt.ReadFrontmatter(
					ctx,
					filepath.Join(tempDir, "004-executing.md"),
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				// Check that other statuses are unchanged
				fm, err = prompt.ReadFrontmatter(
					ctx,
					filepath.Join(tempDir, "001-queued.md"),
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				fm, err = prompt.ReadFrontmatter(
					ctx,
					filepath.Join(tempDir, "003-completed.md"),
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("completed"))

				fm, err = prompt.ReadFrontmatter(
					ctx,
					filepath.Join(tempDir, "005-failed.md"),
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("failed"))
			})
		})

		Context("with no executing prompts", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				createPromptFile(tempDir, "002-completed.md", "completed")
			})

			It("does nothing", func() {
				err := prompt.ResetExecuting(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())

				// Verify statuses are unchanged
				fm, err := prompt.ReadFrontmatter(
					ctx,
					filepath.Join(tempDir, "001-queued.md"),
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				fm, err = prompt.ReadFrontmatter(
					ctx,
					filepath.Join(tempDir, "002-completed.md"),
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("completed"))
			})
		})

		Context("with empty directory", func() {
			It("does nothing", func() {
				err := prompt.ResetExecuting(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
			})
		})
	})

	Describe("ResetFailed", func() {
		Context("with mixed statuses", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				createPromptFile(tempDir, "002-executing.md", "executing")
				createPromptFile(tempDir, "003-completed.md", "completed")
				createPromptFile(tempDir, "004-failed.md", "failed")
				createPromptFile(tempDir, "005-failed.md", "failed")
			})

			It("resets only failed prompts to queued", func() {
				err := prompt.ResetFailed(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())

				// Check that failed prompts are now queued
				fm, err := prompt.ReadFrontmatter(
					ctx,
					filepath.Join(tempDir, "004-failed.md"),
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				fm, err = prompt.ReadFrontmatter(
					ctx,
					filepath.Join(tempDir, "005-failed.md"),
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				// Check that other statuses are unchanged
				fm, err = prompt.ReadFrontmatter(
					ctx,
					filepath.Join(tempDir, "001-queued.md"),
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				fm, err = prompt.ReadFrontmatter(
					ctx,
					filepath.Join(tempDir, "002-executing.md"),
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("executing"))

				fm, err = prompt.ReadFrontmatter(
					ctx,
					filepath.Join(tempDir, "003-completed.md"),
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("completed"))
			})
		})

		Context("with no failed prompts", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				createPromptFile(tempDir, "002-completed.md", "completed")
			})

			It("does nothing", func() {
				err := prompt.ResetFailed(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())

				// Verify statuses are unchanged
				fm, err := prompt.ReadFrontmatter(
					ctx,
					filepath.Join(tempDir, "001-queued.md"),
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				fm, err = prompt.ReadFrontmatter(
					ctx,
					filepath.Join(tempDir, "002-completed.md"),
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("completed"))
			})
		})

		Context("with empty directory", func() {
			It("does nothing", func() {
				err := prompt.ResetFailed(ctx, tempDir, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
			})
		})
	})

	Describe("splitFrontmatter edge cases", func() {
		Context("with inline --- in content", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved
---

# Test Prompt

This content has --- inline which should not be confused with frontmatter.

More content here.
`
				path = filepath.Join(tempDir, "001-inline.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("correctly extracts frontmatter and content", func() {
				fm, err := prompt.ReadFrontmatter(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				content, err := prompt.Content(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("# Test Prompt"))
				Expect(content).To(ContainSubstring("This content has --- inline"))
				Expect(content).NotTo(ContainSubstring("status: approved"))
			})
		})

		Context("with --- at EOF without newline", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved
---`
				path = filepath.Join(tempDir, "002-eof.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("correctly parses frontmatter", func() {
				fm, err := prompt.ReadFrontmatter(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))
			})

			It("returns empty content error", func() {
				_, err := prompt.Content(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(Equal(prompt.ErrEmptyPrompt))
			})
		})

		Context("with in_review status in frontmatter", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: in_review
pr-url: https://github.com/example/repo/pull/42
---
# Some Prompt
Content here.
`
				path = filepath.Join(tempDir, "004-in-review.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("parses in_review status from frontmatter", func() {
				fm, err := prompt.ReadFrontmatter(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal(string(prompt.InReviewPromptStatus)))
			})
		})

		Context("with only opening --- and no closing", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved

# This is not valid frontmatter
Content here.
`
				path = filepath.Join(tempDir, "003-unclosed.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("treats entire file as content (no frontmatter)", func() {
				fm, err := prompt.ReadFrontmatter(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("")) // No frontmatter parsed

				content, err := prompt.Content(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("---"))
				Expect(content).To(ContainSubstring("status: approved"))
			})
		})
	})

	Describe("setField does not accumulate blank lines", func() {
		It("preserves body without adding extra newlines across multiple calls", func() {
			path := filepath.Join(tempDir, "001-no-growth.md")
			original := "# My Prompt\n\nDo the thing.\n"
			err := os.WriteFile(path, []byte(original), 0600)
			Expect(err).To(BeNil())

			// Simulate full lifecycle: created, queued, container, version, executing, completed
			err = prompt.SetStatus(ctx, path, "approved", libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			err = prompt.SetContainer(ctx, path, "test-container", libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			err = prompt.SetVersion(ctx, path, "v1.0.0", libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			err = prompt.SetStatus(ctx, path, "executing", libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			err = prompt.SetStatus(ctx, path, "completed", libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())

			// Read content — should have no leading blank lines
			content, err := prompt.Content(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(content).To(HavePrefix("# My Prompt"))
		})

		It("body size stays constant across 20 setField cycles", func() {
			path := filepath.Join(tempDir, "001-stable.md")
			original := "---\nstatus: approved\n---\n# Prompt\n\nContent here.\n"
			err := os.WriteFile(path, []byte(original), 0600)
			Expect(err).To(BeNil())

			// Read initial file size
			initialData, err := os.ReadFile(path)
			Expect(err).To(BeNil())
			initialSize := len(initialData)

			// Run 20 setField cycles (simulates retries)
			for i := 0; i < 20; i++ {
				err = prompt.SetStatus(ctx, path, "executing", libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				err = prompt.SetStatus(ctx, path, "approved", libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
			}

			// File should not have grown significantly (only timestamp changes)
			finalData, err := os.ReadFile(path)
			Expect(err).To(BeNil())

			// Allow for timestamp field additions but not blank line growth
			// Timestamps add ~80 bytes max, blank line growth would add 20+ bytes per cycle
			Expect(len(finalData)).To(BeNumerically("<", initialSize+200))

			// Content should still start correctly
			content, err := prompt.Content(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(content).To(HavePrefix("# Prompt"))
		})
	})

	Describe("StripNumberPrefix", func() {
		DescribeTable("strips numeric prefixes",
			func(input string, expected string) {
				Expect(prompt.StripNumberPrefix(input)).To(Equal(expected))
			},
			Entry("3-digit prefix", "200-foo.md", "foo.md"),
			Entry("1-digit prefix", "1-bar.md", "bar.md"),
			Entry("no prefix", "foo.md", "foo.md"),
			Entry("valid 3-digit prefix", "001-baz.md", "baz.md"),
			Entry("large number", "9999-test.md", "test.md"),
		)
	})

	Describe("NormalizeFilenames", func() {
		Context("with file missing numeric prefix", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-first.md", "approved")
				createPromptFile(tempDir, "002-second.md", "approved")
				createPromptFile(tempDir, "fix-something.md", "approved")
			})

			It("assigns next available number", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("fix-something.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("003-fix-something.md"))

				// Verify file was actually renamed
				_, err = os.Stat(filepath.Join(tempDir, "003-fix-something.md"))
				Expect(err).To(BeNil())
				_, err = os.Stat(filepath.Join(tempDir, "fix-something.md"))
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("with duplicate number", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "009-foo.md", "approved")
				createPromptFile(tempDir, "009-bar.md", "approved")
			})

			It("renames later file to next available number", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				// First file alphabetically (009-bar.md) is kept, second (009-foo.md) is renamed
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("009-foo.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("001-foo.md"))

				// Verify files exist
				_, err = os.Stat(filepath.Join(tempDir, "009-bar.md"))
				Expect(err).To(BeNil())
				_, err = os.Stat(filepath.Join(tempDir, "001-foo.md"))
				Expect(err).To(BeNil())
				_, err = os.Stat(filepath.Join(tempDir, "009-foo.md"))
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("with wrong format (single digit)", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "9-foo.md", "approved")
			})

			It("normalizes to zero-padded 3-digit format", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("9-foo.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("009-foo.md"))

				// Verify file was renamed
				_, err = os.Stat(filepath.Join(tempDir, "009-foo.md"))
				Expect(err).To(BeNil())
				_, err = os.Stat(filepath.Join(tempDir, "9-foo.md"))
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("with wrong format (two digits)", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "42-answer.md", "approved")
			})

			It("normalizes to zero-padded 3-digit format", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("42-answer.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("042-answer.md"))
			})
		})

		Context("with already-valid files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-first.md", "approved")
				createPromptFile(tempDir, "002-second.md", "approved")
				createPromptFile(tempDir, "003-third.md", "approved")
			})

			It("does not rename any files", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(0))

				// Verify files still exist with same names
				_, err = os.Stat(filepath.Join(tempDir, "001-first.md"))
				Expect(err).To(BeNil())
				_, err = os.Stat(filepath.Join(tempDir, "002-second.md"))
				Expect(err).To(BeNil())
				_, err = os.Stat(filepath.Join(tempDir, "003-third.md"))
				Expect(err).To(BeNil())
			})
		})

		Context("with mixed valid and invalid files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-valid.md", "approved")
				createPromptFile(tempDir, "9-wrong-format.md", "approved")
				createPromptFile(tempDir, "no-number.md", "approved")
			})

			It("renames only invalid files", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(2))

				// Verify valid file is unchanged
				_, err = os.Stat(filepath.Join(tempDir, "001-valid.md"))
				Expect(err).To(BeNil())

				// Verify invalid files were renamed
				_, err = os.Stat(filepath.Join(tempDir, "009-wrong-format.md"))
				Expect(err).To(BeNil())
				_, err = os.Stat(filepath.Join(tempDir, "002-no-number.md"))
				Expect(err).To(BeNil())
			})
		})

		Context("with completed subdirectory", func() {
			BeforeEach(func() {
				// Create completed subdirectory
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())

				// Add files to both root and completed
				createPromptFile(tempDir, "001-valid.md", "approved")
				createPromptFile(completedDir, "wrong-name.md", "completed")
			})

			It("does not rename files in subdirectories", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(0))

				// Verify completed file is unchanged
				_, err = os.Stat(filepath.Join(tempDir, "completed", "wrong-name.md"))
				Expect(err).To(BeNil())
			})
		})

		Context("with numbers used in completed/ subdirectory", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())

				// completed/ has 001-003
				createPromptFile(completedDir, "001-done.md", "completed")
				createPromptFile(completedDir, "002-done.md", "completed")
				createPromptFile(completedDir, "003-done.md", "completed")

				// root has no numbered files, only an unnumbered one
				createPromptFile(tempDir, "new-feature.md", "approved")
			})

			It("assigns next number above completed/ maximum", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("new-feature.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("004-new-feature.md"))
			})
		})

		Context("with completed/ files lacking frontmatter", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())

				// completed/ has files without frontmatter
				path1 := filepath.Join(completedDir, "001-no-fm.md")
				err = os.WriteFile(path1, []byte("# Old prompt\n"), 0600)
				Expect(err).To(BeNil())
				// And one with frontmatter
				createPromptFile(completedDir, "002-with-fm.md", "completed")
				// And one with empty frontmatter
				path3 := filepath.Join(completedDir, "003-empty-fm.md")
				err = os.WriteFile(path3, []byte("---\n---\n# Empty\n"), 0600)
				Expect(err).To(BeNil())

				// root has unnumbered file
				createPromptFile(tempDir, "new-task.md", "approved")
			})

			It("scans completed/ without errors and avoids used numbers", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				// Should assign 004 (not 001-003 which are used in completed/)
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("new-task.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("004-new-task.md"))
			})
		})

		Context("with non-markdown files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-valid.md", "approved")
				// Create non-markdown file without number
				err := os.WriteFile(filepath.Join(tempDir, "readme.txt"), []byte("test"), 0600)
				Expect(err).To(BeNil())
			})

			It("ignores non-markdown files", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(0))

				// Verify non-markdown file is unchanged
				_, err = os.Stat(filepath.Join(tempDir, "readme.txt"))
				Expect(err).To(BeNil())
			})
		})

		Context("with empty directory", func() {
			It("returns no renames", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(0))
			})
		})

		Context("with gaps in numbering", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-first.md", "approved")
				createPromptFile(tempDir, "005-fifth.md", "approved")
				createPromptFile(tempDir, "010-tenth.md", "approved")
				createPromptFile(tempDir, "new-file.md", "approved")
			})

			It("assigns smallest available number", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("002-new-file.md"))
			})
		})

		Context("with wrong-format file conflicting with completed/ number", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())

				createPromptFile(completedDir, "001-core-pipeline.md", "completed")
				createPromptFile(tempDir, "01-foo.md", "approved")
			})

			It("renames to next available number instead of conflicting 001", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("01-foo.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("002-foo.md"))
			})
		})

		Context("with wrong-format file conflicting with many completed/ numbers", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())

				for i := 1; i <= 94; i++ {
					name := fmt.Sprintf("%03d-done.md", i)
					createPromptFile(completedDir, name, "completed")
				}
				createPromptFile(tempDir, "01-foo.md", "approved")
			})

			It("renames to first number above completed/ maximum", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("01-foo.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("095-foo.md"))
			})
		})

		Context("with wrong-format file and no completed files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "01-foo.md", "approved")
			})

			It("reformats to 3-digit prefix keeping same number", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("01-foo.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("001-foo.md"))
			})
		})

		Context("with already-correct 3-digit file in queue", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "095-foo.md", "approved")
			})

			It("does not rename the file", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(0))
			})
		})

		Context("inbox files are ignored for numbering", func() {
			It("does not reserve numbers from inbox files", func() {
				inboxDir := filepath.Join(tempDir, "inbox")
				err := os.MkdirAll(inboxDir, 0750)
				Expect(err).To(BeNil())

				// inbox has 001-003 — these should be ignored
				createPromptFile(inboxDir, "001-draft.md", "approved")
				createPromptFile(inboxDir, "002-draft.md", "approved")
				createPromptFile(inboxDir, "003-draft.md", "approved")

				// in-progress has one unnumbered file
				createPromptFile(tempDir, "new-feature.md", "approved")

				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(
					ctx,
					tempDir,
					completedDir,
					mover,
				)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("new-feature.md"))
				// Should assign 001 — inbox numbers are not reserved
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("001-new-feature.md"))
			})
		})
	})

	Describe("PromptFile.SetPRURL", func() {
		It("sets the pr-url field in frontmatter", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: completed\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())

			pf.SetPRURL("https://github.com/user/repo/pull/42")
			Expect(pf.Frontmatter.PRURL).To(Equal("https://github.com/user/repo/pull/42"))
		})

		It("preserves pr-url after save and load roundtrip", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: completed\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			// Load, set PR URL, and save
			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			pf.SetPRURL("https://github.com/user/repo/pull/99")
			err = pf.Save(ctx)
			Expect(err).To(BeNil())

			// Load again and verify
			pf2, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf2.Frontmatter.PRURL).To(Equal("https://github.com/user/repo/pull/99"))
		})

		It("loads files without pr-url field", func() {
			path := filepath.Join(tempDir, "001-old-prompt.md")
			content := "---\nstatus: completed\nsummary: Old prompt\n---\n\n# Test\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.Frontmatter.PRURL).To(Equal(""))
		})
	})

	Describe("SetPRURL", func() {
		It("sets pr-url field in frontmatter", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: completed\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			err = prompt.SetPRURL(
				ctx,
				path,
				"https://github.com/user/repo/pull/123",
				libtime.NewCurrentDateTime(),
			)
			Expect(err).To(BeNil())

			// Verify the file was updated
			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.Frontmatter.PRURL).To(Equal("https://github.com/user/repo/pull/123"))
		})

		It("adds frontmatter if file has none", func() {
			path := filepath.Join(tempDir, "001-no-frontmatter.md")
			content := "# Test Prompt\n\nContent here.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			err = prompt.SetPRURL(
				ctx,
				path,
				"https://github.com/user/repo/pull/1",
				libtime.NewCurrentDateTime(),
			)
			Expect(err).To(BeNil())

			// Verify frontmatter was added with pr-url
			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.Frontmatter.PRURL).To(Equal("https://github.com/user/repo/pull/1"))
		})
	})

	Describe("PromptFile.PRURL", func() {
		It("returns pr-url value when set in frontmatter", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: completed\npr-url: https://github.com/user/repo/pull/42\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.PRURL()).To(Equal("https://github.com/user/repo/pull/42"))
		})

		It("returns empty string when pr-url is not set", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: completed\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.PRURL()).To(Equal(""))
		})
	})

	Describe("PromptFile.MarkFailed", func() {
		It("sets status to failed with timestamp", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: executing\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())

			pf.MarkFailed()
			Expect(pf.Frontmatter.Status).To(Equal("failed"))
			Expect(pf.Frontmatter.Completed).NotTo(BeEmpty())
		})
	})

	Describe("PromptFile.SetBranch", func() {
		It("sets the branch field in frontmatter", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())

			pf.SetBranch("dark-factory/042-add-feature")
			Expect(pf.Frontmatter.Branch).To(Equal("dark-factory/042-add-feature"))
		})
	})

	Describe("PromptFile.Branch", func() {
		It("returns branch value when branch field is set in frontmatter", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\nbranch: dark-factory/042-add-feature\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.Branch()).To(Equal("dark-factory/042-add-feature"))
		})

		It("returns empty string when branch field is not set", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.Branch()).To(Equal(""))
		})
	})

	Describe("SetBranch", func() {
		It("writes the branch value into frontmatter", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			err = prompt.SetBranch(
				ctx,
				path,
				"dark-factory/042-add-feature",
				libtime.NewCurrentDateTime(),
			)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.Branch()).To(Equal("dark-factory/042-add-feature"))
		})

		It("adds frontmatter if file has none", func() {
			path := filepath.Join(tempDir, "001-no-frontmatter.md")
			content := "# Test Prompt\n\nContent here.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			err = prompt.SetBranch(
				ctx,
				path,
				"dark-factory/042-add-feature",
				libtime.NewCurrentDateTime(),
			)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.Branch()).To(Equal("dark-factory/042-add-feature"))
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

var _ = Describe("Frontmatter spec field", func() {
	var (
		ctx     context.Context
		tempDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "prompt-spec-test-*")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	It("loads spec field from frontmatter (scalar string)", func() {
		path := filepath.Join(tempDir, "091-test.md")
		content := "---\nstatus: approved\nspec: \"017\"\n---\n\n# Test\n"
		Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())

		pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
		Expect(err).To(BeNil())
		Expect(pf.Specs()).To(Equal([]string{"017"}))
		Expect(pf.Frontmatter.HasSpec("017")).To(BeTrue())
		Expect(pf.Frontmatter.Status).To(Equal("approved"))
	})

	It("loads spec field from frontmatter (array)", func() {
		path := filepath.Join(tempDir, "091-array.md")
		content := "---\nstatus: approved\nspec: [\"017\", \"019\"]\n---\n\n# Test\n"
		Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())

		pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
		Expect(err).To(BeNil())
		Expect(pf.Specs()).To(Equal([]string{"017", "019"}))
		Expect(pf.Frontmatter.HasSpec("017")).To(BeTrue())
		Expect(pf.Frontmatter.HasSpec("019")).To(BeTrue())
		Expect(pf.Frontmatter.HasSpec("018")).To(BeFalse())
	})

	It("saves and reloads spec field correctly", func() {
		path := filepath.Join(tempDir, "091-test.md")
		content := "---\nstatus: approved\nspec: \"019\"\n---\n\n# Test\n"
		Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())

		pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
		Expect(err).To(BeNil())
		Expect(pf.Frontmatter.HasSpec("019")).To(BeTrue())

		pf.Frontmatter.Status = "completed"
		Expect(pf.Save(ctx)).To(Succeed())

		pf2, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
		Expect(err).To(BeNil())
		Expect(pf2.Frontmatter.HasSpec("019")).To(BeTrue())
		Expect(pf2.Frontmatter.Status).To(Equal("completed"))
	})

	It("works without spec field (backward compatible)", func() {
		path := filepath.Join(tempDir, "001-no-spec.md")
		content := "---\nstatus: approved\n---\n\n# No spec\n"
		Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())

		pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
		Expect(err).To(BeNil())
		Expect(pf.Specs()).To(BeEmpty())
		Expect(pf.Frontmatter.Status).To(Equal("approved"))
	})

	It("omits spec field when empty on save", func() {
		path := filepath.Join(tempDir, "001-no-spec.md")
		content := "---\nstatus: approved\n---\n\n# No spec\n"
		Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())

		pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
		Expect(err).To(BeNil())
		Expect(pf.Save(ctx)).To(Succeed())

		saved, err := os.ReadFile(path)
		Expect(err).To(BeNil())
		Expect(string(saved)).NotTo(ContainSubstring("spec:"))
	})

	Describe("HasSpec integer-prefix matching", func() {
		It("matches full spec name against padded number stored in frontmatter", func() {
			// Simulates spec_list.go passing sf.Name ("019-review-fix-loop")
			// while the prompt stores spec: ["019"]
			path := filepath.Join(tempDir, "100-test.md")
			content := "---\nstatus: approved\nspec: \"019\"\n---\n\n# Test\n"
			Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.Frontmatter.HasSpec("019-review-fix-loop")).To(BeTrue())
			Expect(pf.Frontmatter.HasSpec("019")).To(BeTrue())
			Expect(pf.Frontmatter.HasSpec("19")).To(BeTrue())
			Expect(pf.Frontmatter.HasSpec("0019")).To(BeTrue())
			Expect(pf.Frontmatter.HasSpec("020-other")).To(BeFalse())
		})

		It("non-numeric spec IDs still match by exact string", func() {
			path := filepath.Join(tempDir, "100-test.md")
			content := "---\nstatus: approved\nspec: \"notifications\"\n---\n\n# Test\n"
			Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.Frontmatter.HasSpec("notifications")).To(BeTrue())
			Expect(pf.Frontmatter.HasSpec("other")).To(BeFalse())
		})
	})

	Describe("PromptFile.RetryCount", func() {
		It("returns retryCount value when set in frontmatter", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\nretryCount: 2\n---\n\n# Test\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.RetryCount()).To(Equal(2))
		})

		It("returns 0 when retryCount is not set", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\n---\n\n# Test\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.RetryCount()).To(Equal(0))
		})
	})

	Describe("IncrementRetryCount", func() {
		It("increments retryCount from 0 to 1", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\n---\n\n# Test\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			err = prompt.IncrementRetryCount(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.RetryCount()).To(Equal(1))
		})

		It("increments retryCount from 2 to 3", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\nretryCount: 2\n---\n\n# Test\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			err = prompt.IncrementRetryCount(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.RetryCount()).To(Equal(3))
		})
	})

	Describe("PendingVerificationPromptStatus", func() {
		It("is in AvailablePromptStatuses", func() {
			Expect(
				prompt.AvailablePromptStatuses,
			).To(ContainElement(prompt.PendingVerificationPromptStatus))
		})

		It("is recognized by Contains", func() {
			Expect(
				prompt.AvailablePromptStatuses.Contains(prompt.PendingVerificationPromptStatus),
			).To(BeTrue())
		})
	})

	Describe("MarkPendingVerification", func() {
		It("sets Frontmatter.Status to pending_verification", func() {
			pf := &prompt.PromptFile{}
			pf.MarkPendingVerification()
			Expect(pf.Frontmatter.Status).To(Equal("pending_verification"))
		})
	})

	Describe("VerificationSection", func() {
		It("returns trimmed content between verification tags", func() {
			pf := &prompt.PromptFile{
				Body: []byte(
					"Some text\n<verification>\n  Run make test\n</verification>\nMore text",
				),
			}
			Expect(pf.VerificationSection()).To(Equal("Run make test"))
		})

		It("returns empty string when no verification tag is present", func() {
			pf := &prompt.PromptFile{
				Body: []byte("Some text without verification tags"),
			}
			Expect(pf.VerificationSection()).To(Equal(""))
		})

		It("returns empty string when only opening tag is present", func() {
			pf := &prompt.PromptFile{
				Body: []byte("Some text\n<verification>\nRun make test"),
			}
			Expect(pf.VerificationSection()).To(Equal(""))
		})
	})

	Describe("ListQueued skips pending_verification", func() {
		It("does not return a file with status pending_verification", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: pending_verification\n---\n\n# Test\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			prompts, err := prompt.ListQueued(ctx, tempDir, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(prompts).To(BeEmpty())
		})
	})

	Describe("Issue field", func() {
		var cdt libtime.CurrentDateTimeGetter

		BeforeEach(func() {
			cdt = libtime.NewCurrentDateTime()
		})

		It("loads prompt with issue field preserved", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\nissue: BRO-19476\n---\n\n# Test\n"
			Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())

			pf, err := prompt.Load(ctx, path, cdt)
			Expect(err).NotTo(HaveOccurred())
			Expect(pf.Frontmatter.Issue).To(Equal("BRO-19476"))
		})

		It("saves prompt with issue field in output YAML", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\n---\n\n# Test\n"
			Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())

			pf, err := prompt.Load(ctx, path, cdt)
			Expect(err).NotTo(HaveOccurred())
			pf.SetIssue("BRO-99")
			Expect(pf.Save(ctx)).To(Succeed())

			saved, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(saved)).To(ContainSubstring("issue: BRO-99"))
		})

		It("SetIssueIfEmpty does not overwrite an existing value", func() {
			pf := prompt.NewPromptFile(
				filepath.Join(tempDir, "001-test.md"),
				prompt.Frontmatter{Status: "approved", Issue: "original"},
				[]byte("# Test\n"),
				cdt,
			)
			pf.SetIssueIfEmpty("new-value")
			Expect(pf.Frontmatter.Issue).To(Equal("original"))
		})

		It("SetIssueIfEmpty sets value when field is empty", func() {
			pf := prompt.NewPromptFile(
				filepath.Join(tempDir, "001-test.md"),
				prompt.Frontmatter{Status: "approved"},
				[]byte("# Test\n"),
				cdt,
			)
			pf.SetIssueIfEmpty("BRO-42")
			Expect(pf.Frontmatter.Issue).To(Equal("BRO-42"))
		})

		It("SetBranchIfEmpty does not overwrite an existing value", func() {
			pf := prompt.NewPromptFile(
				filepath.Join(tempDir, "001-test.md"),
				prompt.Frontmatter{Status: "approved", Branch: "my-branch"},
				[]byte("# Test\n"),
				cdt,
			)
			pf.SetBranchIfEmpty("other-branch")
			Expect(pf.Frontmatter.Branch).To(Equal("my-branch"))
		})

		It("SetBranchIfEmpty sets value when field is empty", func() {
			pf := prompt.NewPromptFile(
				filepath.Join(tempDir, "001-test.md"),
				prompt.Frontmatter{Status: "approved"},
				[]byte("# Test\n"),
				cdt,
			)
			pf.SetBranchIfEmpty("dark-factory/spec-028")
			Expect(pf.Frontmatter.Branch).To(Equal("dark-factory/spec-028"))
		})

		It("existing prompt without issue loads and saves without error", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\n---\n\n# Test\n"
			Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())

			pf, err := prompt.Load(ctx, path, cdt)
			Expect(err).NotTo(HaveOccurred())
			Expect(pf.Frontmatter.Issue).To(Equal(""))
			Expect(pf.Save(ctx)).To(Succeed())
		})

		It("Issue() returns frontmatter issue value", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\nissue: BRO-42\n---\n\n# Test\n"
			Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())

			pf, err := prompt.Load(ctx, path, cdt)
			Expect(err).NotTo(HaveOccurred())
			Expect(pf.Issue()).To(Equal("BRO-42"))
		})

		It("Issue() returns empty string when issue field is not set", func() {
			pf := prompt.NewPromptFile(
				filepath.Join(tempDir, "001-test.md"),
				prompt.Frontmatter{Status: "approved"},
				[]byte("# Test\n"),
				cdt,
			)
			Expect(pf.Issue()).To(Equal(""))
		})
	})

	Describe("Manager.HasQueuedPromptsOnBranch", func() {
		var (
			inProgressDir string
			completedDir  string
			mgr           *prompt.Manager
		)

		BeforeEach(func() {
			inProgressDir = filepath.Join(tempDir, "in-progress")
			completedDir = filepath.Join(tempDir, "completed")
			Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())
			Expect(os.MkdirAll(completedDir, 0750)).To(Succeed())
			localMover := &simpleMover{}
			mgr = prompt.NewManager(
				filepath.Join(tempDir, "inbox"),
				inProgressDir,
				completedDir,
				localMover,
				libtime.NewCurrentDateTime(),
			)
		})

		writeQueuedPromptWithBranch := func(filename, branch string) string {
			path := filepath.Join(inProgressDir, filename)
			content := fmt.Sprintf("---\nstatus: approved\nbranch: %s\n---\n\n# Test\n", branch)
			Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
			return path
		}

		It("returns true when another queued prompt shares the same branch", func() {
			path1 := writeQueuedPromptWithBranch("001-a.md", "feature/shared")
			_ = writeQueuedPromptWithBranch("002-b.md", "feature/shared")

			has, err := mgr.HasQueuedPromptsOnBranch(ctx, "feature/shared", path1)
			Expect(err).NotTo(HaveOccurred())
			Expect(has).To(BeTrue())
		})

		It("returns false when the only matching prompt is excluded", func() {
			path1 := writeQueuedPromptWithBranch("001-only.md", "feature/solo")

			has, err := mgr.HasQueuedPromptsOnBranch(ctx, "feature/solo", path1)
			Expect(err).NotTo(HaveOccurred())
			Expect(has).To(BeFalse())
		})

		It("returns false when prompts have different branches", func() {
			path1 := writeQueuedPromptWithBranch("001-x.md", "feature/branch-a")
			_ = writeQueuedPromptWithBranch("002-y.md", "feature/branch-b")

			has, err := mgr.HasQueuedPromptsOnBranch(ctx, "feature/branch-a", path1)
			Expect(err).NotTo(HaveOccurred())
			Expect(has).To(BeFalse())
		})

		It("returns false when queue is empty", func() {
			has, err := mgr.HasQueuedPromptsOnBranch(ctx, "feature/anything", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(has).To(BeFalse())
		})
	})

	Describe("PromptFile.MarkCompleted clears LastFailReason", func() {
		It(
			"success after failure clears the field (reproducer: 003-test-build-info-metrics)",
			func() {
				path := filepath.Join(tempDir, "001-test.md")
				content := "---\nstatus: failed\nlastFailReason: 'execute prompt: docker run failed: wait command: exit status 128'\n---\n\n# Test\n\nContent.\n"
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())

				pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(pf.Frontmatter.LastFailReason).NotTo(BeEmpty())

				pf.MarkCompleted()
				err = pf.Save(ctx)
				Expect(err).To(BeNil())

				pf2, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).To(BeNil())
				Expect(pf2.Frontmatter.Status).To(Equal("completed"))
				Expect(pf2.Frontmatter.LastFailReason).To(BeEmpty())

				raw, err := os.ReadFile(path)
				Expect(err).To(BeNil())
				Expect(string(raw)).NotTo(ContainSubstring("lastFailReason"))
			},
		)

		It("pristine success leaves frontmatter clean", func() {
			path := filepath.Join(tempDir, "002-test.md")
			content := "---\nstatus: approved\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.Frontmatter.LastFailReason).To(BeEmpty())

			pf.MarkCompleted()
			err = pf.Save(ctx)
			Expect(err).To(BeNil())

			pf2, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf2.Frontmatter.Status).To(Equal("completed"))
			Expect(pf2.Frontmatter.LastFailReason).To(BeEmpty())

			raw, err := os.ReadFile(path)
			Expect(err).To(BeNil())
			Expect(string(raw)).NotTo(ContainSubstring("lastFailReason"))
		})

		It("second failure replaces the old reason (failure path is untouched)", func() {
			path := filepath.Join(tempDir, "003-test.md")
			content := "---\nstatus: approved\nlastFailReason: 'first reason'\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())

			pf.SetLastFailReason("second reason")
			pf.MarkFailed()
			err = pf.Save(ctx)
			Expect(err).To(BeNil())

			pf2, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf2.Frontmatter.Status).To(Equal("failed"))
			Expect(pf2.Frontmatter.LastFailReason).To(Equal("second reason"))

			raw, err := os.ReadFile(path)
			Expect(err).To(BeNil())
			Expect(string(raw)).To(ContainSubstring("second reason"))
			Expect(string(raw)).NotTo(ContainSubstring("first reason"))
		})

		It("in-memory clear without Save clears the field immediately", func() {
			pf := prompt.NewPromptFile(
				"/tmp/unused.md",
				prompt.Frontmatter{
					Status:         "failed",
					LastFailReason: "stale",
				},
				[]byte("# Test\n"),
				libtime.NewCurrentDateTime(),
			)
			Expect(pf.Frontmatter.LastFailReason).To(Equal("stale"))

			pf.MarkCompleted()
			Expect(pf.Frontmatter.LastFailReason).To(BeEmpty())
		})
	})

	Describe("PromptFile.SetLastFailReason", func() {
		It("sets the LastFailReason field", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: failed\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())

			pf.SetLastFailReason("msg")
			Expect(pf.Frontmatter.LastFailReason).To(Equal("msg"))
		})
	})

	Describe("Frontmatter without lastFailReason", func() {
		It("parses correctly with zero value when field is absent", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: failed\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(pf.Frontmatter.LastFailReason).To(BeEmpty())
		})
	})
})
