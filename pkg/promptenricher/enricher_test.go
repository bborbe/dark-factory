// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package promptenricher_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/promptenricher"
	"github.com/bborbe/dark-factory/pkg/report"
)

var _ = Describe("Enricher", func() {
	var (
		ctx      context.Context
		releaser *mocks.Releaser
	)

	BeforeEach(func() {
		ctx = context.Background()
		releaser = &mocks.Releaser{}
		releaser.HasChangelogReturns(false)
	})

	Describe("Enrich", func() {
		It("appends completion report suffix to content", func() {
			enricher := promptenricher.NewEnricher(releaser, "", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("base content"))
			Expect(result).To(ContainSubstring(report.MarkerStart))
		})

		It("prepends additionalInstructions when non-empty", func() {
			enricher := promptenricher.NewEnricher(releaser, "extra instructions", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(HavePrefix("extra instructions\n\nbase content"))
		})

		It("does not prepend additionalInstructions when empty", func() {
			enricher := promptenricher.NewEnricher(releaser, "", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(HavePrefix("base content"))
		})

		It("appends changelog suffix when HasChangelog returns true", func() {
			releaser.HasChangelogReturns(true)
			enricher := promptenricher.NewEnricher(releaser, "", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("Update CHANGELOG.md"))
		})

		It("does not append changelog suffix when HasChangelog returns false", func() {
			releaser.HasChangelogReturns(false)
			enricher := promptenricher.NewEnricher(releaser, "", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Update CHANGELOG.md"))
		})

		It("appends test command suffix when testCommand is non-empty", func() {
			enricher := promptenricher.NewEnricher(releaser, "", "make test", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("make test"))
			Expect(result).To(ContainSubstring("Fast Feedback Command"))
		})

		It("does not append test command suffix when testCommand is empty", func() {
			enricher := promptenricher.NewEnricher(releaser, "", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Fast Feedback Command"))
		})

		It("appends validation suffix when validationCommand is non-empty", func() {
			enricher := promptenricher.NewEnricher(releaser, "", "", "make precommit", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("make precommit"))
			Expect(result).To(ContainSubstring("Project Validation Command"))
		})

		It("does not append validation suffix when validationCommand is empty", func() {
			enricher := promptenricher.NewEnricher(releaser, "", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Project Validation Command"))
		})

		It("appends validation prompt suffix when criteria is inline text", func() {
			enricher := promptenricher.NewEnricher(
				releaser,
				"",
				"",
				"",
				"# My Criteria\n- item one",
			)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("My Criteria"))
			Expect(result).To(ContainSubstring("Project Quality Criteria"))
		})

		It("does not append validation prompt suffix when criteria is empty", func() {
			enricher := promptenricher.NewEnricher(releaser, "", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Project Quality Criteria"))
		})

		It("appends validation prompt suffix from file when path exists", func() {
			tempDir, err := os.MkdirTemp("", "enricher-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tempDir) }()

			criteriaPath := filepath.Join(tempDir, "criteria.md")
			Expect(
				os.WriteFile(criteriaPath, []byte("# File Criteria\n- check this"), 0600),
			).To(Succeed())

			enricher := promptenricher.NewEnricher(releaser, "", "", "", criteriaPath)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("File Criteria"))
			Expect(result).To(ContainSubstring("Project Quality Criteria"))
		})

		It("does not append validation prompt suffix when file path does not exist", func() {
			enricher := promptenricher.NewEnricher(
				releaser,
				"",
				"",
				"",
				"/nonexistent/path/criteria.md",
			)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Project Quality Criteria"))
		})

		It("preserves suffix ordering: report, changelog, test, validation, criteria", func() {
			releaser.HasChangelogReturns(true)
			enricher := promptenricher.NewEnricher(
				releaser, "", "make test", "make precommit", "my criteria",
			)
			result := enricher.Enrich(ctx, "base content")

			reportIdx := indexOf(result, report.MarkerStart)
			changelogIdx := indexOf(result, "Update CHANGELOG.md")
			testIdx := indexOf(result, "Fast Feedback Command")
			validationIdx := indexOf(result, "Project Validation Command")
			criteriaIdx := indexOf(result, "Project Quality Criteria")

			Expect(reportIdx).To(BeNumerically("<", changelogIdx))
			Expect(changelogIdx).To(BeNumerically("<", testIdx))
			Expect(testIdx).To(BeNumerically("<", validationIdx))
			Expect(validationIdx).To(BeNumerically("<", criteriaIdx))
		})
	})
})

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
