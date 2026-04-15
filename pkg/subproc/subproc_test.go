// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package subproc_test

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/bborbe/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/goleak"

	"github.com/bborbe/dark-factory/pkg/subproc"
)

// capturingHandler captures slog records for test assertions.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

func (h *capturingHandler) WithGroup(_ string) slog.Handler { return h }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *capturingHandler) messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	msgs := make([]string, len(h.records))
	for i, r := range h.records {
		msgs[i] = r.Message
	}
	return msgs
}

var _ = Describe("Runner", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("RunWithWarnAndTimeoutDir", func() {
		It("fast command returns output", func() {
			r := subproc.NewRunner()
			out, err := r.RunWithWarnAndTimeoutDir(ctx, "true", "/", "true")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(BeEmpty())
		})

		It("slow command warns then succeeds", func() {
			// 50ms sleep > 30ms warn threshold → warn fires; 50ms < 1s timeout → succeeds
			h := &capturingHandler{}
			prev := slog.Default()
			slog.SetDefault(slog.New(h))
			defer slog.SetDefault(prev)

			r := subproc.NewRunnerWithThresholds(30*time.Millisecond, 1*time.Second)
			out, err := r.RunWithWarnAndTimeout(
				ctx,
				"sh-slow",
				"sh",
				"-c",
				"sleep 0.05 && echo done",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("done\n"))
			Expect(h.messages()).To(ContainElement("subprocess slow"))
		})

		It("slow command times out and returns DeadlineExceeded", func() {
			// 5s sleep > 50ms timeout → command is killed
			h := &capturingHandler{}
			prev := slog.Default()
			slog.SetDefault(slog.New(h))
			defer slog.SetDefault(prev)

			r := subproc.NewRunnerWithThresholds(10*time.Millisecond, 50*time.Millisecond)
			_, err := r.RunWithWarnAndTimeout(ctx, "sh-hang", "sh", "-c", "sleep 5")
			Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
			Expect(h.messages()).To(ContainElement("subprocess skipped"))
		})

		It("parent context cancellation propagates", func() {
			// Context cancelled after 100ms; runner has 5s timeout — ctx cancel wins.
			// Use sleep directly (not sh -c) to avoid shell forking keeping pipe open.
			cancelCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			defer cancel()

			r := subproc.NewRunnerWithThresholds(2*time.Second, 5*time.Second)
			start := time.Now()
			_, err := r.RunWithWarnAndTimeout(cancelCtx, "sleep-infinite", "sleep", "30")
			elapsed := time.Since(start)
			Expect(err).To(HaveOccurred())
			// Should return well within 5s timeout (cancelled at ~100ms)
			Expect(elapsed).To(BeNumerically("<", 2*time.Second))
		})

		It("no goroutine leak after many fast commands", func() {
			// Snapshot goroutines before test to exclude ginkgo/glog infrastructure goroutines.
			ignoreOpts := goleak.IgnoreCurrent()
			r := subproc.NewRunner()
			for i := 0; i < 100; i++ {
				_, err := r.RunWithWarnAndTimeout(ctx, "true-loop", "true")
				Expect(err).NotTo(HaveOccurred())
			}
			goleak.VerifyNone(GinkgoT(), ignoreOpts)
		})
	})
})
