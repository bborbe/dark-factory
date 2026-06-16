// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healthcheckgate_test

import (
	"context"
	"fmt"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/healthcheckgate"
)

var _ = Describe("Gate", func() {
	var (
		ctx        context.Context
		fakeCmd    *mocks.HealthcheckGateCommand
		fakeCache  *mocks.HealthcheckGateCache
		fakeNotify *mocks.Notifier
		startTime  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeCmd = &mocks.HealthcheckGateCommand{}
		fakeCache = &mocks.HealthcheckGateCache{}
		fakeNotify = &mocks.Notifier{}
		startTime = time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	})

	Context("disabled (enabled=false)", func() {
		var g healthcheckgate.Gate

		BeforeEach(func() {
			cdt := libtime.NewCurrentDateTime()
			cdt.SetNow(libtime.DateTime(startTime))
			g = healthcheckgate.NewGate(
				false,
				false,
				time.Hour,
				fakeCmd,
				fakeCache,
				fakeNotify,
				"proj",
				cdt,
			)
		})

		It("returns nil", func() {
			Expect(g.Check(ctx)).To(Succeed())
		})

		It("does not call run", func() {
			Expect(g.Check(ctx)).To(Succeed())
			Expect(fakeCmd.RunCallCount()).To(Equal(0))
		})

		It("does not call cache Fresh", func() {
			Expect(g.Check(ctx)).To(Succeed())
			Expect(fakeCache.FreshCallCount()).To(Equal(0))
		})
	})

	Context("skip (skip=true)", func() {
		var g healthcheckgate.Gate

		BeforeEach(func() {
			cdt := libtime.NewCurrentDateTime()
			cdt.SetNow(libtime.DateTime(startTime))
			g = healthcheckgate.NewGate(
				true,
				true,
				time.Hour,
				fakeCmd,
				fakeCache,
				fakeNotify,
				"proj",
				cdt,
			)
		})

		It("returns nil", func() {
			Expect(g.Check(ctx)).To(Succeed())
		})

		It("does not call run", func() {
			Expect(g.Check(ctx)).To(Succeed())
			Expect(fakeCmd.RunCallCount()).To(Equal(0))
		})

		It("does not call cache Fresh or Write", func() {
			Expect(g.Check(ctx)).To(Succeed())
			Expect(fakeCache.FreshCallCount()).To(Equal(0))
			Expect(fakeCache.WriteCallCount()).To(Equal(0))
		})
	})

	Context("cache hit", func() {
		var g healthcheckgate.Gate

		BeforeEach(func() {
			cdt := libtime.NewCurrentDateTime()
			cdt.SetNow(libtime.DateTime(startTime))
			g = healthcheckgate.NewGate(
				true,
				false,
				time.Hour,
				fakeCmd,
				fakeCache,
				fakeNotify,
				"proj",
				cdt,
			)
			fakeCache.FreshReturns(true)
		})

		It("returns nil", func() {
			Expect(g.Check(ctx)).To(Succeed())
		})

		It("does not call run", func() {
			Expect(g.Check(ctx)).To(Succeed())
			Expect(fakeCmd.RunCallCount()).To(Equal(0))
		})

		It("does not call cache Write", func() {
			Expect(g.Check(ctx)).To(Succeed())
			Expect(fakeCache.WriteCallCount()).To(Equal(0))
		})
	})

	Context("cache miss + probes pass", func() {
		var g healthcheckgate.Gate

		BeforeEach(func() {
			endTime := startTime.Add(100 * time.Millisecond)
			callN := 0
			getter := libtime.CurrentDateTimeGetterFunc(func() libtime.DateTime {
				callN++
				if callN == 1 {
					return libtime.DateTime(startTime)
				}
				return libtime.DateTime(endTime)
			})
			g = healthcheckgate.NewGate(
				true,
				false,
				time.Hour,
				fakeCmd,
				fakeCache,
				fakeNotify,
				"proj",
				getter,
			)
			fakeCache.FreshReturns(false)
			fakeCmd.RunReturns(nil)
		})

		It("returns nil", func() {
			Expect(g.Check(ctx)).To(Succeed())
		})

		It("calls run once with empty args", func() {
			Expect(g.Check(ctx)).To(Succeed())
			Expect(fakeCmd.RunCallCount()).To(Equal(1))
			_, args := fakeCmd.RunArgsForCall(0)
			Expect(args).To(BeEmpty())
		})

		It("writes cache once", func() {
			Expect(g.Check(ctx)).To(Succeed())
			Expect(fakeCache.WriteCallCount()).To(Equal(1))
		})
	})

	Context("cache miss + probes fail", func() {
		var g healthcheckgate.Gate

		BeforeEach(func() {
			cdt := libtime.NewCurrentDateTime()
			cdt.SetNow(libtime.DateTime(startTime))
			g = healthcheckgate.NewGate(
				true,
				false,
				time.Hour,
				fakeCmd,
				fakeCache,
				fakeNotify,
				"proj",
				cdt,
			)
			fakeCache.FreshReturns(false)
			fakeCmd.RunReturns(
				fmt.Errorf("healthcheck probe \"docker\" failed: daemon unreachable"),
			)
		})

		It("returns error with prefix healthcheck failed:", func() {
			err := g.Check(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("healthcheck failed:"))
		})

		It("does not write cache", func() {
			_ = g.Check(ctx)
			Expect(fakeCache.WriteCallCount()).To(Equal(0))
		})

		It("fires healthcheck_failed notification once", func() {
			_ = g.Check(ctx)
			Expect(fakeNotify.NotifyCallCount()).To(Equal(1))
			_, event := fakeNotify.NotifyArgsForCall(0)
			Expect(event.EventType).To(Equal("healthcheck_failed"))
		})
	})
})
