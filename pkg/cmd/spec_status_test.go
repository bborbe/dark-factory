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

var _ = Describe("SpecStatusCommand", func() {
	var (
		mockLister    *mocks.Lister
		mockCounter   *mocks.PromptCounter
		specStatusCmd cmd.SpecStatusCommand
		ctx           context.Context
	)

	BeforeEach(func() {
		mockLister = &mocks.Lister{}
		mockCounter = &mocks.PromptCounter{}
		specStatusCmd = cmd.NewSpecStatusCommand(mockLister, mockCounter)
		ctx = context.Background()
	})

	Describe("Run", func() {
		It("outputs human-readable summary", func() {
			mockLister.SummaryReturns(&spec.Summary{
				Total:     4,
				Draft:     1,
				Approved:  1,
				Prompted:  1,
				Completed: 1,
			}, nil)

			err := specStatusCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockLister.SummaryCallCount()).To(Equal(1))
		})

		It("outputs zero counts for empty specs dir", func() {
			mockLister.SummaryReturns(&spec.Summary{}, nil)

			err := specStatusCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("outputs JSON when --json flag is provided", func() {
			mockLister.SummaryReturns(&spec.Summary{
				Total:     2,
				Draft:     2,
				Approved:  0,
				Prompted:  0,
				Completed: 0,
			}, nil)

			err := specStatusCmd.Run(ctx, []string{"--json"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error when lister fails", func() {
			mockLister.SummaryReturns(nil, errors.New("summary error"))
			err := specStatusCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get spec summary"))
		})

		It("includes linked prompt counts from counter", func() {
			mockLister.SummaryReturns(&spec.Summary{Total: 1, Completed: 1}, nil)
			mockLister.ListReturns([]*spec.SpecFile{
				{Name: "001-my-spec", Frontmatter: spec.Frontmatter{Status: "completed"}},
			}, nil)
			mockCounter.CountBySpecReturns(3, 5, nil)

			err := specStatusCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockCounter.CountBySpecCallCount()).To(Equal(1))
		})
	})
})
