// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package promptenricher_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/promptenricher"
	"github.com/bborbe/dark-factory/pkg/report"
)

var _ = Describe("Enricher", func() {
	var (
		ctx          context.Context
		releaser     *mocks.Releaser
		resolverMock *mocks.Resolver
	)

	BeforeEach(func() {
		ctx = context.Background()
		releaser = &mocks.Releaser{}
		releaser.HasChangelogReturns(false)
		resolverMock = &mocks.Resolver{}
		resolverMock.ResolveReturns("", false, nil)
	})

	newEnricher := func(additionalInstructions, testCommand, validationCommand, validationPromptCriteria string) promptenricher.Enricher {
		return promptenricher.NewEnricher(
			releaser,
			additionalInstructions,
			testCommand,
			validationCommand,
			validationPromptCriteria,
			resolverMock,
		)
	}

	Describe("Enrich", func() {
		It("appends completion report suffix to content", func() {
			enricher := newEnricher("", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("base content"))
			Expect(result).To(ContainSubstring(report.MarkerStart))
		})

		It("prepends additionalInstructions when non-empty", func() {
			enricher := newEnricher("extra instructions", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(HavePrefix("extra instructions\n\nbase content"))
		})

		It("does not prepend additionalInstructions when empty", func() {
			enricher := newEnricher("", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(HavePrefix("base content"))
		})

		It("appends changelog suffix when HasChangelog returns true", func() {
			releaser.HasChangelogReturns(true)
			enricher := newEnricher("", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("Update CHANGELOG.md"))
		})

		It("does not append changelog suffix when HasChangelog returns false", func() {
			releaser.HasChangelogReturns(false)
			enricher := newEnricher("", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Update CHANGELOG.md"))
		})

		It("appends test command suffix when testCommand is non-empty", func() {
			enricher := newEnricher("", "make test", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("make test"))
			Expect(result).To(ContainSubstring("Fast Feedback Command"))
		})

		It("does not append test command suffix when testCommand is empty", func() {
			enricher := newEnricher("", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Fast Feedback Command"))
		})

		It("appends validation suffix when validationCommand is non-empty", func() {
			enricher := newEnricher("", "", "make precommit", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("make precommit"))
			Expect(result).To(ContainSubstring("Project Validation Command"))
		})

		It("does not append validation suffix when validationCommand is empty", func() {
			enricher := newEnricher("", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Project Validation Command"))
		})

		It("appends validation prompt suffix when resolver returns criteria", func() {
			resolverMock.ResolveReturns("# My Criteria\n- item one", true, nil)
			enricher := newEnricher("", "", "", "some-criteria-value")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("My Criteria"))
			Expect(result).To(ContainSubstring("Project Quality Criteria"))
		})

		It("does not append validation prompt suffix when resolver returns false", func() {
			resolverMock.ResolveReturns("", false, nil)
			enricher := newEnricher("", "", "", "")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Project Quality Criteria"))
		})

		It("does not append validation prompt suffix when resolver returns error", func() {
			resolverMock.ResolveReturns("", false, fmt.Errorf("read error"))
			enricher := newEnricher("", "", "", "bad-path")
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Project Quality Criteria"))
		})

		It("preserves suffix ordering: report, changelog, test, validation, criteria", func() {
			releaser.HasChangelogReturns(true)
			resolverMock.ResolveReturns("my criteria", true, nil)
			enricher := newEnricher("", "make test", "make precommit", "my criteria")
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
