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

var _ = Describe("ScenarioShowCommand", func() {
	var (
		lister          *mocks.ScenarioLister
		scenarioShowCmd cmd.ScenarioShowCommand
		ctx             context.Context
	)

	BeforeEach(func() {
		lister = &mocks.ScenarioLister{}
		scenarioShowCmd = cmd.NewScenarioShowCommand(lister)
		ctx = context.Background()
	})

	Describe("Run", func() {
		It("returns error when no id provided", func() {
			err := scenarioShowCmd.Run(ctx, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("scenario identifier required"))
		})

		It("returns error when no match found", func() {
			lister.FindReturns([]*scenario.ScenarioFile{}, nil)
			err := scenarioShowCmd.Run(ctx, []string{"999"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no scenario matching"))
		})

		It("writes raw content to stdout for single match", func() {
			lister.FindReturns([]*scenario.ScenarioFile{
				{
					Name:       "001-workflow-direct",
					RawContent: []byte("# Workflow Direct\nbody content"),
				},
			}, nil)
			err := scenarioShowCmd.Run(ctx, []string{"001"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error when multiple matches found", func() {
			lister.FindReturns([]*scenario.ScenarioFile{
				{Name: "001-workflow-direct", RawContent: []byte("# A")},
				{Name: "002-workflow-pr", RawContent: []byte("# B")},
			}, nil)
			err := scenarioShowCmd.Run(ctx, []string{"workflow"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ambiguous"))
		})

		It("returns error when lister.Find fails", func() {
			lister.FindReturns(nil, errors.New("find error"))
			err := scenarioShowCmd.Run(ctx, []string{"001"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("find scenario"))
		})
	})
})
