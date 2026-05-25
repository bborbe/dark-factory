// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflightconditions_test

import (
	"context"
	stderrors "errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/preflightconditions"
)

var _ = Describe("Conditions", func() {
	var (
		ctx              context.Context
		preflightChecker *mocks.PreflightChecker
		gitLockChecker   *mocks.GitLockChecker
		dirtyChecker     *mocks.DirtyFileChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		preflightChecker = &mocks.PreflightChecker{}
		gitLockChecker = &mocks.GitLockChecker{}
		dirtyChecker = &mocks.DirtyFileChecker{}
	})

	Context("preflight checker", func() {
		It("skips preflight check when checker is nil", func() {
			c := preflightconditions.NewConditions(nil, nil, nil, 0)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
		})

		It("returns (false, nil) when preflight passes", func() {
			preflightChecker.CheckReturns(true, nil)
			c := preflightconditions.NewConditions(preflightChecker, nil, nil, 0)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
			Expect(preflightChecker.CheckCallCount()).To(Equal(1))
		})

		It("returns (false, ErrPreflightFailed) when preflight returns false", func() {
			preflightChecker.CheckReturns(false, nil)
			c := preflightconditions.NewConditions(preflightChecker, nil, nil, 0)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).To(HaveOccurred())
			Expect(stderrors.Is(err, preflightconditions.ErrPreflightFailed)).To(BeTrue())
			Expect(skip).To(BeFalse())
		})

		It("returns (false, ErrPreflightFailed) when preflight returns an error", func() {
			preflightChecker.CheckReturns(false, stderrors.New("internal error"))
			c := preflightconditions.NewConditions(preflightChecker, nil, nil, 0)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).To(HaveOccurred())
			Expect(stderrors.Is(err, preflightconditions.ErrPreflightFailed)).To(BeTrue())
			Expect(skip).To(BeFalse())
		})
	})

	Context("git index lock", func() {
		It("returns (true, nil) when git lock exists", func() {
			gitLockChecker.ExistsReturns(true)
			c := preflightconditions.NewConditions(nil, gitLockChecker, nil, 0)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeTrue())
		})

		It("returns (false, nil) when git lock does not exist", func() {
			gitLockChecker.ExistsReturns(false)
			c := preflightconditions.NewConditions(nil, gitLockChecker, nil, 0)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
		})

		It("skips git lock check when checker is nil", func() {
			c := preflightconditions.NewConditions(nil, nil, nil, 0)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
		})
	})

	Context("dirty file threshold", func() {
		It("skips dirty file check when threshold is 0", func() {
			c := preflightconditions.NewConditions(nil, nil, dirtyChecker, 0)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
			Expect(dirtyChecker.CountDirtyFilesCallCount()).To(Equal(0))
		})

		It("skips dirty file check when checker is nil", func() {
			c := preflightconditions.NewConditions(nil, nil, nil, 10)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
		})

		It("returns (false, nil) when dirty count is within threshold", func() {
			dirtyChecker.CountDirtyFilesReturns(5, nil)
			c := preflightconditions.NewConditions(nil, nil, dirtyChecker, 10)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
		})

		It("returns (false, nil) when dirty count equals threshold", func() {
			dirtyChecker.CountDirtyFilesReturns(10, nil)
			c := preflightconditions.NewConditions(nil, nil, dirtyChecker, 10)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
		})

		It("returns (true, nil) when dirty count exceeds threshold", func() {
			dirtyChecker.CountDirtyFilesReturns(11, nil)
			c := preflightconditions.NewConditions(nil, nil, dirtyChecker, 10)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeTrue())
		})

		It("returns (false, err) when checker returns an error", func() {
			dirtyChecker.CountDirtyFilesReturns(0, stderrors.New("git error"))
			c := preflightconditions.NewConditions(nil, nil, dirtyChecker, 10)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).To(HaveOccurred())
			Expect(skip).To(BeFalse())
		})
	})

	Context("all checks disabled", func() {
		It("returns (false, nil) when all checkers are nil and threshold is 0", func() {
			c := preflightconditions.NewConditions(nil, nil, nil, 0)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
		})
	})

	Context("check ordering", func() {
		It("returns ErrPreflightFailed before checking git lock when preflight fails", func() {
			preflightChecker.CheckReturns(false, nil)
			gitLockChecker.ExistsReturns(true)
			c := preflightconditions.NewConditions(
				preflightChecker,
				gitLockChecker,
				dirtyChecker,
				10,
			)
			_, err := c.ShouldSkip(ctx)
			Expect(stderrors.Is(err, preflightconditions.ErrPreflightFailed)).To(BeTrue())
			Expect(dirtyChecker.CountDirtyFilesCallCount()).To(Equal(0))
		})

		It("returns git lock skip before checking dirty files when lock exists", func() {
			gitLockChecker.ExistsReturns(true)
			dirtyChecker.CountDirtyFilesReturns(100, nil)
			c := preflightconditions.NewConditions(nil, gitLockChecker, dirtyChecker, 10)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeTrue())
			Expect(dirtyChecker.CountDirtyFilesCallCount()).To(Equal(0))
		})
	})
})
