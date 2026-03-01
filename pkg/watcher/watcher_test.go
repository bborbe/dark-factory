// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package watcher_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/watcher"
)

var _ = Describe("Watcher", func() {
	var (
		tempDir    string
		promptsDir string
		ready      chan struct{}
		ctx        context.Context
		cancel     context.CancelFunc
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "watcher-test-*")
		Expect(err).NotTo(HaveOccurred())

		promptsDir = filepath.Join(tempDir, "prompts")
		err = os.MkdirAll(promptsDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		ready = make(chan struct{}, 10)
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It("should start and stop cleanly", func() {
		promptManager := &mocks.Manager{}
		promptManager.NormalizeFilenamesReturns([]prompt.Rename{}, nil)

		w := watcher.NewWatcher(promptsDir, promptsDir, promptManager, ready, 500*time.Millisecond)

		// Run watcher in goroutine
		errCh := make(chan error, 1)
		go func() {
			errCh <- w.Watch(ctx)
		}()

		// Let it run briefly
		time.Sleep(200 * time.Millisecond)

		// Cancel and verify clean shutdown
		cancel()

		select {
		case err := <-errCh:
			Expect(err).To(BeNil())
		case <-time.After(2 * time.Second):
			Fail("watcher did not stop within timeout")
		}
	})

	It("should normalize filenames when a file is created", func() {
		promptManager := &mocks.Manager{}
		promptManager.NormalizeFilenamesReturns([]prompt.Rename{
			{
				OldPath: filepath.Join(promptsDir, "test.md"),
				NewPath: filepath.Join(promptsDir, "001-test.md"),
			},
		}, nil)

		w := watcher.NewWatcher(promptsDir, promptsDir, promptManager, ready, 500*time.Millisecond)

		// Run watcher in goroutine
		go func() {
			_ = w.Watch(ctx)
		}()

		// Wait for watcher to start
		time.Sleep(200 * time.Millisecond)

		// Create a file
		testFile := filepath.Join(promptsDir, "test.md")
		err := os.WriteFile(testFile, []byte("# Test"), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Wait for normalization to be called
		Eventually(func() int {
			return promptManager.NormalizeFilenamesCallCount()
		}, 2*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 1))

		// Verify ready signal was sent
		Eventually(func() int {
			return len(ready)
		}, 1*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()
	})

	It("should debounce rapid file writes", func() {
		promptManager := &mocks.Manager{}
		promptManager.NormalizeFilenamesReturns([]prompt.Rename{}, nil)

		w := watcher.NewWatcher(promptsDir, promptsDir, promptManager, ready, 500*time.Millisecond)

		// Run watcher in goroutine
		go func() {
			_ = w.Watch(ctx)
		}()

		// Wait for watcher to start
		time.Sleep(200 * time.Millisecond)

		// Write to file multiple times rapidly
		testFile := filepath.Join(promptsDir, "rapid.md")
		for i := 0; i < 5; i++ {
			err := os.WriteFile(testFile, []byte("# Test"), 0600)
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(50 * time.Millisecond)
		}

		// Wait for debounce period
		time.Sleep(800 * time.Millisecond)

		// Should have called normalize only once (debounced)
		Expect(promptManager.NormalizeFilenamesCallCount()).To(Equal(1))

		cancel()
	})

	It("should ignore non-markdown files", func() {
		promptManager := &mocks.Manager{}
		promptManager.NormalizeFilenamesReturns([]prompt.Rename{}, nil)

		w := watcher.NewWatcher(promptsDir, promptsDir, promptManager, ready, 500*time.Millisecond)

		// Run watcher in goroutine
		go func() {
			_ = w.Watch(ctx)
		}()

		// Wait for watcher to start
		time.Sleep(200 * time.Millisecond)

		// Create a non-markdown file
		testFile := filepath.Join(promptsDir, "readme.txt")
		err := os.WriteFile(testFile, []byte("Hello"), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Wait and verify normalize was NOT called
		Consistently(func() int {
			return promptManager.NormalizeFilenamesCallCount()
		}, 1*time.Second, 100*time.Millisecond).Should(Equal(0))

		cancel()
	})

	It("should handle normalization errors gracefully", func() {
		promptManager := &mocks.Manager{}
		promptManager.NormalizeFilenamesReturns(nil, os.ErrPermission)

		w := watcher.NewWatcher(promptsDir, promptsDir, promptManager, ready, 500*time.Millisecond)

		// Run watcher in goroutine
		errCh := make(chan error, 1)
		go func() {
			errCh <- w.Watch(ctx)
		}()

		// Wait for watcher to start
		time.Sleep(200 * time.Millisecond)

		// Create a file
		testFile := filepath.Join(promptsDir, "test.md")
		err := os.WriteFile(testFile, []byte("# Test"), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Wait for error to be logged (but watcher should not crash)
		time.Sleep(800 * time.Millisecond)

		// Watcher should still be running
		select {
		case <-errCh:
			Fail("watcher should not exit on normalization error")
		default:
			// Good, still running
		}

		cancel()

		// Should exit cleanly on context cancel
		select {
		case err := <-errCh:
			Expect(err).To(BeNil())
		case <-time.After(2 * time.Second):
			Fail("watcher did not exit on context cancel")
		}
	})

	It("should send ready signal after normalization", func() {
		promptManager := &mocks.Manager{}
		promptManager.NormalizeFilenamesReturns([]prompt.Rename{
			{
				OldPath: filepath.Join(promptsDir, "a.md"),
				NewPath: filepath.Join(promptsDir, "001-a.md"),
			},
		}, nil)

		w := watcher.NewWatcher(promptsDir, promptsDir, promptManager, ready, 500*time.Millisecond)

		// Run watcher in goroutine
		go func() {
			_ = w.Watch(ctx)
		}()

		// Wait for watcher to start
		time.Sleep(200 * time.Millisecond)

		// Create a file
		testFile := filepath.Join(promptsDir, "a.md")
		err := os.WriteFile(testFile, []byte("# Test"), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Wait for ready signal
		Eventually(func() int {
			return len(ready)
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		// Consume signal
		<-ready

		cancel()
	})
})
