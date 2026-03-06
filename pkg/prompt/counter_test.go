// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

func writeLinkedPrompt(path, status, specID string) {
	fm := fmt.Sprintf("---\nstatus: %s\n", status)
	if specID != "" {
		fm += fmt.Sprintf("spec: %s\n", specID)
	}
	fm += "---\n# Test\n\nContent.\n"
	Expect(os.WriteFile(path, []byte(fm), 0600)).To(Succeed())
}

var _ = Describe("PromptCounter", func() {
	var (
		ctx     context.Context
		dir1    string
		dir2    string
		counter prompt.Counter
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		dir1, err = os.MkdirTemp("", "counter-test-1-*")
		Expect(err).To(BeNil())
		dir2, err = os.MkdirTemp("", "counter-test-2-*")
		Expect(err).To(BeNil())
		counter = prompt.NewCounter(dir1, dir2)
	})

	AfterEach(func() {
		_ = os.RemoveAll(dir1)
		_ = os.RemoveAll(dir2)
	})

	It("returns 0/0 when no prompts exist", func() {
		completed, total, err := counter.CountBySpec(ctx, "017")
		Expect(err).To(BeNil())
		Expect(completed).To(Equal(0))
		Expect(total).To(Equal(0))
	})

	It("counts prompts across directories", func() {
		writeLinkedPrompt(filepath.Join(dir1, "001-a.md"), "queued", "017")
		writeLinkedPrompt(filepath.Join(dir2, "002-b.md"), "completed", "017")

		completed, total, err := counter.CountBySpec(ctx, "017")
		Expect(err).To(BeNil())
		Expect(total).To(Equal(2))
		Expect(completed).To(Equal(1))
	})

	It("ignores prompts linked to other specs", func() {
		writeLinkedPrompt(filepath.Join(dir1, "001-a.md"), "completed", "017")
		writeLinkedPrompt(filepath.Join(dir1, "002-b.md"), "completed", "018")

		completed, total, err := counter.CountBySpec(ctx, "017")
		Expect(err).To(BeNil())
		Expect(total).To(Equal(1))
		Expect(completed).To(Equal(1))
	})

	It("ignores prompts with no spec field", func() {
		writeLinkedPrompt(filepath.Join(dir1, "001-a.md"), "completed", "")
		writeLinkedPrompt(filepath.Join(dir1, "002-b.md"), "completed", "017")

		completed, total, err := counter.CountBySpec(ctx, "017")
		Expect(err).To(BeNil())
		Expect(total).To(Equal(1))
		Expect(completed).To(Equal(1))
	})

	It("returns 0/0 for non-existent directory", func() {
		c := prompt.NewCounter("/nonexistent/path")
		completed, total, err := c.CountBySpec(ctx, "017")
		Expect(err).To(BeNil())
		Expect(completed).To(Equal(0))
		Expect(total).To(Equal(0))
	})

	It("counts prompts with multi-spec array that includes the target spec", func() {
		// Prompt belongs to both 017 and 019
		fm := "---\nstatus: completed\nspec: [\"017\", \"019\"]\n---\n# Test\n\nContent.\n"
		Expect(os.WriteFile(filepath.Join(dir1, "001-multi.md"), []byte(fm), 0600)).To(Succeed())
		// Prompt belongs only to 018
		writeLinkedPrompt(filepath.Join(dir1, "002-other.md"), "completed", "018")

		completed, total, err := counter.CountBySpec(ctx, "017")
		Expect(err).To(BeNil())
		Expect(total).To(Equal(1))
		Expect(completed).To(Equal(1))

		// Also counts for 019
		completed, total, err = counter.CountBySpec(ctx, "019")
		Expect(err).To(BeNil())
		Expect(total).To(Equal(1))
		Expect(completed).To(Equal(1))
	})
})
