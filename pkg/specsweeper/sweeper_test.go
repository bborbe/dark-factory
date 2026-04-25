// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package specsweeper_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/spec"
	"github.com/bborbe/dark-factory/pkg/specsweeper"
)

var _ = Describe("Sweeper", func() {
	var (
		ctx           context.Context
		specLister    *mocks.Lister
		autoCompleter *mocks.AutoCompleter
		sweeper       specsweeper.Sweeper
	)

	BeforeEach(func() {
		ctx = context.Background()
		specLister = &mocks.Lister{}
		autoCompleter = &mocks.AutoCompleter{}
		sweeper = specsweeper.NewSweeper(specLister, autoCompleter)
	})

	Describe("Sweep", func() {
		Context("no specs", func() {
			BeforeEach(func() {
				specLister.ListReturns([]*spec.SpecFile{}, nil)
			})

			It("returns 0 transitioned with no error", func() {
				count, err := sweeper.Sweep(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(0))
				Expect(autoCompleter.CheckAndCompleteCallCount()).To(Equal(0))
			})
		})

		Context("no prompted specs", func() {
			BeforeEach(func() {
				sf := &spec.SpecFile{}
				sf.Frontmatter.Status = string(spec.StatusApproved)
				sf.Name = "some-spec"
				specLister.ListReturns([]*spec.SpecFile{sf}, nil)
			})

			It("returns 0 and does not call CheckAndComplete", func() {
				count, err := sweeper.Sweep(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(0))
				Expect(autoCompleter.CheckAndCompleteCallCount()).To(Equal(0))
			})
		})

		Context("prompted spec succeeds", func() {
			BeforeEach(func() {
				sf := &spec.SpecFile{}
				sf.Frontmatter.Status = string(spec.StatusPrompted)
				sf.Name = "prompted-spec"
				specLister.ListReturns([]*spec.SpecFile{sf}, nil)
				autoCompleter.CheckAndCompleteReturns(nil)
			})

			It("returns 1 transitioned with no error", func() {
				count, err := sweeper.Sweep(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(1))
				Expect(autoCompleter.CheckAndCompleteCallCount()).To(Equal(1))
				_, specName := autoCompleter.CheckAndCompleteArgsForCall(0)
				Expect(specName).To(Equal("prompted-spec"))
			})
		})

		Context("prompted spec returns error", func() {
			BeforeEach(func() {
				sf := &spec.SpecFile{}
				sf.Frontmatter.Status = string(spec.StatusPrompted)
				sf.Name = "failing-spec"
				specLister.ListReturns([]*spec.SpecFile{sf}, nil)
				autoCompleter.CheckAndCompleteReturns(errors.New("complete failed"))
			})

			It("propagates the error and returns count so far", func() {
				count, err := sweeper.Sweep(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("complete failed"))
				Expect(count).To(Equal(0))
			})
		})

		Context("list error", func() {
			BeforeEach(func() {
				specLister.ListReturns(nil, errors.New("list failed"))
			})

			It("propagates the error", func() {
				count, err := sweeper.Sweep(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("list failed"))
				Expect(count).To(Equal(0))
				Expect(autoCompleter.CheckAndCompleteCallCount()).To(Equal(0))
			})
		})

		Context("multiple specs with mix of statuses", func() {
			BeforeEach(func() {
				prompted1 := &spec.SpecFile{}
				prompted1.Frontmatter.Status = string(spec.StatusPrompted)
				prompted1.Name = "prompted-1"

				approved := &spec.SpecFile{}
				approved.Frontmatter.Status = string(spec.StatusApproved)
				approved.Name = "approved-spec"

				prompted2 := &spec.SpecFile{}
				prompted2.Frontmatter.Status = string(spec.StatusPrompted)
				prompted2.Name = "prompted-2"

				specLister.ListReturns([]*spec.SpecFile{prompted1, approved, prompted2}, nil)
				autoCompleter.CheckAndCompleteReturns(nil)
			})

			It("only calls CheckAndComplete for prompted specs", func() {
				count, err := sweeper.Sweep(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(2))
				Expect(autoCompleter.CheckAndCompleteCallCount()).To(Equal(2))
			})
		})
	})
})
