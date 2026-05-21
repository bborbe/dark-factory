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

	newEnricher := func(additionalInstructions, testCommand, validationCommand, validationPromptCriteria string, hideGit bool) promptenricher.Enricher {
		return promptenricher.NewEnricher(
			releaser,
			additionalInstructions,
			testCommand,
			validationCommand,
			validationPromptCriteria,
			resolverMock,
			hideGit,
		)
	}

	Describe("Enrich", func() {
		It("appends completion report suffix to content", func() {
			enricher := newEnricher("", "", "", "", false)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("base content"))
			Expect(result).To(ContainSubstring(report.MarkerStart))
		})

		It("prepends additionalInstructions when non-empty", func() {
			enricher := newEnricher("extra instructions", "", "", "", false)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(HavePrefix("extra instructions\n\nbase content"))
		})

		It("does not prepend additionalInstructions when empty", func() {
			enricher := newEnricher("", "", "", "", false)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(HavePrefix("base content"))
		})

		It("appends changelog suffix when HasChangelog returns true", func() {
			releaser.HasChangelogReturns(true)
			enricher := newEnricher("", "", "", "", false)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("Update CHANGELOG.md"))
		})

		It("does not append changelog suffix when HasChangelog returns false", func() {
			releaser.HasChangelogReturns(false)
			enricher := newEnricher("", "", "", "", false)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Update CHANGELOG.md"))
		})

		It("appends test command suffix when testCommand is non-empty", func() {
			enricher := newEnricher("", "make test", "", "", false)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("make test"))
			Expect(result).To(ContainSubstring("Fast Feedback Command"))
		})

		It("does not append test command suffix when testCommand is empty", func() {
			enricher := newEnricher("", "", "", "", false)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Fast Feedback Command"))
		})

		It("appends validation suffix when validationCommand is non-empty", func() {
			enricher := newEnricher("", "", "make precommit", "", false)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("make precommit"))
			Expect(result).To(ContainSubstring("Project Validation Command"))
		})

		It("does not append validation suffix when validationCommand is empty", func() {
			enricher := newEnricher("", "", "", "", false)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Project Validation Command"))
		})

		It("appends validation prompt suffix when resolver returns criteria", func() {
			resolverMock.ResolveReturns("# My Criteria\n- item one", true, nil)
			enricher := newEnricher("", "", "", "some-criteria-value", false)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).To(ContainSubstring("My Criteria"))
			Expect(result).To(ContainSubstring("Project Quality Criteria"))
		})

		It("does not append validation prompt suffix when resolver returns false", func() {
			resolverMock.ResolveReturns("", false, nil)
			enricher := newEnricher("", "", "", "", false)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Project Quality Criteria"))
		})

		It("does not append validation prompt suffix when resolver returns error", func() {
			resolverMock.ResolveReturns("", false, fmt.Errorf("read error"))
			enricher := newEnricher("", "", "", "bad-path", false)
			result := enricher.Enrich(ctx, "base content")
			Expect(result).NotTo(ContainSubstring("Project Quality Criteria"))
		})

		It("preserves suffix ordering: report, changelog, test, validation, criteria", func() {
			releaser.HasChangelogReturns(true)
			resolverMock.ResolveReturns("my criteria", true, nil)
			enricher := newEnricher("", "make test", "make precommit", "my criteria", false)
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

		It("prepends hideGit fragment when hideGit=true and additionalInstructions is set", func() {
			enricher := newEnricher("PROJECT_HEADER", "", "", "", true)
			result := enricher.Enrich(ctx, "PROMPT_BODY")
			Expect(result).To(ContainSubstring("PROJECT_HEADER"))
			Expect(result).To(ContainSubstring("hideGit=true active"))
			Expect(result).To(ContainSubstring("PROMPT_BODY"))
			headerIdx := indexOf(result, "PROJECT_HEADER")
			fragmentIdx := indexOf(result, "hideGit=true active")
			promptIdx := indexOf(result, "PROMPT_BODY")
			Expect(headerIdx).To(BeNumerically("<", fragmentIdx))
			Expect(fragmentIdx).To(BeNumerically("<", promptIdx))
		})

		It(
			"prepends hideGit fragment when hideGit=true and additionalInstructions is empty",
			func() {
				enricher := newEnricher("", "", "", "", true)
				result := enricher.Enrich(ctx, "PROMPT_BODY")
				Expect(result).To(ContainSubstring("hideGit=true active"))
				Expect(result).To(ContainSubstring("PROMPT_BODY"))
				fragmentIdx := indexOf(result, "hideGit=true active")
				promptIdx := indexOf(result, "PROMPT_BODY")
				Expect(fragmentIdx).To(BeNumerically("<", promptIdx))
				Expect(result).To(HavePrefix("hideGit=true active"))
			},
		)

		It("does not prepend hideGit fragment when hideGit=false", func() {
			enricher := newEnricher("", "", "", "", false)
			result := enricher.Enrich(ctx, "PROMPT_BODY")
			Expect(result).NotTo(ContainSubstring("hideGit=true active"))
			Expect(result).To(HavePrefix("PROMPT_BODY"))
		})

		It("preserves suffix ordering with hideGit fragment", func() {
			releaser.HasChangelogReturns(true)
			resolverMock.ResolveReturns("my criteria", true, nil)
			enricher := newEnricher("HEADER", "make test", "make precommit", "my criteria", true)
			result := enricher.Enrich(ctx, "PROMPT_BODY")

			headerIdx := indexOf(result, "HEADER")
			fragmentIdx := indexOf(result, "hideGit=true active")
			promptIdx := indexOf(result, "PROMPT_BODY")
			reportIdx := indexOf(result, report.MarkerStart)
			changelogIdx := indexOf(result, "Update CHANGELOG.md")
			testIdx := indexOf(result, "Fast Feedback Command")
			validationIdx := indexOf(result, "Project Validation Command")
			criteriaIdx := indexOf(result, "Project Quality Criteria")

			Expect(headerIdx).To(BeNumerically("<", fragmentIdx))
			Expect(fragmentIdx).To(BeNumerically("<", promptIdx))
			Expect(promptIdx).To(BeNumerically("<", reportIdx))
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
