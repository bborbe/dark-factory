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
	"github.com/bborbe/dark-factory/pkg/status"
)

var _ = Describe("CombinedStatusCommand", func() {
	var (
		ctx               context.Context
		checker           *mocks.Checker
		formatter         *mocks.Formatter
		lister            *mocks.Lister
		counter           *mocks.PromptCounter
		combinedStatusCmd cmd.CombinedStatusCommand
		testStatus        *status.Status
	)

	BeforeEach(func() {
		ctx = context.Background()
		checker = &mocks.Checker{}
		formatter = &mocks.Formatter{}
		lister = &mocks.Lister{}
		counter = &mocks.PromptCounter{}
		combinedStatusCmd = cmd.NewCombinedStatusCommand(
			checker,
			formatter,
			lister,
			counter,
		)

		testStatus = &status.Status{
			Daemon:         "not running",
			QueueCount:     0,
			QueuedPrompts:  []string{},
			CompletedCount: 0,
		}
	})

	Describe("Run", func() {
		It("outputs combined human-readable format", func() {
			checker.GetStatusReturns(testStatus, nil)
			formatter.FormatReturns("Daemon: not running\n")
			lister.SummaryReturns(&spec.Summary{Total: 2, Draft: 1, Approved: 1}, nil)
			lister.ListReturns([]*spec.SpecFile{}, nil)

			err := combinedStatusCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(checker.GetStatusCallCount()).To(Equal(1))
			Expect(formatter.FormatCallCount()).To(Equal(1))
			Expect(lister.SummaryCallCount()).To(Equal(1))
		})

		It("includes linked prompt counts", func() {
			checker.GetStatusReturns(testStatus, nil)
			formatter.FormatReturns("status\n")
			lister.SummaryReturns(&spec.Summary{Total: 1}, nil)
			lister.ListReturns([]*spec.SpecFile{
				{Name: "001-spec", Frontmatter: spec.Frontmatter{Status: "approved"}},
			}, nil)
			counter.CountBySpecReturns(2, 5, nil)

			err := combinedStatusCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(counter.CountBySpecCallCount()).To(Equal(1))
		})

		It("outputs JSON with prompts and specs keys when --json flag provided", func() {
			checker.GetStatusReturns(testStatus, nil)
			lister.SummaryReturns(&spec.Summary{Total: 1, Draft: 1}, nil)
			lister.ListReturns([]*spec.SpecFile{}, nil)

			err := combinedStatusCmd.Run(ctx, []string{"--json"})
			Expect(err).NotTo(HaveOccurred())
			Expect(checker.GetStatusCallCount()).To(Equal(1))
			Expect(formatter.FormatCallCount()).To(Equal(0))
		})

		It("returns error when checker fails", func() {
			checker.GetStatusReturns(nil, errors.New("checker error"))

			err := combinedStatusCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get prompt status"))
		})

		It("returns error when lister summary fails", func() {
			checker.GetStatusReturns(testStatus, nil)
			lister.SummaryReturns(nil, errors.New("summary error"))

			err := combinedStatusCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get spec summary"))
		})

		It("returns error when lister list fails", func() {
			checker.GetStatusReturns(testStatus, nil)
			lister.SummaryReturns(&spec.Summary{}, nil)
			lister.ListReturns(nil, errors.New("list error"))

			err := combinedStatusCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("list specs"))
		})

		It("returns error when counter fails", func() {
			checker.GetStatusReturns(testStatus, nil)
			lister.SummaryReturns(&spec.Summary{Total: 1}, nil)
			lister.ListReturns([]*spec.SpecFile{
				{Name: "001-spec", Frontmatter: spec.Frontmatter{Status: "draft"}},
			}, nil)
			counter.CountBySpecReturns(0, 0, errors.New("counter error"))

			err := combinedStatusCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("count prompts for spec"))
		})
	})
})
