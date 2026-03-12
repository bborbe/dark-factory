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

var _ = Describe("SpecListCommand", func() {
	var (
		lister      *mocks.Lister
		counter     *mocks.PromptCounter
		specListCmd cmd.SpecListCommand
		ctx         context.Context
	)

	BeforeEach(func() {
		lister = &mocks.Lister{}
		counter = &mocks.PromptCounter{}
		specListCmd = cmd.NewSpecListCommand(lister, counter)
		ctx = context.Background()
	})

	Describe("Run", func() {
		It("outputs table for empty list", func() {
			lister.ListReturns([]*spec.SpecFile{}, nil)
			err := specListCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(lister.ListCallCount()).To(Equal(1))
		})

		It("outputs table with specs", func() {
			lister.ListReturns([]*spec.SpecFile{
				{
					Name:        "001-my-spec",
					Frontmatter: spec.Frontmatter{Status: "draft"},
				},
				{
					Name:        "002-another",
					Frontmatter: spec.Frontmatter{Status: "approved"},
				},
			}, nil)

			err := specListCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("outputs JSON when --json flag is provided", func() {
			lister.ListReturns([]*spec.SpecFile{
				{
					Name:        "001-my-spec",
					Frontmatter: spec.Frontmatter{Status: "draft"},
				},
			}, nil)

			err := specListCmd.Run(ctx, []string{"--json"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error when lister fails", func() {
			lister.ListReturns(nil, errors.New("lister error"))
			err := specListCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("list specs"))
		})

		It("handles spec with empty status", func() {
			lister.ListReturns([]*spec.SpecFile{
				{
					Name:        "001-no-status",
					Frontmatter: spec.Frontmatter{},
				},
			}, nil)

			err := specListCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("renders verifying spec with ! prefix in STATUS column", func() {
			lister.ListReturns([]*spec.SpecFile{
				{
					Name:        "021-verifying-spec",
					Frontmatter: spec.Frontmatter{Status: "verifying"},
				},
			}, nil)

			err := specListCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(counter.CountBySpecCallCount()).To(Equal(1))
		})

		It("hides completed specs by default", func() {
			lister.ListReturns([]*spec.SpecFile{
				{Name: "017-spec", Frontmatter: spec.Frontmatter{Status: "approved"}},
				{Name: "019-spec", Frontmatter: spec.Frontmatter{Status: "completed"}},
			}, nil)
			counter.CountBySpecReturnsOnCall(0, 0, 3, nil)

			err := specListCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(counter.CountBySpecCallCount()).To(Equal(1))
		})

		It("shows completed specs with --all flag", func() {
			lister.ListReturns([]*spec.SpecFile{
				{Name: "017-spec", Frontmatter: spec.Frontmatter{Status: "approved"}},
				{Name: "019-spec", Frontmatter: spec.Frontmatter{Status: "completed"}},
			}, nil)
			counter.CountBySpecReturnsOnCall(0, 0, 3, nil)
			counter.CountBySpecReturnsOnCall(1, 5, 5, nil)

			err := specListCmd.Run(ctx, []string{"--all"})
			Expect(err).NotTo(HaveOccurred())
			Expect(counter.CountBySpecCallCount()).To(Equal(2))
		})
	})
})
