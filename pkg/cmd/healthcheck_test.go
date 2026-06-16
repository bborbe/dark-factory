// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/bborbe/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cmd"
)

// captureHandler captures slog records for test assertions. Mirrors the
// helper used in pkg/specwatcher/watcher_test.go.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r)
	h.mu.Unlock()
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

func (h *captureHandler) WithGroup(_ string) slog.Handler { return h }

// recordsText returns a textual rendering of every captured record in
// capture order. Records are joined with newlines so substring searches
// can match the full line including key=value attributes.
func (h *captureHandler) recordsText() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var buf bytes.Buffer
	for _, r := range h.records {
		buf.WriteString(r.Message)
		buf.WriteByte(' ')
		r.Attrs(func(a slog.Attr) bool {
			buf.WriteString(a.Key)
			buf.WriteByte('=')
			buf.WriteString(a.Value.String())
			buf.WriteByte(' ')
			return true
		})
		buf.WriteByte('\n')
	}
	return buf.String()
}

var _ = Describe("HealthcheckCommand", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// buildProbes returns a Probes slice of N mocks (in order) and the parallel
	// slice of *mocks.HealthcheckProbe for direct access in assertions. The
	// mocks have Name returns set to the supplied names and RunReturns(nil).
	buildProbes := func(names ...string) (cmd.Probes, []*mocks.HealthcheckProbe) {
		ps := make([]*mocks.HealthcheckProbe, 0, len(names))
		out := make(cmd.Probes, 0, len(names))
		for _, n := range names {
			p := &mocks.HealthcheckProbe{}
			p.NameReturns(n)
			p.RunReturns(nil)
			ps = append(ps, p)
			out = append(out, p)
		}
		return out, ps
	}

	Describe("Run", func() {
		It("returns error for unknown flag", func() {
			hc, _ := buildProbes("docker", "image", "boot", "mount")
			err := cmd.NewHealthcheckCommand(hc).Run(ctx, []string{"--unknown"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown flag"))
		})

		It("returns nil when all four probes pass", func() {
			hc, _ := buildProbes("docker", "image", "boot", "mount")
			Expect(cmd.NewHealthcheckCommand(hc).Run(ctx, []string{})).To(Succeed())
		})

		It("exits non-zero on first probe failure", func() {
			hc, ps := buildProbes("docker", "image", "boot", "mount")
			ps[0].RunReturns(errors.Errorf(ctx, "boom"))
			err := cmd.NewHealthcheckCommand(hc).Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("docker"))
			// Subsequent probes must NOT be called when an earlier one fails.
			Expect(ps[1].RunCallCount()).To(Equal(0))
			Expect(ps[2].RunCallCount()).To(Equal(0))
			Expect(ps[3].RunCallCount()).To(Equal(0))
		})

		It("stops at first failure and reports category in stdout", func() {
			hc, ps := buildProbes("docker", "image", "boot", "mount")
			ps[1].RunReturns(
				errors.Errorf(
					ctx,
					"container image \"claude-yolo:does-not-exist\" not present locally",
				),
			)

			// Redirect stdout.
			orig := os.Stdout
			r, w, pipeErr := os.Pipe()
			Expect(pipeErr).NotTo(HaveOccurred())
			os.Stdout = w
			defer func() { os.Stdout = orig }()

			_ = cmd.NewHealthcheckCommand(hc).Run(ctx, []string{})

			Expect(w.Close()).To(Succeed())
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			out := buf.String()
			Expect(out).To(ContainSubstring("image\n"))
			Expect(out).To(ContainSubstring("does-not-exist"))
		})

		It("rejects --no-claude as an unknown flag (claude probe is non-optional)", func() {
			hc, _ := buildProbes("docker", "image", "boot", "mount")
			err := cmd.NewHealthcheckCommand(hc).Run(ctx, []string{"--no-claude"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown flag"))
		})

		It("returns nil with no args and no probes", func() {
			Expect(cmd.NewHealthcheckCommand(cmd.Probes{}).Run(ctx, []string{})).To(Succeed())
		})
	})

	Describe("HealthcheckHelp", func() {
		It("prints help to stdout without panicking", func() {
			orig := os.Stdout
			r, w, pipeErr := os.Pipe()
			Expect(pipeErr).NotTo(HaveOccurred())
			os.Stdout = w
			defer func() { os.Stdout = orig }()

			cmd.HealthcheckHelp()

			Expect(w.Close()).To(Succeed())
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			out := buf.String()
			Expect(out).To(ContainSubstring("Usage: dark-factory healthcheck"))
			Expect(out).To(ContainSubstring("--help"))
		})
	})

	// sevenProbes is the canonical full-sequence list. Tests use it to
	// assert the seven-probe orchestration.
	sevenNames := []string{
		"docker", "image", "boot", "claude", "mount", "gh", "notifications",
	}

	Describe("7-probe orchestration", func() {
		It("executes all seven probes in fixed order when all are configured and enabled", func() {
			handler := &captureHandler{}
			origLogger := slog.Default()
			slog.SetDefault(slog.New(handler))
			defer slog.SetDefault(origLogger)

			hc, _ := buildProbes(sevenNames...)
			err := cmd.NewHealthcheckCommand(hc).Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())

			text := handler.recordsText()
			idx := 0
			for _, name := range sevenNames {
				at := bytes.Index([]byte(text[idx:]), []byte("probe="+name))
				Expect(at).To(
					BeNumerically(">=", 0),
					"expected probe=%s in captured slog output: %s",
					name,
					text,
				)
				idx += at + 1
			}
		})

		It("skips gh when pr is false (factory pre-trims slice)", func() {
			handler := &captureHandler{}
			origLogger := slog.Default()
			slog.SetDefault(slog.New(handler))
			defer slog.SetDefault(origLogger)

			hc, _ := buildProbes("docker", "image", "boot", "claude", "mount", "notifications")
			err := cmd.NewHealthcheckCommand(hc).Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())

			text := handler.recordsText()
			Expect(text).NotTo(ContainSubstring("probe=gh"))
		})

		It("skips notifications when no channel configured (factory pre-trims slice)", func() {
			handler := &captureHandler{}
			origLogger := slog.Default()
			slog.SetDefault(slog.New(handler))
			defer slog.SetDefault(origLogger)

			hc, _ := buildProbes("docker", "image", "boot", "claude", "mount", "gh")
			err := cmd.NewHealthcheckCommand(hc).Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())

			text := handler.recordsText()
			Expect(text).NotTo(ContainSubstring("probe=notifications"))
		})
	})
})
