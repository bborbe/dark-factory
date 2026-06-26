// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package promptstate_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	promptstate "github.com/bborbe/dark-factory/pkg/promptstate"
)

// TestTransitionTableCoversAllStates verifies that every State in AvailableStates
// is reachable as a source or sink in the transition table, via the exported surface.
func TestTransitionTableCoversAllStates(t *testing.T) {
	// Known-good edges that must return true.
	allowed := [][2]promptstate.State{
		{promptstate.StateApproved, promptstate.StateExecuting},
		{promptstate.StateApproved, promptstate.StateCancelled},
		{promptstate.StateExecuting, promptstate.StateCommitting},
		{promptstate.StateExecuting, promptstate.StateCancelled},
		{promptstate.StateExecuting, promptstate.StateAborted},
		{promptstate.StateCommitting, promptstate.StateCompleted},
		{promptstate.StateAborted, promptstate.StateApproved},
		{promptstate.StatePendingVerification, promptstate.StateCompleted},
	}
	for _, edge := range allowed {
		if !promptstate.IsValidTransition(edge[0], edge[1]) {
			t.Errorf("expected IsValidTransition(%s, %s) == true", edge[0], edge[1])
		}
	}

	// Known-bad edges that must return false.
	disallowed := [][2]promptstate.State{
		{promptstate.StateCompleted, promptstate.StateExecuting},
		{promptstate.StateUnknown, promptstate.StateApproved},
		{promptstate.StateCancelled, promptstate.StateApproved},
		{promptstate.StateCompleted, promptstate.StateApproved},
	}
	for _, edge := range disallowed {
		if promptstate.IsValidTransition(edge[0], edge[1]) {
			t.Errorf("expected IsValidTransition(%s, %s) == false", edge[0], edge[1])
		}
	}
}

// ---- Ginkgo table-driven tests for IsValidTransition ----

var _ = Describe("IsValidTransition", func() {
	DescribeTable(
		"allowed transitions return true",
		func(from, to promptstate.State) {
			Expect(promptstate.IsValidTransition(from, to)).To(BeTrue())
		},
		Entry("approved → executing", promptstate.StateApproved, promptstate.StateExecuting),
		Entry("approved → cancelled", promptstate.StateApproved, promptstate.StateCancelled),
		Entry("executing → committing", promptstate.StateExecuting, promptstate.StateCommitting),
		Entry("executing → cancelled", promptstate.StateExecuting, promptstate.StateCancelled),
		Entry("executing → aborted", promptstate.StateExecuting, promptstate.StateAborted),
		Entry("committing → completed", promptstate.StateCommitting, promptstate.StateCompleted),
		Entry("aborted → approved (recovery)", promptstate.StateAborted, promptstate.StateApproved),
		Entry(
			"pending_verification → completed",
			promptstate.StatePendingVerification,
			promptstate.StateCompleted,
		),
	)

	DescribeTable(
		"disallowed transitions return false",
		func(from, to promptstate.State) {
			Expect(promptstate.IsValidTransition(from, to)).To(BeFalse())
		},
		Entry(
			"completed → executing (terminal sink)",
			promptstate.StateCompleted,
			promptstate.StateExecuting,
		),
		Entry("unknown → approved (sentinel)", promptstate.StateUnknown, promptstate.StateApproved),
		Entry(
			"cancelled → approved (terminal sink)",
			promptstate.StateCancelled,
			promptstate.StateApproved,
		),
		Entry(
			"completed → approved (terminal sink)",
			promptstate.StateCompleted,
			promptstate.StateApproved,
		),
		Entry(
			"approved → completed (skip states)",
			promptstate.StateApproved,
			promptstate.StateCompleted,
		),
		Entry(
			"aborted → executing (not allowed)",
			promptstate.StateAborted,
			promptstate.StateExecuting,
		),
	)
})

var _ = Describe("State", func() {
	Describe("Validate", func() {
		DescribeTable("canonical states are valid",
			func(s promptstate.State) {
				Expect(s.Validate(context.Background())).To(Succeed())
			},
			Entry("approved", promptstate.StateApproved),
			Entry("executing", promptstate.StateExecuting),
			Entry("committing", promptstate.StateCommitting),
			Entry("completed", promptstate.StateCompleted),
			Entry("cancelled", promptstate.StateCancelled),
			Entry("pending_verification", promptstate.StatePendingVerification),
			Entry("aborted", promptstate.StateAborted),
		)

		DescribeTable("non-canonical states are invalid",
			func(s promptstate.State) {
				Expect(s.Validate(context.Background())).To(HaveOccurred())
			},
			Entry("unknown sentinel", promptstate.StateUnknown),
			Entry("empty string", promptstate.State("")),
			Entry("bogus value", promptstate.State("bogus")),
		)
	})

	Describe("String", func() {
		It("returns the string value", func() {
			Expect(promptstate.StateApproved.String()).To(Equal("approved"))
			Expect(promptstate.StateUnknown.String()).To(Equal("unknown"))
		})
	})

	Describe("AvailableStates", func() {
		It("contains exactly the seven canonical states", func() {
			Expect(promptstate.AvailableStates).To(HaveLen(7))
			Expect(promptstate.AvailableStates.Contains(promptstate.StateApproved)).To(BeTrue())
			Expect(promptstate.AvailableStates.Contains(promptstate.StateUnknown)).To(BeFalse())
		})
	})
})
