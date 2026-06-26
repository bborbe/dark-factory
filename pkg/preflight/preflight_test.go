// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflight_test

import (
	"context"
	"fmt"
	"time"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/preflight"
	"github.com/bborbe/dark-factory/pkg/subproc"
)

var _ = Describe("Checker", func() {
	var (
		ctx      context.Context
		notifier *mocks.Notifier
	)

	BeforeEach(func() {
		ctx = context.Background()
		notifier = &mocks.Notifier{}
		notifier.NotifyReturns(nil)
	})

	Describe("disabled (empty command)", func() {
		It("returns true without calling the runner", func() {
			runnerCalled := false
			ch := preflight.NewCheckerWithRunner("", 0, notifier, "proj",
				func(ctx context.Context) (string, error) {
					runnerCalled = true
					return "", nil
				},
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(runnerCalled).To(BeFalse())
			Expect(notifier.NotifyCallCount()).To(Equal(0))
		})
	})

	Describe("preflight passes", func() {
		It("returns true and does not notify", func() {
			ch := preflight.NewCheckerWithRunner(
				"make precommit",
				libtime.Duration(8*time.Hour),
				notifier,
				"proj",
				func(ctx context.Context) (string, error) { return "ok output", nil },
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(notifier.NotifyCallCount()).To(Equal(0))
		})
	})

	Describe("preflight fails", func() {
		It("returns false and sends preflight_failed notification", func() {
			ch := preflight.NewCheckerWithRunner(
				"make precommit",
				libtime.Duration(8*time.Hour),
				notifier,
				"myproject",
				func(ctx context.Context) (string, error) {
					return "lint error on line 42", errors.New(ctx, "exit status 1")
				},
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
			Expect(notifier.NotifyCallCount()).To(Equal(1))
			_, event := notifier.NotifyArgsForCall(0)
			Expect(event.EventType).To(Equal("preflight_failed"))
			Expect(event.ProjectName).To(Equal("myproject"))
		})
	})

	Describe("caching", func() {
		It("reuses cached result within interval", func() {
			callCount := 0
			ch := preflight.NewCheckerWithRunner(
				"make precommit",
				libtime.Duration(1*time.Hour),
				notifier,
				"proj",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			ok1, _ := ch.Check(ctx)
			ok2, _ := ch.Check(ctx)
			Expect(ok1).To(BeTrue())
			Expect(ok2).To(BeTrue())
			Expect(callCount).To(Equal(1), "runner should be called only once due to cache")
		})

		It("re-runs after interval elapses", func() {
			callCount := 0
			ch := preflight.NewCheckerWithRunner(
				"make precommit",
				libtime.Duration(10*time.Millisecond),
				notifier,
				"proj",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			_, _ = ch.Check(ctx)
			time.Sleep(50 * time.Millisecond)
			_, _ = ch.Check(ctx)
			Expect(callCount).To(Equal(2), "runner should re-run after interval elapses")
		})

		It("re-runs when interval is zero (no caching)", func() {
			callCount := 0
			ch := preflight.NewCheckerWithRunner("make precommit", 0, notifier, "proj",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			_, _ = ch.Check(ctx)
			_, _ = ch.Check(ctx)
			Expect(callCount).To(Equal(2), "runner should always re-run when interval is 0")
		})

		It("does not cache a failed preflight — next call re-runs the command", func() {
			callCount := 0
			ch := preflight.NewCheckerWithRunner(
				"make precommit",
				libtime.Duration(
					1*time.Hour,
				), // huge interval — would cache forever if failures were cached
				notifier,
				"proj",
				func(ctx context.Context) (string, error) {
					callCount++
					return "boom", errors.Wrap(ctx, fmt.Errorf("exit 1"), "preflight failed")
				},
			)

			ok1, _ := ch.Check(ctx)
			ok2, _ := ch.Check(ctx)
			Expect(ok1).To(BeFalse())
			Expect(ok2).To(BeFalse())
			Expect(
				callCount,
			).To(Equal(2), "both calls should re-run the command since failures are not cached")
		})
	})

	Describe("NewChecker constructor", func() {
		It("returns a non-nil Checker", func() {
			ch := preflight.NewChecker(
				"",
				0,
				"/tmp",
				notifier,
				"proj",
				libtime.NewCurrentDateTime(),
				subproc.NewRunner(),
			)
			Expect(ch).NotTo(BeNil())
		})

		It("disabled checker returns true immediately", func() {
			ch := preflight.NewChecker(
				"",
				0,
				"/tmp",
				notifier,
				"proj",
				libtime.NewCurrentDateTime(),
				subproc.NewRunner(),
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
		})
	})

	Describe("failure notification", func() {
		It("notifies on failure and returns false with no error", func() {
			runner := func(_ context.Context) (string, error) {
				return "FAIL: assertion failed at foo_test.go:12", errors.New(ctx, "exit status 1")
			}
			ch := preflight.NewCheckerWithRunner(
				"make test",
				0,
				notifier,
				"proj",
				runner,
			)

			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
			Expect(notifier.NotifyCallCount()).To(Equal(1))
		})
	})
})
