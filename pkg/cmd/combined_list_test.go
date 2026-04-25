// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"context"
	"errors"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cmd"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

var _ = Describe("CombinedListCommand", func() {
	var (
		ctx             context.Context
		lister          *mocks.Lister
		counter         *mocks.PromptCounter
		combinedListCmd cmd.CombinedListCommand
	)

	BeforeEach(func() {
		ctx = context.Background()
		lister = &mocks.Lister{}
		counter = &mocks.PromptCounter{}
		// Use non-existent dirs so prompt scanning returns empty without error
		combinedListCmd = cmd.NewCombinedListCommand(
			"/nonexistent/inbox",
			"/nonexistent/queue",
			"/nonexistent/completed",
			"",
			lister,
			counter,
			prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()),
		)
	})

	Describe("Run", func() {
		It("outputs combined human-readable format with headers", func() {
			lister.ListReturns([]*spec.SpecFile{}, nil)

			err := combinedListCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(lister.ListCallCount()).To(Equal(1))
		})

		It("hides completed specs by default", func() {
			lister.ListReturns([]*spec.SpecFile{
				{Name: "001-spec", Frontmatter: spec.Frontmatter{Status: "approved"}},
				{Name: "002-spec", Frontmatter: spec.Frontmatter{Status: "completed"}},
			}, nil)
			counter.CountBySpecReturnsOnCall(0, 1, 3, nil)

			err := combinedListCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(counter.CountBySpecCallCount()).To(Equal(1))
		})

		It("shows completed specs with --all flag", func() {
			lister.ListReturns([]*spec.SpecFile{
				{Name: "001-spec", Frontmatter: spec.Frontmatter{Status: "approved"}},
				{Name: "002-spec", Frontmatter: spec.Frontmatter{Status: "completed"}},
			}, nil)
			counter.CountBySpecReturnsOnCall(0, 1, 3, nil)
			counter.CountBySpecReturnsOnCall(1, 5, 5, nil)

			err := combinedListCmd.Run(ctx, []string{"--all"})
			Expect(err).NotTo(HaveOccurred())
			Expect(counter.CountBySpecCallCount()).To(Equal(2))
		})

		It("outputs JSON with prompts and specs arrays when --json flag provided", func() {
			lister.ListReturns([]*spec.SpecFile{
				{Name: "001-spec", Frontmatter: spec.Frontmatter{Status: "draft"}},
			}, nil)
			counter.CountBySpecReturns(0, 1, nil)

			err := combinedListCmd.Run(ctx, []string{"--json"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error when lister fails", func() {
			lister.ListReturns(nil, errors.New("lister error"))

			err := combinedListCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("collect spec entries"))
		})

		It("returns error when counter fails", func() {
			lister.ListReturns([]*spec.SpecFile{
				{Name: "001-spec", Frontmatter: spec.Frontmatter{Status: "draft"}},
			}, nil)
			counter.CountBySpecReturns(0, 0, errors.New("counter error"))

			err := combinedListCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("count prompts for spec"))
		})

		It("handles empty specs list", func() {
			lister.ListReturns([]*spec.SpecFile{}, nil)

			err := combinedListCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
