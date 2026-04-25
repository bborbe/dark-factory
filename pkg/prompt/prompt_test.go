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

})

var _ = Describe("rejected status", func() {
	var (
		ctx     context.Context
		tempDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "prompt-rejected-*")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	It("RejectedPromptStatus is in AvailablePromptStatuses", func() {
		Expect(prompt.AvailablePromptStatuses.Contains(prompt.RejectedPromptStatus)).To(BeTrue())
	})

	Describe("IsRejectable returns true from pre-execution states only", func() {
		It("returns true for idea", func() {
			Expect(prompt.IdeaPromptStatus.IsRejectable()).To(BeTrue())
		})
		It("returns true for draft", func() {
			Expect(prompt.DraftPromptStatus.IsRejectable()).To(BeTrue())
		})
		It("returns true for approved", func() {
			Expect(prompt.ApprovedPromptStatus.IsRejectable()).To(BeTrue())
		})
		It("returns false for executing", func() {
			Expect(prompt.ExecutingPromptStatus.IsRejectable()).To(BeFalse())
		})
		It("returns false for failed", func() {
			Expect(prompt.FailedPromptStatus.IsRejectable()).To(BeFalse())
		})
		It("returns false for completed", func() {
			Expect(prompt.CompletedPromptStatus.IsRejectable()).To(BeFalse())
		})
		It("returns false for cancelled", func() {
			Expect(prompt.CancelledPromptStatus.IsRejectable()).To(BeFalse())
		})
		It("returns false for rejected", func() {
			Expect(prompt.RejectedPromptStatus.IsRejectable()).To(BeFalse())
		})
	})

	Describe("valid reject transitions succeed", func() {
		It("idea → rejected", func() {
			Expect(
				prompt.IdeaPromptStatus.CanTransitionTo(prompt.RejectedPromptStatus),
			).To(Succeed())
		})
		It("draft → rejected", func() {
			Expect(
				prompt.DraftPromptStatus.CanTransitionTo(prompt.RejectedPromptStatus),
			).To(Succeed())
		})
		It("approved → rejected", func() {
			Expect(
				prompt.ApprovedPromptStatus.CanTransitionTo(prompt.RejectedPromptStatus),
			).To(Succeed())
		})
	})

	Describe("no outgoing edges from rejected; non-pre-execution cannot be rejected", func() {
		It("rejected cannot transition to draft", func() {
			Expect(
				prompt.RejectedPromptStatus.CanTransitionTo(prompt.DraftPromptStatus),
			).To(HaveOccurred())
		})
		It("executing cannot transition to rejected", func() {
			Expect(
				prompt.ExecutingPromptStatus.CanTransitionTo(prompt.RejectedPromptStatus),
			).To(HaveOccurred())
		})
		It("completed cannot transition to rejected", func() {
			Expect(
				prompt.CompletedPromptStatus.CanTransitionTo(prompt.RejectedPromptStatus),
			).To(HaveOccurred())
		})
	})

	Describe("StampRejected sets all three fields", func() {
		It("sets status, reason, and timestamp", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())

			pf.StampRejected("abandoned work")

			Expect(pf.Frontmatter.Status).To(Equal(string(prompt.RejectedPromptStatus)))
			Expect(pf.Frontmatter.RejectedReason).To(Equal("abandoned work"))
			Expect(pf.Frontmatter.Rejected).NotTo(BeEmpty())
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

	Describe("CommittingPromptStatus", func() {
		It("is in AvailablePromptStatuses", func() {
			Expect(
				prompt.AvailablePromptStatuses.Contains(prompt.CommittingPromptStatus),
			).To(BeTrue())
		})
	})

	Describe("MarkCommitting", func() {
		It("sets status to committing", func() {
			pf := prompt.NewPromptFile(
				filepath.Join(tempDir, "001-test.md"),
				prompt.Frontmatter{Status: "executing"},
				[]byte("# Test\n"),
				libtime.NewCurrentDateTime(),
			)
			pf.MarkCommitting()
			Expect(pf.Frontmatter.Status).To(Equal("committing"))
		})
	})

	Describe("ListQueued skips committing", func() {
		It("does not return a file with status committing", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: committing\n---\n\n# Test\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			prompts, err := prompt.ListQueued(ctx, tempDir, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(prompts).To(BeEmpty())
		})
	})

	Describe("FindCommitting", func() {
		It("returns only files with committing status", func() {
			committingPath := filepath.Join(tempDir, "001-committing.md")
			err := os.WriteFile(
				committingPath,
				[]byte("---\nstatus: committing\n---\n\n# Test\n"),
				0600,
			)
			Expect(err).To(BeNil())

			approvedPath := filepath.Join(tempDir, "002-approved.md")
			err = os.WriteFile(approvedPath, []byte("---\nstatus: approved\n---\n\n# Test\n"), 0600)
			Expect(err).To(BeNil())

			paths, err := prompt.FindCommitting(ctx, tempDir, libtime.NewCurrentDateTime())
			Expect(err).To(BeNil())
			Expect(paths).To(HaveLen(1))
			Expect(paths[0]).To(Equal(committingPath))
		})

		It("returns nil for a non-existent directory", func() {
			paths, err := prompt.FindCommitting(
				ctx,
				filepath.Join(tempDir, "nonexistent"),
				libtime.NewCurrentDateTime(),
			)
			Expect(err).To(BeNil())
			Expect(paths).To(BeNil())
		})
	})

	Describe("PromptStatus lifecycle model", func() {
		Describe("CanTransitionTo", func() {
			It("allows valid forward transitions", func() {
				Expect(
					prompt.IdeaPromptStatus.CanTransitionTo(prompt.DraftPromptStatus),
				).To(Succeed())
				Expect(
					prompt.DraftPromptStatus.CanTransitionTo(prompt.ApprovedPromptStatus),
				).To(Succeed())
				Expect(
					prompt.ApprovedPromptStatus.CanTransitionTo(prompt.ExecutingPromptStatus),
				).To(Succeed())
				Expect(
					prompt.FailedPromptStatus.CanTransitionTo(prompt.ApprovedPromptStatus),
				).To(Succeed())
				Expect(
					prompt.PendingVerificationPromptStatus.CanTransitionTo(
						prompt.CompletedPromptStatus,
					),
				).To(Succeed())
			})
			It("allows unapprove edge: approved → draft", func() {
				Expect(
					prompt.ApprovedPromptStatus.CanTransitionTo(prompt.DraftPromptStatus),
				).To(Succeed())
			})
			It("rejects invalid transition", func() {
				err := prompt.DraftPromptStatus.CanTransitionTo(prompt.CompletedPromptStatus)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("draft")))
				Expect(err).To(MatchError(ContainSubstring("completed")))
			})
		})

		Describe("IsTerminal", func() {
			It("returns true for terminal statuses", func() {
				Expect(prompt.CompletedPromptStatus.IsTerminal()).To(BeTrue())
				Expect(prompt.CancelledPromptStatus.IsTerminal()).To(BeTrue())
			})
			It("returns false for non-terminal statuses", func() {
				Expect(prompt.FailedPromptStatus.IsTerminal()).To(BeFalse())
				Expect(prompt.ApprovedPromptStatus.IsTerminal()).To(BeFalse())
			})
		})

		Describe("IsPreExecution", func() {
			It("returns true for pre-execution statuses", func() {
				Expect(prompt.IdeaPromptStatus.IsPreExecution()).To(BeTrue())
				Expect(prompt.DraftPromptStatus.IsPreExecution()).To(BeTrue())
				Expect(prompt.ApprovedPromptStatus.IsPreExecution()).To(BeTrue())
			})
			It("returns false for active statuses", func() {
				Expect(prompt.ExecutingPromptStatus.IsPreExecution()).To(BeFalse())
				Expect(prompt.FailedPromptStatus.IsPreExecution()).To(BeFalse())
			})
		})

		Describe("IsActive", func() {
			It("returns true for active statuses", func() {
				Expect(prompt.ExecutingPromptStatus.IsActive()).To(BeTrue())
				Expect(prompt.FailedPromptStatus.IsActive()).To(BeTrue())
				Expect(prompt.CommittingPromptStatus.IsActive()).To(BeTrue())
				Expect(prompt.InReviewPromptStatus.IsActive()).To(BeTrue())
				Expect(prompt.PendingVerificationPromptStatus.IsActive()).To(BeTrue())
			})
			It("returns false for pre-execution and terminal statuses", func() {
				Expect(prompt.ApprovedPromptStatus.IsActive()).To(BeFalse())
				Expect(prompt.CompletedPromptStatus.IsActive()).To(BeFalse())
				Expect(prompt.CancelledPromptStatus.IsActive()).To(BeFalse())
			})
		})

		Describe("Load permissiveness", func() {
			It("accepts legacy queued status without error", func() {
				path := filepath.Join(tempDir, "001-legacy.md")
				Expect(
					os.WriteFile(path, []byte("---\nstatus: queued\n---\n# Prompt\n"), 0600),
				).To(Succeed())
				pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).NotTo(HaveOccurred())
				Expect(pf.Frontmatter.Status).To(Equal("queued"))
			})
			It("accepts valid status string", func() {
				path := filepath.Join(tempDir, "002-valid.md")
				Expect(
					os.WriteFile(path, []byte("---\nstatus: approved\n---\n# Prompt\n"), 0600),
				).To(Succeed())
				pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).NotTo(HaveOccurred())
				Expect(pf.Frontmatter.Status).To(Equal("approved"))
			})
			It("accepts file with no frontmatter", func() {
				path := filepath.Join(tempDir, "003-no-fm.md")
				Expect(
					os.WriteFile(path, []byte("# No frontmatter\n\nJust content.\n"), 0600),
				).To(Succeed())
				pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
				Expect(err).NotTo(HaveOccurred())
				Expect(pf.Frontmatter.Status).To(Equal(""))
			})
		})
	})
})
