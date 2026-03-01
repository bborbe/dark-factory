// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cmd"
	"github.com/bborbe/dark-factory/pkg/status"
)

var _ = Describe("StatusCommand", func() {
	var (
		ctx           context.Context
		mockChecker   *mocks.Checker
		mockFormatter *mocks.Formatter
		statusCommand cmd.StatusCommand
		testStatus    *status.Status
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockChecker = &mocks.Checker{}
		mockFormatter = &mocks.Formatter{}
		statusCommand = cmd.NewStatusCommand(mockChecker, mockFormatter)

		testStatus = &status.Status{
			Daemon:         "not running",
			QueueCount:     0,
			QueuedPrompts:  []string{},
			CompletedCount: 0,
			IdeasCount:     0,
		}
	})

	Describe("Run", func() {
		It("outputs human-readable format by default", func() {
			mockChecker.GetStatusReturns(testStatus, nil)
			mockFormatter.FormatReturns("formatted output\n")

			err := statusCommand.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockChecker.GetStatusCallCount()).To(Equal(1))
			Expect(mockFormatter.FormatCallCount()).To(Equal(1))
		})

		It("outputs JSON format with --json flag", func() {
			mockChecker.GetStatusReturns(testStatus, nil)

			err := statusCommand.Run(ctx, []string{"--json"})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockChecker.GetStatusCallCount()).To(Equal(1))
			Expect(mockFormatter.FormatCallCount()).To(Equal(0))
		})

		It("returns error when checker fails", func() {
			mockChecker.GetStatusReturns(nil, fmt.Errorf("checker error"))

			err := statusCommand.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
		})
	})
})
