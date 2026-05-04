// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor_test

import (
	"context"
	stderrors "errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/processor"
)

var _ = Describe("branchWorkflowExecutor setupInPlaceBranch", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("when FetchAndVerifyBranch succeeds (branch exists)", func() {
		It("calls DiscardUncommittedInPaths before Switch", func() {
			fakeBrancher := &mocks.Brancher{}
			fakeBrancher.IsCleanIgnoringReturns([]string{}, nil)
			fakeBrancher.DefaultBranchReturns("master", nil)
			fakeBrancher.FetchAndVerifyBranchReturns(nil)
			fakeBrancher.SwitchReturns(nil)
			fakeBrancher.DiscardUncommittedInPathsReturns(nil)

			prefixes := []string{"prompts/in-progress/", "prompts/completed/"}
			deps := processor.WorkflowDeps{
				Brancher:           fakeBrancher,
				IgnorePathPrefixes: prefixes,
			}
			err := processor.SetupInPlaceBranchForTest(deps, ctx, "dark-factory/test-prompt")
			Expect(err).NotTo(HaveOccurred())

			// DiscardUncommittedInPaths must be called with the prefix list.
			Expect(fakeBrancher.DiscardUncommittedInPathsCallCount()).To(Equal(1))
			_, gotPrefixes := fakeBrancher.DiscardUncommittedInPathsArgsForCall(0)
			Expect(gotPrefixes).To(Equal(prefixes))

			// Switch must be called (branch existed); CreateAndSwitch must not.
			Expect(fakeBrancher.SwitchCallCount()).To(Equal(1))
			Expect(fakeBrancher.CreateAndSwitchCallCount()).To(Equal(0))
		})
	})

	Describe("when FetchAndVerifyBranch fails (branch does not exist)", func() {
		It("calls DiscardUncommittedInPaths and CreateAndSwitch", func() {
			fakeBrancher := &mocks.Brancher{}
			fakeBrancher.IsCleanIgnoringReturns([]string{}, nil)
			fakeBrancher.DefaultBranchReturns("master", nil)
			fakeBrancher.FetchAndVerifyBranchReturns(stderrors.New("not found"))
			fakeBrancher.CreateAndSwitchReturns(nil)
			fakeBrancher.DiscardUncommittedInPathsReturns(nil)

			deps := processor.WorkflowDeps{
				Brancher:           fakeBrancher,
				IgnorePathPrefixes: []string{"prompts/in-progress/"},
			}
			err := processor.SetupInPlaceBranchForTest(deps, ctx, "dark-factory/new-prompt")
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeBrancher.DiscardUncommittedInPathsCallCount()).To(Equal(1))
			Expect(fakeBrancher.SwitchCallCount()).To(Equal(0))
			Expect(fakeBrancher.CreateAndSwitchCallCount()).To(Equal(1))
		})
	})

	It("returns error when DiscardUncommittedInPaths fails", func() {
		fakeBrancher := &mocks.Brancher{}
		fakeBrancher.IsCleanIgnoringReturns([]string{}, nil)
		fakeBrancher.DefaultBranchReturns("master", nil)
		fakeBrancher.DiscardUncommittedInPathsReturns(stderrors.New("git error"))

		deps := processor.WorkflowDeps{
			Brancher:           fakeBrancher,
			IgnorePathPrefixes: []string{"prompts/"},
		}
		err := processor.SetupInPlaceBranchForTest(deps, ctx, "dark-factory/broken-prompt")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("discard bookkeeping dirt before branch switch"))

		// Neither Switch nor CreateAndSwitch must be called.
		Expect(fakeBrancher.SwitchCallCount()).To(Equal(0))
		Expect(fakeBrancher.CreateAndSwitchCallCount()).To(Equal(0))
	})

	It("aborts before discard when IsCleanIgnoring finds user-source dirt", func() {
		fakeBrancher := &mocks.Brancher{}
		fakeBrancher.IsCleanIgnoringReturns([]string{"pkg/handler/list-sprints.go"}, nil)

		deps := processor.WorkflowDeps{
			Brancher:           fakeBrancher,
			IgnorePathPrefixes: []string{"prompts/"},
		}
		err := processor.SetupInPlaceBranchForTest(deps, ctx, "dark-factory/some-prompt")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("working tree is not clean"))

		// Discard must NOT be called — IsCleanIgnoring gate runs first.
		Expect(fakeBrancher.DiscardUncommittedInPathsCallCount()).To(Equal(0))
	})
})
