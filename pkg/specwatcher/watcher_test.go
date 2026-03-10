// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package specwatcher_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/specwatcher"
)

// captureHandler captures slog records for test assertions.
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

func (h *captureHandler) Messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	msgs := make([]string, len(h.records))
	for i, r := range h.records {
		msgs[i] = r.Message
	}
	return msgs
}

var _ = Describe("SpecWatcher", func() {
	var (
		tempDir       string
		inProgressDir string
		ctx           context.Context
		cancel        context.CancelFunc
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "specwatcher-test-*")
		Expect(err).NotTo(HaveOccurred())

		inProgressDir = filepath.Join(tempDir, "specs", "in-progress")
		err = os.MkdirAll(inProgressDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It(
		"should call generator for spec present in inProgressDir on startup",
		func() {
			gen := &mocks.SpecGenerator{}
			gen.GenerateReturns(nil)

			// Create spec BEFORE starting the watcher.
			specFile := filepath.Join(inProgressDir, "pre-existing-spec.md")
			content := "---\nstatus: approved\n---\n# Pre-existing Spec\n"
			err := os.WriteFile(specFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			w := specwatcher.NewSpecWatcher(
				inProgressDir,
				gen,
				200*time.Millisecond,
				libtime.NewCurrentDateTime(),
			)

			go func() {
				_ = w.Watch(ctx)
			}()

			Eventually(func() int {
				return gen.GenerateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			_, passedPath := gen.GenerateArgsForCall(0)
			Expect(passedPath).To(Equal(specFile))

			cancel()
		},
	)

	It("should ignore non-markdown files in inProgressDir on startup", func() {
		gen := &mocks.SpecGenerator{}

		txtFile := filepath.Join(inProgressDir, "readme.txt")
		err := os.WriteFile(txtFile, []byte("hello"), 0600)
		Expect(err).NotTo(HaveOccurred())

		w := specwatcher.NewSpecWatcher(
			inProgressDir,
			gen,
			200*time.Millisecond,
			libtime.NewCurrentDateTime(),
		)

		go func() {
			_ = w.Watch(ctx)
		}()

		Consistently(func() int {
			return gen.GenerateCallCount()
		}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

		cancel()
	})

	It("should start and stop cleanly", func() {
		gen := &mocks.SpecGenerator{}

		w := specwatcher.NewSpecWatcher(
			inProgressDir,
			gen,
			500*time.Millisecond,
			libtime.NewCurrentDateTime(),
		)

		errCh := make(chan error, 1)
		go func() {
			errCh <- w.Watch(ctx)
		}()

		time.Sleep(200 * time.Millisecond)
		cancel()

		select {
		case err := <-errCh:
			Expect(err).To(BeNil())
		case <-time.After(2 * time.Second):
			Fail("watcher did not stop within timeout")
		}
	})

	It("should call generator when a new .md file is created in inProgressDir", func() {
		gen := &mocks.SpecGenerator{}
		gen.GenerateReturns(nil)

		w := specwatcher.NewSpecWatcher(
			inProgressDir,
			gen,
			200*time.Millisecond,
			libtime.NewCurrentDateTime(),
		)

		go func() {
			_ = w.Watch(ctx)
		}()

		time.Sleep(100 * time.Millisecond)

		specFile := filepath.Join(inProgressDir, "my-spec.md")
		content := "---\nstatus: approved\n---\n# My Spec\n"
		err := os.WriteFile(specFile, []byte(content), 0600)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() int {
			return gen.GenerateCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		_, passedPath := gen.GenerateArgsForCall(0)
		Expect(passedPath).To(Equal(specFile))

		cancel()
	})

	It("should NOT call generator on Write events (only Create triggers generation)", func() {
		gen := &mocks.SpecGenerator{}
		gen.GenerateReturns(nil)

		// Create the file before starting the watcher so startup scan fires once
		specFile := filepath.Join(inProgressDir, "write-test.md")
		content := "---\nstatus: approved\n---\n# Write Test\n"
		err := os.WriteFile(specFile, []byte(content), 0600)
		Expect(err).NotTo(HaveOccurred())

		w := specwatcher.NewSpecWatcher(
			inProgressDir,
			gen,
			200*time.Millisecond,
			libtime.NewCurrentDateTime(),
		)

		go func() {
			_ = w.Watch(ctx)
		}()

		// Wait for startup scan to complete
		Eventually(func() int {
			return gen.GenerateCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		callsBefore := gen.GenerateCallCount()

		// Write to the existing file (Write event, not Create)
		err = os.WriteFile(specFile, []byte(content+"updated\n"), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Generator should not be called again for a Write event
		Consistently(func() int {
			return gen.GenerateCallCount()
		}, 800*time.Millisecond, 50*time.Millisecond).Should(Equal(callsBefore))

		cancel()
	})

	It("should log error and continue when generator fails", func() {
		gen := &mocks.SpecGenerator{}
		gen.GenerateReturns(os.ErrPermission)

		w := specwatcher.NewSpecWatcher(
			inProgressDir,
			gen,
			200*time.Millisecond,
			libtime.NewCurrentDateTime(),
		)

		errCh := make(chan error, 1)
		go func() {
			errCh <- w.Watch(ctx)
		}()

		time.Sleep(100 * time.Millisecond)

		specFile := filepath.Join(inProgressDir, "failing-spec.md")
		content := "---\nstatus: approved\n---\n# Failing Spec\n"
		err := os.WriteFile(specFile, []byte(content), 0600)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() int {
			return gen.GenerateCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		// Watcher should still be running
		select {
		case <-errCh:
			Fail("watcher should not exit on generator error")
		default:
			// Good, still running
		}

		cancel()

		select {
		case err := <-errCh:
			Expect(err).To(BeNil())
		case <-time.After(2 * time.Second):
			Fail("watcher did not exit on context cancel")
		}
	})

	It("should ignore non-markdown files", func() {
		gen := &mocks.SpecGenerator{}

		w := specwatcher.NewSpecWatcher(
			inProgressDir,
			gen,
			200*time.Millisecond,
			libtime.NewCurrentDateTime(),
		)

		go func() {
			_ = w.Watch(ctx)
		}()

		time.Sleep(100 * time.Millisecond)

		testFile := filepath.Join(inProgressDir, "readme.txt")
		err := os.WriteFile(testFile, []byte("Hello"), 0600)
		Expect(err).NotTo(HaveOccurred())

		Consistently(func() int {
			return gen.GenerateCallCount()
		}, 800*time.Millisecond, 100*time.Millisecond).Should(Equal(0))

		cancel()
	})

	It("should log cancelled not failed when context is cancelled during generation", func() {
		handler := &captureHandler{}
		origLogger := slog.Default()
		slog.SetDefault(slog.New(handler))
		defer slog.SetDefault(origLogger)

		gen := &mocks.SpecGenerator{}
		gen.GenerateStub = func(ctx context.Context, _ string) error {
			<-ctx.Done()
			return context.Canceled
		}

		w := specwatcher.NewSpecWatcher(
			inProgressDir,
			gen,
			50*time.Millisecond,
			libtime.NewCurrentDateTime(),
		)

		go func() {
			_ = w.Watch(ctx)
		}()

		time.Sleep(100 * time.Millisecond)

		specFile := filepath.Join(inProgressDir, "cancel-spec.md")
		content := "---\nstatus: approved\n---\n# Cancel Spec\n"
		err := os.WriteFile(specFile, []byte(content), 0600)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() int {
			return gen.GenerateCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()

		// Allow the goroutine to finish after cancellation.
		time.Sleep(200 * time.Millisecond)

		msgs := handler.Messages()
		Expect(msgs).To(ContainElement("spec generation cancelled"))
		Expect(msgs).NotTo(ContainElement("spec generation failed"))
	})

	It("should NOT call generator for spec with status prompted in inProgressDir", func() {
		gen := &mocks.SpecGenerator{}

		specFile := filepath.Join(inProgressDir, "already-prompted-spec.md")
		content := "---\nstatus: prompted\n---\n# Already Prompted Spec\n"
		err := os.WriteFile(specFile, []byte(content), 0600)
		Expect(err).NotTo(HaveOccurred())

		w := specwatcher.NewSpecWatcher(
			inProgressDir,
			gen,
			200*time.Millisecond,
			libtime.NewCurrentDateTime(),
		)

		go func() {
			_ = w.Watch(ctx)
		}()

		Consistently(func() int {
			return gen.GenerateCallCount()
		}, 800*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

		cancel()
	})

	It("should work with relative paths", func() {
		origDir, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			_ = os.Chdir(origDir)
		}()

		err = os.Chdir(tempDir)
		Expect(err).NotTo(HaveOccurred())

		relInProgressDir := "specs-rel/in-progress"
		err = os.MkdirAll(relInProgressDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		gen := &mocks.SpecGenerator{}
		gen.GenerateReturns(nil)

		w := specwatcher.NewSpecWatcher(
			relInProgressDir,
			gen,
			200*time.Millisecond,
			libtime.NewCurrentDateTime(),
		)

		go func() {
			_ = w.Watch(ctx)
		}()

		time.Sleep(100 * time.Millisecond)

		absInProgressRelDir := filepath.Join(tempDir, relInProgressDir)
		specFile := filepath.Join(absInProgressRelDir, "rel-spec.md")
		content := "---\nstatus: approved\n---\n# Rel Spec\n"
		err = os.WriteFile(specFile, []byte(content), 0600)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() int {
			return gen.GenerateCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()
	})
})
