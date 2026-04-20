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
	"github.com/bborbe/dark-factory/pkg/scenario"
)

var _ = Describe("ScenarioStatusCommand", func() {
	var (
		lister            *mocks.ScenarioLister
		scenarioStatusCmd cmd.ScenarioStatusCommand
		ctx               context.Context
	)

	BeforeEach(func() {
		lister = &mocks.ScenarioLister{}
		scenarioStatusCmd = cmd.NewScenarioStatusCommand(lister)
		ctx = context.Background()
	})

	Describe("Run", func() {
		It("prints status counts without unknown line when Unknown==0", func() {
			lister.SummaryReturns(&scenario.Summary{
				Idea:     2,
				Draft:    0,
				Active:   3,
				Outdated: 1,
				Unknown:  0,
				Total:    6,
			}, nil)
			err := scenarioStatusCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("prints unknown line when Unknown > 0", func() {
			lister.SummaryReturns(&scenario.Summary{
				Idea:     1,
				Draft:    1,
				Active:   1,
				Outdated: 0,
				Unknown:  2,
				Total:    5,
			}, nil)
			err := scenarioStatusCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("outputs JSON when --json flag is provided", func() {
			lister.SummaryReturns(&scenario.Summary{
				Idea:   1,
				Active: 2,
				Total:  3,
			}, nil)
			err := scenarioStatusCmd.Run(ctx, []string{"--json"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error when Summary fails", func() {
			lister.SummaryReturns(nil, errors.New("summary error"))
			err := scenarioStatusCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get scenario summary"))
		})
	})
})
