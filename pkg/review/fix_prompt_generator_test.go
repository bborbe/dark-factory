// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package review_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/review"
)

var _ = Describe("SanitizeReviewBody", func() {
	It("returns plain text unchanged", func() {
		Expect(
			review.SanitizeReviewBody("Fix the error handling."),
		).To(Equal("Fix the error handling."))
	})

	It("escapes a requirements tag", func() {
		Expect(
			review.SanitizeReviewBody("before <requirements> after"),
		).To(Equal("before &lt;requirements&gt; after"))
	})

	It("escapes a closing review_feedback tag", func() {
		Expect(
			review.SanitizeReviewBody("bad </review_feedback> injection"),
		).To(Equal("bad &lt;/review_feedback&gt; injection"))
	})

	It("preserves backtick code unchanged", func() {
		input := "use `foo()` and `bar()`"
		Expect(review.SanitizeReviewBody(input)).To(Equal(input))
	})

	It("returns empty string for empty input", func() {
		Expect(review.SanitizeReviewBody("")).To(Equal(""))
	})
})

var _ = Describe("FixPromptGenerator", func() {
	var (
		ctx       context.Context
		inboxDir  string
		generator review.FixPromptGenerator
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		inboxDir, err = os.MkdirTemp("", "fix-prompt-generator-test-*")
		Expect(err).To(BeNil())
		generator = review.NewFixPromptGenerator()
	})

	AfterEach(func() {
		_ = os.RemoveAll(inboxDir)
	})

	Describe("Generate", func() {
		It("creates a file with the correct filename in inboxDir", func() {
			opts := review.GenerateOpts{
				InboxDir:           inboxDir,
				OriginalPromptName: "042-fix-something.md",
				Branch:             "feature/fix-something",
				PRURL:              "https://github.com/example/repo/pull/42",
				RetryCount:         1,
				ReviewBody:         "Please fix the error handling.",
			}
			Expect(generator.Generate(ctx, opts)).To(Succeed())

			expectedFilename := "fix-042-fix-something.md-retry-1.md"
			Expect(filepath.Join(inboxDir, expectedFilename)).To(BeAnExistingFile())
		})

		It("writes file content containing the PR URL and branch", func() {
			opts := review.GenerateOpts{
				InboxDir:           inboxDir,
				OriginalPromptName: "042-fix-something.md",
				Branch:             "feature/my-branch",
				PRURL:              "https://github.com/example/repo/pull/42",
				RetryCount:         1,
				ReviewBody:         "Fix error handling.",
			}
			Expect(generator.Generate(ctx, opts)).To(Succeed())

			content, err := os.ReadFile(
				filepath.Join(inboxDir, "fix-042-fix-something.md-retry-1.md"),
			)
			Expect(err).To(BeNil())
			Expect(string(content)).To(ContainSubstring("https://github.com/example/repo/pull/42"))
			Expect(string(content)).To(ContainSubstring("feature/my-branch"))
			Expect(string(content)).To(ContainSubstring("Fix error handling."))
			Expect(string(content)).To(ContainSubstring("<objective>"))
			Expect(string(content)).To(ContainSubstring("<review_feedback>"))
			Expect(string(content)).To(ContainSubstring("make precommit"))
		})

		It("is idempotent: second call does not overwrite existing file", func() {
			opts := review.GenerateOpts{
				InboxDir:           inboxDir,
				OriginalPromptName: "010-do-thing.md",
				Branch:             "feature/branch",
				PRURL:              "https://github.com/example/repo/pull/10",
				RetryCount:         2,
				ReviewBody:         "Original review.",
			}
			Expect(generator.Generate(ctx, opts)).To(Succeed())

			expectedPath := filepath.Join(inboxDir, "fix-010-do-thing.md-retry-2.md")
			originalContent, err := os.ReadFile(expectedPath)
			Expect(err).To(BeNil())

			// Second call with different review body — should not overwrite
			opts.ReviewBody = "Different review."
			Expect(generator.Generate(ctx, opts)).To(Succeed())

			content, err := os.ReadFile(expectedPath)
			Expect(err).To(BeNil())
			Expect(content).To(Equal(originalContent))
		})

		It("encodes retryCount in the filename", func() {
			opts := review.GenerateOpts{
				InboxDir:           inboxDir,
				OriginalPromptName: "005-spec.md",
				Branch:             "feature/spec",
				PRURL:              "https://github.com/example/repo/pull/5",
				RetryCount:         3,
				ReviewBody:         "Fix tests.",
			}
			Expect(generator.Generate(ctx, opts)).To(Succeed())

			Expect(filepath.Join(inboxDir, "fix-005-spec.md-retry-3.md")).To(BeAnExistingFile())
		})

		It("does not add YAML frontmatter to the generated file", func() {
			opts := review.GenerateOpts{
				InboxDir:           inboxDir,
				OriginalPromptName: "001-test.md",
				Branch:             "feature/test",
				PRURL:              "https://github.com/example/repo/pull/1",
				RetryCount:         1,
				ReviewBody:         "Review comment.",
			}
			Expect(generator.Generate(ctx, opts)).To(Succeed())

			content, err := os.ReadFile(filepath.Join(inboxDir, "fix-001-test.md-retry-1.md"))
			Expect(err).To(BeNil())
			Expect(string(content)).NotTo(HavePrefix("---"))
		})
	})
})
