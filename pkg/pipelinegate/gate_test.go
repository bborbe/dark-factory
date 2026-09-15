// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipelinegate_test

import (
	"context"
	stderrors "errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/doctor"
	"github.com/bborbe/dark-factory/pkg/pipelinegate"
)

var _ = Describe("Gate", func() {
	var (
		ctx         context.Context
		fakeChecker *mocks.DoctorChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeChecker = &mocks.DoctorChecker{}
	})

	dirtyFindings := func() []doctor.Finding {
		return []doctor.Finding{
			{
				Category: doctor.CategoryMissingCompletedPrompt,
				Detail:   "prompt number 186 is missing from prompts/completed",
			},
			{
				Category: doctor.CategoryMissingCompletedPrompt,
				Detail:   "prompt number 187 is missing from prompts/completed",
			},
		}
	}

	Context("disabled (enabled=false)", func() {
		It("returns nil and does not call the checker", func() {
			fakeChecker.CheckReturns(dirtyFindings(), nil)

			g := pipelinegate.NewGate(false, false, fakeChecker)

			Expect(g.Check(ctx)).To(Succeed())
			Expect(fakeChecker.CheckCallCount()).To(Equal(0))
		})
	})

	Context("skip (skip=true)", func() {
		It("returns nil and does not call the checker", func() {
			fakeChecker.CheckReturns(dirtyFindings(), nil)

			g := pipelinegate.NewGate(true, true, fakeChecker)

			Expect(g.Check(ctx)).To(Succeed())
			Expect(fakeChecker.CheckCallCount()).To(Equal(0))
		})
	})

	Context("enabled + clean pipeline", func() {
		It("returns nil and calls the checker once", func() {
			fakeChecker.CheckReturns([]doctor.Finding{}, nil)

			g := pipelinegate.NewGate(true, false, fakeChecker)

			Expect(g.Check(ctx)).To(Succeed())
			Expect(fakeChecker.CheckCallCount()).To(Equal(1))
		})
	})

	Context("enabled + findings", func() {
		It("returns a findings-naming error", func() {
			fakeChecker.CheckReturns(dirtyFindings(), nil)

			g := pipelinegate.NewGate(true, false, fakeChecker)

			err := g.Check(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("pipeline not clean"))
			Expect(err.Error()).To(ContainSubstring("186"))
			Expect(err.Error()).To(ContainSubstring("187"))
			Expect(fakeChecker.CheckCallCount()).To(Equal(1))
		})
	})

	Context("checker error", func() {
		It("wraps the checker error", func() {
			fakeChecker.CheckReturns(nil, stderrors.New("boom"))

			g := pipelinegate.NewGate(true, false, fakeChecker)

			err := g.Check(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("pipeline check"))
			Expect(err.Error()).To(ContainSubstring("boom"))
		})
	})

	Context("no caching (AC8)", func() {
		It("fails on the second Check when findings appear only then", func() {
			fakeChecker.CheckReturnsOnCall(0, []doctor.Finding{}, nil)
			fakeChecker.CheckReturnsOnCall(1, dirtyFindings(), nil)

			g := pipelinegate.NewGate(true, false, fakeChecker)

			Expect(g.Check(ctx)).To(Succeed())
			Expect(g.Check(ctx)).NotTo(Succeed())
			Expect(fakeChecker.CheckCallCount()).To(Equal(2))
		})
	})

	Context("default arming", func() {
		It("ships disabled", func() {
			// The arming decision is owned outside this spec. Flipping
			// pipelinegate.DefaultEnabled makes this assertion fail on purpose,
			// so the change is reviewed.
			Expect(pipelinegate.DefaultEnabled).To(BeFalse())
		})
	})
})
