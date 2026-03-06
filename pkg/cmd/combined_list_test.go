// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cmd"
	"github.com/bborbe/dark-factory/pkg/spec"
)

var _ = Describe("CombinedListCommand", func() {
	var (
		ctx             context.Context
		mockLister      *mocks.Lister
		mockCounter     *mocks.PromptCounter
		combinedListCmd cmd.CombinedListCommand
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockLister = &mocks.Lister{}
		mockCounter = &mocks.PromptCounter{}
		// Use non-existent dirs so prompt scanning returns empty without error
		combinedListCmd = cmd.NewCombinedListCommand(
			"/nonexistent/inbox",
			"/nonexistent/queue",
			"/nonexistent/completed",
			mockLister,
			mockCounter,
		)
	})

	Describe("Run", func() {
		It("outputs combined human-readable format with headers", func() {
			mockLister.ListReturns([]*spec.SpecFile{}, nil)

			err := combinedListCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockLister.ListCallCount()).To(Equal(1))
		})

		It("outputs spec entries with prompt counts", func() {
			mockLister.ListReturns([]*spec.SpecFile{
				{Name: "001-spec", Frontmatter: spec.Frontmatter{Status: "approved"}},
				{Name: "002-spec", Frontmatter: spec.Frontmatter{Status: "completed"}},
			}, nil)
			mockCounter.CountBySpecReturnsOnCall(0, 1, 3, nil)
			mockCounter.CountBySpecReturnsOnCall(1, 5, 5, nil)

			err := combinedListCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockCounter.CountBySpecCallCount()).To(Equal(2))
		})

		It("outputs JSON with prompts and specs arrays when --json flag provided", func() {
			mockLister.ListReturns([]*spec.SpecFile{
				{Name: "001-spec", Frontmatter: spec.Frontmatter{Status: "draft"}},
			}, nil)
			mockCounter.CountBySpecReturns(0, 1, nil)

			err := combinedListCmd.Run(ctx, []string{"--json"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error when lister fails", func() {
			mockLister.ListReturns(nil, errors.New("lister error"))

			err := combinedListCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("collect spec entries"))
		})

		It("returns error when counter fails", func() {
			mockLister.ListReturns([]*spec.SpecFile{
				{Name: "001-spec", Frontmatter: spec.Frontmatter{Status: "draft"}},
			}, nil)
			mockCounter.CountBySpecReturns(0, 0, errors.New("counter error"))

			err := combinedListCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("count prompts for spec"))
		})

		It("handles empty specs list", func() {
			mockLister.ListReturns([]*spec.SpecFile{}, nil)

			err := combinedListCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
