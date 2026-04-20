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

var _ = Describe("ScenarioListCommand", func() {
	var (
		lister          *mocks.ScenarioLister
		scenarioListCmd cmd.ScenarioListCommand
		ctx             context.Context
	)

	BeforeEach(func() {
		lister = &mocks.ScenarioLister{}
		scenarioListCmd = cmd.NewScenarioListCommand(lister)
		ctx = context.Background()
	})

	Describe("Run", func() {
		It("outputs table header for empty list", func() {
			lister.ListReturns([]*scenario.ScenarioFile{}, nil)
			err := scenarioListCmd.Run(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(lister.ListCallCount()).To(Equal(1))
		})

		It("outputs table rows for scenarios", func() {
			lister.ListReturns([]*scenario.ScenarioFile{
				{
					Number:      1,
					Name:        "001-first-scenario",
					Title:       "First scenario",
					Frontmatter: scenario.Frontmatter{Status: "active"},
				},
				{
					Number:      2,
					Name:        "002-second-scenario",
					Title:       "Second scenario",
					Frontmatter: scenario.Frontmatter{Status: "draft"},
				},
			}, nil)
			err := scenarioListCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("shows unknown for empty status", func() {
			lister.ListReturns([]*scenario.ScenarioFile{
				{
					Number:      3,
					Name:        "003-no-status",
					Title:       "No status",
					Frontmatter: scenario.Frontmatter{Status: ""},
				},
			}, nil)
			err := scenarioListCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("shows unknown for unrecognized status", func() {
			lister.ListReturns([]*scenario.ScenarioFile{
				{
					Number:      4,
					Name:        "004-bad-status",
					Title:       "Bad status",
					Frontmatter: scenario.Frontmatter{Status: "bogus"},
				},
			}, nil)
			err := scenarioListCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("outputs JSON when --json flag is provided", func() {
			lister.ListReturns([]*scenario.ScenarioFile{
				{
					Number:      1,
					Name:        "001-first-scenario",
					Title:       "First scenario",
					Frontmatter: scenario.Frontmatter{Status: "active"},
				},
			}, nil)
			err := scenarioListCmd.Run(ctx, []string{"--json"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error when lister fails", func() {
			lister.ListReturns(nil, errors.New("lister error"))
			err := scenarioListCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("list scenarios"))
		})
	})
})
