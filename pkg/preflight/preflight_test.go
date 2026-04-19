// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflight_test

import (
	"context"
	"time"

	"github.com/bborbe/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/preflight"
)

var _ = Describe("truncateSHA", func() {
	It("returns first 12 chars for long SHA", func() {
		Expect(preflight.TruncateSHA("abcdef123456789")).To(Equal("abcdef123456"))
	})

	It("returns full SHA when shorter than 12", func() {
		Expect(preflight.TruncateSHA("abc")).To(Equal("abc"))
	})

	It("handles empty string", func() {
		Expect(preflight.TruncateSHA("")).To(Equal(""))
	})
})

var _ = Describe("Checker", func() {
	var (
		ctx          context.Context
		fakeNotifier *mocks.Notifier
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeNotifier = &mocks.Notifier{}
		fakeNotifier.NotifyReturns(nil)
	})

	Describe("disabled (empty command)", func() {
		It("returns true without calling the runner", func() {
			runnerCalled := false
			ch := preflight.NewCheckerWithRunner("", 0, fakeNotifier, "proj", "abc123",
				func(ctx context.Context) (string, error) {
					runnerCalled = true
					return "", nil
				},
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(runnerCalled).To(BeFalse())
			Expect(fakeNotifier.NotifyCallCount()).To(Equal(0))
		})
	})

	Describe("preflight passes", func() {
		It("returns true and does not notify", func() {
			ch := preflight.NewCheckerWithRunner(
				"make precommit",
				8*time.Hour,
				fakeNotifier,
				"proj",
				"sha1",
				func(ctx context.Context) (string, error) { return "ok output", nil },
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(fakeNotifier.NotifyCallCount()).To(Equal(0))
		})
	})

	Describe("preflight fails", func() {
		It("returns false and sends preflight_failed notification", func() {
			ch := preflight.NewCheckerWithRunner(
				"make precommit",
				8*time.Hour,
				fakeNotifier,
				"myproject",
				"sha2",
				func(ctx context.Context) (string, error) {
					return "lint error on line 42", errors.New(ctx, "exit status 1")
				},
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
			Expect(fakeNotifier.NotifyCallCount()).To(Equal(1))
			_, event := fakeNotifier.NotifyArgsForCall(0)
			Expect(event.EventType).To(Equal("preflight_failed"))
			Expect(event.ProjectName).To(Equal("myproject"))
		})
	})

	Describe("caching", func() {
		It("reuses cached result within interval for same SHA", func() {
			callCount := 0
			ch := preflight.NewCheckerWithRunner(
				"make precommit",
				1*time.Hour,
				fakeNotifier,
				"proj",
				"sha3",
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

		It("re-runs when SHA changes", func() {
			callCount := 0
			ch1 := preflight.NewCheckerWithRunner(
				"make precommit",
				1*time.Hour,
				fakeNotifier,
				"proj",
				"sha-A",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			_, _ = ch1.Check(ctx)
			ch2 := preflight.NewCheckerWithRunner(
				"make precommit",
				1*time.Hour,
				fakeNotifier,
				"proj",
				"sha-B",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			_, _ = ch2.Check(ctx)
			Expect(callCount).To(Equal(2), "runner should re-run when SHA changes")
		})

		It("re-runs when interval is zero (no caching)", func() {
			callCount := 0
			ch := preflight.NewCheckerWithRunner("make precommit", 0, fakeNotifier, "proj", "sha4",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			_, _ = ch.Check(ctx)
			_, _ = ch.Check(ctx)
			Expect(callCount).To(Equal(2), "runner should always re-run when interval is 0")
		})

		It("re-runs when SHA fetcher returns empty (cache miss)", func() {
			callCount := 0
			ch := preflight.NewCheckerWithRunner(
				"make precommit",
				1*time.Hour,
				fakeNotifier,
				"proj",
				"",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			_, _ = ch.Check(ctx)
			_, _ = ch.Check(ctx)
			Expect(callCount).To(Equal(2), "empty SHA always triggers re-run")
		})
	})

	Describe("SHA fetcher error", func() {
		It("proceeds without cache when SHA fetch fails, still runs the check", func() {
			callCount := 0
			ch := preflight.NewCheckerWithSHAError(
				"make precommit",
				1*time.Hour,
				fakeNotifier,
				"proj",
				errors.New(ctx, "git not found"),
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(callCount).To(Equal(1))
		})
	})

	Describe("NewChecker constructor", func() {
		It("returns a non-nil Checker", func() {
			ch := preflight.NewChecker("", 0, "/tmp", fakeNotifier, "proj")
			Expect(ch).NotTo(BeNil())
		})

		It("disabled checker returns true immediately", func() {
			ch := preflight.NewChecker("", 0, "/tmp", fakeNotifier, "proj")
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
		})
	})

	Describe("retains runner output in the cache entry when baseline check fails", func() {
		It("notifies on failure and returns false with no error", func() {
			runner := func(_ context.Context) (string, error) {
				return "FAIL: assertion failed at foo_test.go:12", errors.New(ctx, "exit status 1")
			}
			ch := preflight.NewCheckerWithRunner(
				"make test",
				0,
				fakeNotifier,
				"proj",
				"abc123",
				runner,
			)

			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
			Expect(fakeNotifier.NotifyCallCount()).To(Equal(1))
		})
	})
})
