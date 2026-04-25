// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflightconditions_test

import (
	"context"
	stderrors "errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/preflightconditions"
)

// fakePreflightChecker is a test stub for preflight.Checker.
type fakePreflightChecker struct {
	ok        bool
	err       error
	callCount int
}

func (f *fakePreflightChecker) Check(_ context.Context) (bool, error) {
	f.callCount++
	return f.ok, f.err
}

// fakeGitLockChecker is a test stub for GitLockChecker.
type fakeGitLockChecker struct {
	exists bool
}

func (f *fakeGitLockChecker) Exists() bool { return f.exists }

// fakeDirtyFileChecker is a test stub for DirtyFileChecker.
type fakeDirtyFileChecker struct {
	count     int
	err       error
	callCount int
}

func (f *fakeDirtyFileChecker) CountDirtyFiles(_ context.Context) (int, error) {
	f.callCount++
	return f.count, f.err
}

var _ = Describe("Conditions", func() {
	var (
		ctx              context.Context
		preflightChecker *fakePreflightChecker
		gitLockChecker   *fakeGitLockChecker
		dirtyChecker     *fakeDirtyFileChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		preflightChecker = &fakePreflightChecker{ok: true}
		gitLockChecker = &fakeGitLockChecker{}
		dirtyChecker = &fakeDirtyFileChecker{}
	})

	Context("preflight checker", func() {
		It("skips preflight check when checker is nil", func() {
			c := preflightconditions.NewConditions(nil, nil, nil, 0)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
		})

		It("returns (false, nil) when preflight passes", func() {
			preflightChecker.ok = true
			c := preflightconditions.NewConditions(preflightChecker, nil, nil, 0)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
			Expect(preflightChecker.callCount).To(Equal(1))
		})

		It("returns (false, ErrPreflightSkip) when preflight returns false", func() {
			preflightChecker.ok = false
			c := preflightconditions.NewConditions(preflightChecker, nil, nil, 0)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).To(HaveOccurred())
			Expect(stderrors.Is(err, preflightconditions.ErrPreflightSkip)).To(BeTrue())
			Expect(skip).To(BeFalse())
		})

		It("returns (false, ErrPreflightSkip) when preflight returns an error", func() {
			preflightChecker.err = stderrors.New("internal error")
			c := preflightconditions.NewConditions(preflightChecker, nil, nil, 0)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).To(HaveOccurred())
			Expect(stderrors.Is(err, preflightconditions.ErrPreflightSkip)).To(BeTrue())
			Expect(skip).To(BeFalse())
		})
	})

	Context("git index lock", func() {
		It("returns (true, nil) when git lock exists", func() {
			gitLockChecker.exists = true
			c := preflightconditions.NewConditions(nil, gitLockChecker, nil, 0)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeTrue())
		})

		It("returns (false, nil) when git lock does not exist", func() {
			gitLockChecker.exists = false
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
			Expect(dirtyChecker.callCount).To(Equal(0))
		})

		It("skips dirty file check when checker is nil", func() {
			c := preflightconditions.NewConditions(nil, nil, nil, 10)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
		})

		It("returns (false, nil) when dirty count is within threshold", func() {
			dirtyChecker.count = 5
			c := preflightconditions.NewConditions(nil, nil, dirtyChecker, 10)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
		})

		It("returns (false, nil) when dirty count equals threshold", func() {
			dirtyChecker.count = 10
			c := preflightconditions.NewConditions(nil, nil, dirtyChecker, 10)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
		})

		It("returns (true, nil) when dirty count exceeds threshold", func() {
			dirtyChecker.count = 11
			c := preflightconditions.NewConditions(nil, nil, dirtyChecker, 10)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeTrue())
		})

		It("returns (false, err) when checker returns an error", func() {
			dirtyChecker.err = stderrors.New("git error")
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
		It("returns ErrPreflightSkip before checking git lock when preflight fails", func() {
			preflightChecker.ok = false
			gitLockChecker.exists = true
			c := preflightconditions.NewConditions(
				preflightChecker,
				gitLockChecker,
				dirtyChecker,
				10,
			)
			_, err := c.ShouldSkip(ctx)
			Expect(stderrors.Is(err, preflightconditions.ErrPreflightSkip)).To(BeTrue())
			Expect(dirtyChecker.callCount).To(Equal(0))
		})

		It("returns git lock skip before checking dirty files when lock exists", func() {
			gitLockChecker.exists = true
			dirtyChecker.count = 100
			c := preflightconditions.NewConditions(nil, gitLockChecker, dirtyChecker, 10)
			skip, err := c.ShouldSkip(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeTrue())
			Expect(dirtyChecker.callCount).To(Equal(0))
		})
	})
})
