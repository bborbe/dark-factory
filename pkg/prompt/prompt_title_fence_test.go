// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt_test

import (
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("Title with fenced code blocks", func() {
	Describe("AC1 — fence-only hash line yields empty title", func() {
		It("returns empty when the only # line is inside a fenced YAML block", func() {
			body := "<requirements>\n```yaml\n# yaml-comment-line\nkey: value\n```\n</requirements>\n"
			pf := prompt.NewPromptFile(
				"001-test.md",
				prompt.Frontmatter{},
				[]byte(body),
				libtime.NewCurrentDateTime(),
			)
			Expect(pf.Title()).To(Equal(""))
		})
	})

	Describe("AC2 — real heading after a fence wins", func() {
		It("returns the first # heading outside the fence", func() {
			body := "```yaml\n# inside fence\n```\n\n# Real Title\n\nbody\n"
			pf := prompt.NewPromptFile(
				"001-test.md",
				prompt.Frontmatter{},
				[]byte(body),
				libtime.NewCurrentDateTime(),
			)
			Expect(pf.Title()).To(Equal("Real Title"))
		})
	})

	Describe("AC3 — no fences, existing behavior preserved", func() {
		It("extracts the heading when no fences are present", func() {
			body := "# Hello\n\nbody"
			pf := prompt.NewPromptFile(
				"001-test.md",
				prompt.Frontmatter{},
				[]byte(body),
				libtime.NewCurrentDateTime(),
			)
			Expect(pf.Title()).To(Equal("Hello"))
		})
	})

	Describe("AC4 — unterminated fence yields empty title", func() {
		It("does not panic and returns empty when fence is never closed", func() {
			body := "```\n# heading inside unterminated fence\nstill inside\n"
			pf := prompt.NewPromptFile(
				"001-test.md",
				prompt.Frontmatter{},
				[]byte(body),
				libtime.NewCurrentDateTime(),
			)
			Expect(pf.Title()).To(Equal(""))
		})
	})

	Describe("AC5 — tilde fences recognised", func() {
		It("returns empty when the only # line is inside a tilde fence", func() {
			body := "~~~yaml\n# tilde-fenced comment\nkey: value\n~~~\n"
			pf := prompt.NewPromptFile(
				"001-test.md",
				prompt.Frontmatter{},
				[]byte(body),
				libtime.NewCurrentDateTime(),
			)
			Expect(pf.Title()).To(Equal(""))
		})
	})

	Describe("AC6 — production reproduction body", func() {
		It("returns empty for the exact reproduction body from the bug report", func() {
			body := "<summary>\n- Demonstrates the Title() bug\n</summary>\n\n" +
				"<requirements>\n" +
				"1. Add the following block to `k8s/some-deploy.yaml`:\n" +
				"   ```yaml\n" +
				"   # SOME_FLAG toggles between modes\n" +
				"   - name: SOME_FLAG\n" +
				"     value: \"false\"\n" +
				"   ```\n" +
				"2. Run `git add k8s/some-deploy.yaml`.\n" +
				"3. Commit with: `git commit -m \"feat: add some flag\"`.\n" +
				"</requirements>\n"
			pf := prompt.NewPromptFile(
				"001-test.md",
				prompt.Frontmatter{},
				[]byte(body),
				libtime.NewCurrentDateTime(),
			)
			Expect(pf.Title()).To(Equal(""))
		})
	})

	Describe("regression — indented heading outside fence is extracted", func() {
		It("still extracts a leading-whitespace heading outside any fence", func() {
			body := "  # Indented Heading\n\nbody\n"
			pf := prompt.NewPromptFile(
				"001-test.md",
				prompt.Frontmatter{},
				[]byte(body),
				libtime.NewCurrentDateTime(),
			)
			Expect(pf.Title()).To(Equal("Indented Heading"))
		})
	})
})
