// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package promptstate_test

import (
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	prompt "github.com/bborbe/dark-factory/pkg/prompt"
	promptstate "github.com/bborbe/dark-factory/pkg/promptstate"
)

// ---- Tests for the raw-string helpers (requirement 0) ----

func TestInterpretRawTupleMatchesInterpretTuple(t *testing.T) {
	got := promptstate.InterpretRawTuple(
		promptstate.LocationInProgress,
		string(prompt.ExecutingPromptStatus),
		"c1",
		promptstate.DockerStateRunning,
	)
	if got != promptstate.StateExecuting {
		t.Fatalf("InterpretRawTuple: want StateExecuting, got %s", got)
	}
}

func TestIsPreExecutionStatusApproved(t *testing.T) {
	if !promptstate.IsPreExecutionStatus("approved") {
		t.Fatal("want IsPreExecutionStatus(approved) == true")
	}
}

func TestIsPreExecutionStatusExecuting(t *testing.T) {
	if promptstate.IsPreExecutionStatus("executing") {
		t.Fatal("want IsPreExecutionStatus(executing) == false")
	}
}

func TestStatusFromRaw(t *testing.T) {
	got := promptstate.StatusFromRaw("approved")
	if got != prompt.ApprovedPromptStatus {
		t.Fatalf("StatusFromRaw: want ApprovedPromptStatus, got %s", got)
	}
}

// ---- Plain testing.T functions required by AC-11 (-run 'Recover|Resume|HalfState|Cancel') ----

func TestResumeExecutingStaysExecuting(t *testing.T) {
	got := promptstate.InterpretTuple(
		promptstate.LocationInProgress,
		prompt.ExecutingPromptStatus,
		"c1",
		promptstate.DockerStateRunning,
	)
	if got != promptstate.StateExecuting {
		t.Fatalf("want StateExecuting, got %s", got)
	}
}

func TestRecoverExecutingToAborted(t *testing.T) {
	got := promptstate.InterpretTuple(
		promptstate.LocationInProgress,
		prompt.ExecutingPromptStatus,
		"c1",
		promptstate.DockerStateStopped,
	)
	if got != promptstate.StateAborted {
		t.Fatalf("want StateAborted, got %s", got)
	}
}

func TestRecoverCommittingToCompleted(t *testing.T) {
	got := promptstate.InterpretTuple(
		promptstate.LocationInProgress,
		prompt.CommittingPromptStatus,
		"c1",
		promptstate.DockerStateStopped,
	)
	if got != promptstate.StateCommitting {
		t.Fatalf("want StateCommitting, got %s", got)
	}
}

func TestHalfStateCommittingInCompletedDir(t *testing.T) {
	got := promptstate.InterpretTuple(
		promptstate.LocationCompleted,
		prompt.CommittingPromptStatus,
		"c1",
		promptstate.DockerStateStopped,
	)
	if got != promptstate.StateCompleted {
		t.Fatalf("want StateCompleted, got %s", got)
	}
}

func TestCancelInterpretsCancelled(t *testing.T) {
	got := promptstate.InterpretTuple(
		promptstate.LocationInProgress,
		prompt.CancelledPromptStatus,
		"",
		promptstate.DockerStateUnavailable,
	)
	if got != promptstate.StateCancelled {
		t.Fatalf("want StateCancelled, got %s", got)
	}
}

// TestInterpretDockerUnavailableNeverCoerces asserts that docker-unavailable does not
// coerce executing → aborted (spec Failure Mode "Docker daemon unavailable").
func TestInterpretDockerUnavailableNeverCoerces(t *testing.T) {
	got := promptstate.InterpretTuple(
		promptstate.LocationInProgress,
		prompt.ExecutingPromptStatus,
		"c1",
		promptstate.DockerStateUnavailable,
	)
	if got != promptstate.StateExecuting {
		t.Fatalf("want StateExecuting (not StateAborted), got %s", got)
	}
}

// TestInterpretUnknownStatus asserts that unrecognised status values map to StateUnknown.
func TestInterpretUnknownStatus(t *testing.T) {
	cases := []prompt.PromptStatus{
		prompt.PromptStatus("bogus-status"),
		prompt.DraftPromptStatus,
		prompt.FailedPromptStatus,
	}
	for _, s := range cases {
		got := promptstate.InterpretTuple(
			promptstate.LocationInProgress,
			s,
			"",
			promptstate.DockerStateUnavailable,
		)
		if got != promptstate.StateUnknown {
			t.Errorf("status %q: want StateUnknown, got %s", s, got)
		}
	}
}

// TestInterpretConcurrent asserts InterpretTuple is safe for concurrent calls (no data race).
func TestInterpretConcurrent(t *testing.T) {
	type call struct {
		loc    promptstate.Location
		status prompt.PromptStatus
		docker promptstate.DockerState
		want   promptstate.State
	}
	calls := []call{
		{
			promptstate.LocationInProgress,
			prompt.ApprovedPromptStatus,
			promptstate.DockerStateUnavailable,
			promptstate.StateApproved,
		},
		{
			promptstate.LocationInProgress,
			prompt.ExecutingPromptStatus,
			promptstate.DockerStateRunning,
			promptstate.StateExecuting,
		},
		{
			promptstate.LocationInProgress,
			prompt.ExecutingPromptStatus,
			promptstate.DockerStateStopped,
			promptstate.StateAborted,
		},
		{
			promptstate.LocationCompleted,
			prompt.CommittingPromptStatus,
			promptstate.DockerStateStopped,
			promptstate.StateCompleted,
		},
		{
			promptstate.LocationInProgress,
			prompt.CancelledPromptStatus,
			promptstate.DockerStateUnavailable,
			promptstate.StateCancelled,
		},
	}
	var wg sync.WaitGroup
	for _, c := range calls {
		c := c
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := promptstate.InterpretTuple(c.loc, c.status, "ctr", c.docker)
			if got != c.want {
				t.Errorf("concurrent call: want %s, got %s", c.want, got)
			}
		}()
	}
	wg.Wait()
}

// ---- Ginkgo table-driven tests covering all InterpretTuple rules ----

var _ = Describe("InterpretTuple", func() {
	DescribeTable(
		"maps inputs to the correct State",
		func(loc promptstate.Location, status prompt.PromptStatus, docker promptstate.DockerState, want promptstate.State) {
			got := promptstate.InterpretTuple(loc, status, "ctr", docker)
			Expect(got).To(Equal(want))
		},
		Entry(
			"approved → StateApproved",
			promptstate.LocationInProgress,
			prompt.ApprovedPromptStatus,
			promptstate.DockerStateUnavailable,
			promptstate.StateApproved,
		),
		Entry(
			"executing + running → StateExecuting",
			promptstate.LocationInProgress,
			prompt.ExecutingPromptStatus,
			promptstate.DockerStateRunning,
			promptstate.StateExecuting,
		),
		Entry(
			"executing + stopped → StateAborted",
			promptstate.LocationInProgress,
			prompt.ExecutingPromptStatus,
			promptstate.DockerStateStopped,
			promptstate.StateAborted,
		),
		Entry(
			"executing + unavailable → StateExecuting (no coerce)",
			promptstate.LocationInProgress,
			prompt.ExecutingPromptStatus,
			promptstate.DockerStateUnavailable,
			promptstate.StateExecuting,
		),
		Entry(
			"committing + in-progress dir → StateCommitting",
			promptstate.LocationInProgress,
			prompt.CommittingPromptStatus,
			promptstate.DockerStateStopped,
			promptstate.StateCommitting,
		),
		Entry(
			"committing + completed dir → StateCompleted (half-state PR #30)",
			promptstate.LocationCompleted,
			prompt.CommittingPromptStatus,
			promptstate.DockerStateStopped,
			promptstate.StateCompleted,
		),
		Entry(
			"completed → StateCompleted",
			promptstate.LocationInProgress,
			prompt.CompletedPromptStatus,
			promptstate.DockerStateUnavailable,
			promptstate.StateCompleted,
		),
		Entry(
			"cancelled → StateCancelled",
			promptstate.LocationInProgress,
			prompt.CancelledPromptStatus,
			promptstate.DockerStateUnavailable,
			promptstate.StateCancelled,
		),
		Entry(
			"pending_verification → StatePendingVerification",
			promptstate.LocationInProgress,
			prompt.PendingVerificationPromptStatus,
			promptstate.DockerStateUnavailable,
			promptstate.StatePendingVerification,
		),
		Entry(
			"idea → StateUnknown",
			promptstate.LocationInProgress,
			prompt.IdeaPromptStatus,
			promptstate.DockerStateUnavailable,
			promptstate.StateUnknown,
		),
		Entry(
			"draft → StateUnknown",
			promptstate.LocationInProgress,
			prompt.DraftPromptStatus,
			promptstate.DockerStateUnavailable,
			promptstate.StateUnknown,
		),
		Entry(
			"failed → StateUnknown",
			promptstate.LocationInProgress,
			prompt.FailedPromptStatus,
			promptstate.DockerStateUnavailable,
			promptstate.StateUnknown,
		),
		Entry(
			"in_review → StateUnknown",
			promptstate.LocationInProgress,
			prompt.InReviewPromptStatus,
			promptstate.DockerStateUnavailable,
			promptstate.StateUnknown,
		),
		Entry(
			"rejected → StateUnknown",
			promptstate.LocationInProgress,
			prompt.RejectedPromptStatus,
			promptstate.DockerStateUnavailable,
			promptstate.StateUnknown,
		),
		Entry(
			"bogus string → StateUnknown",
			promptstate.LocationInProgress,
			prompt.PromptStatus("bogus"),
			promptstate.DockerStateUnavailable,
			promptstate.StateUnknown,
		),
	)
})
