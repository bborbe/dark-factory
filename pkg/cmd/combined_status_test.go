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
		mockChecker       *mocks.Checker
		mockFormatter     *mocks.Formatter
		mockLister        *mocks.Lister
		mockCounter       *mocks.PromptCounter
		combinedStatusCmd cmd.CombinedStatusCommand
		testStatus        *status.Status
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockChecker = &mocks.Checker{}
		mockFormatter = &mocks.Formatter{}
		mockLister = &mocks.Lister{}
		mockCounter = &mocks.PromptCounter{}
		combinedStatusCmd = cmd.NewCombinedStatusCommand(
			mockChecker,
			mockFormatter,
			mockLister,
			mockCounter,
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
			mockChecker.GetStatusReturns(testStatus, nil)
			mockFormatter.FormatReturns("Daemon: not running\n")
			mockLister.SummaryReturns(&spec.Summary{Total: 2, Draft: 1, Approved: 1}, nil)
			mockLister.ListReturns([]*spec.SpecFile{}, nil)

			err := combinedStatusCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockChecker.GetStatusCallCount()).To(Equal(1))
			Expect(mockFormatter.FormatCallCount()).To(Equal(1))
			Expect(mockLister.SummaryCallCount()).To(Equal(1))
		})

		It("includes linked prompt counts", func() {
			mockChecker.GetStatusReturns(testStatus, nil)
			mockFormatter.FormatReturns("status\n")
			mockLister.SummaryReturns(&spec.Summary{Total: 1}, nil)
			mockLister.ListReturns([]*spec.SpecFile{
				{Name: "001-spec", Frontmatter: spec.Frontmatter{Status: "approved"}},
			}, nil)
			mockCounter.CountBySpecReturns(2, 5, nil)

			err := combinedStatusCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockCounter.CountBySpecCallCount()).To(Equal(1))
		})

		It("outputs JSON with prompts and specs keys when --json flag provided", func() {
			mockChecker.GetStatusReturns(testStatus, nil)
			mockLister.SummaryReturns(&spec.Summary{Total: 1, Draft: 1}, nil)
			mockLister.ListReturns([]*spec.SpecFile{}, nil)

			err := combinedStatusCmd.Run(ctx, []string{"--json"})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockChecker.GetStatusCallCount()).To(Equal(1))
			Expect(mockFormatter.FormatCallCount()).To(Equal(0))
		})

		It("returns error when checker fails", func() {
			mockChecker.GetStatusReturns(nil, errors.New("checker error"))

			err := combinedStatusCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get prompt status"))
		})

		It("returns error when lister summary fails", func() {
			mockChecker.GetStatusReturns(testStatus, nil)
			mockLister.SummaryReturns(nil, errors.New("summary error"))

			err := combinedStatusCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get spec summary"))
		})

		It("returns error when lister list fails", func() {
			mockChecker.GetStatusReturns(testStatus, nil)
			mockLister.SummaryReturns(&spec.Summary{}, nil)
			mockLister.ListReturns(nil, errors.New("list error"))

			err := combinedStatusCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("list specs"))
		})

		It("returns error when counter fails", func() {
			mockChecker.GetStatusReturns(testStatus, nil)
			mockLister.SummaryReturns(&spec.Summary{Total: 1}, nil)
			mockLister.ListReturns([]*spec.SpecFile{
				{Name: "001-spec", Frontmatter: spec.Frontmatter{Status: "draft"}},
			}, nil)
			mockCounter.CountBySpecReturns(0, 0, errors.New("counter error"))

			err := combinedStatusCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("count prompts for spec"))
		})
	})
})
