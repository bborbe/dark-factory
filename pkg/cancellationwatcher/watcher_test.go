// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cancellationwatcher_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cancellationwatcher"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("Watcher", func() {
	var (
		tempDir      string
		promptPath   string
		mockExecutor *mocks.Executor
		mockLoader   *mocks.ProcessorPromptManager
		w            cancellationwatcher.Watcher
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "cancellation-watcher-test-*")
		Expect(err).NotTo(HaveOccurred())

		promptPath = filepath.Join(tempDir, "080-test-prompt.md")
		err = os.WriteFile(promptPath, []byte("---\nstatus: executing\n---\n\n# Test\n"), 0600)
		Expect(err).NotTo(HaveOccurred())

		mockExecutor = &mocks.Executor{}
		mockLoader = &mocks.ProcessorPromptManager{}

		w = cancellationwatcher.NewWatcher(mockExecutor, mockLoader)
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It("returns a channel that never closes when no status change occurs", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := w.Watch(ctx, promptPath, "test-container")

		// Channel should not close within a short window
		Consistently(func() bool {
			select {
			case <-ch:
				return true
			default:
				return false
			}
		}, 200*time.Millisecond, 50*time.Millisecond).Should(BeFalse())

		Expect(mockExecutor.StopAndRemoveContainerCallCount()).To(Equal(0))
	})

	It("closes the channel and stops the container when status flips to cancelled", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		cancelledPF := prompt.NewPromptFile(
			promptPath,
			prompt.Frontmatter{Status: string(prompt.CancelledPromptStatus)},
			[]byte("# Test\n"),
			libtime.NewCurrentDateTime(),
		)
		mockLoader.LoadReturns(cancelledPF, nil)

		ch := w.Watch(ctx, promptPath, "test-container")

		// Give fsnotify time to set up the watch before writing
		time.Sleep(100 * time.Millisecond)
		err := os.WriteFile(promptPath, []byte("---\nstatus: cancelled\n---\n\n# Test\n"), 0600)
		Expect(err).NotTo(HaveOccurred())

		Eventually(ch, 2*time.Second).Should(BeClosed())
		Expect(mockExecutor.StopAndRemoveContainerCallCount()).To(Equal(1))
		_, gotContainerName := mockExecutor.StopAndRemoveContainerArgsForCall(0)
		Expect(gotContainerName).To(Equal("test-container"))
	})

	It("exits cleanly when ctx is cancelled mid-watch", func() {
		ctx, cancel := context.WithCancel(context.Background())

		ch := w.Watch(ctx, promptPath, "test-container")

		// Cancel context to trigger goroutine exit
		cancel()

		// Channel should not be closed (cancellation was by context, not by prompt status)
		// but the goroutine should not leak — we verify by waiting briefly
		time.Sleep(200 * time.Millisecond)
		Expect(mockExecutor.StopAndRemoveContainerCallCount()).To(Equal(0))
		// ch is not closed, so a select on it should not fire
		select {
		case <-ch:
			Fail("channel should not be closed on ctx cancellation")
		default:
		}
	})

	It("exits cleanly when fsnotify cannot be created (logs warning and returns)", func() {
		// We can't directly inject a failing fsnotify, but we can verify that
		// watching a non-existent path logs and returns without closing the channel.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		nonExistentPath := filepath.Join(tempDir, "nonexistent-prompt.md")
		ch := w.Watch(ctx, nonExistentPath, "test-container")

		// Give the goroutine time to attempt and fail to add the watch
		time.Sleep(200 * time.Millisecond)

		// Channel should not be closed — watcher returned without detecting cancellation
		select {
		case <-ch:
			Fail("channel should not be closed when watch setup fails")
		default:
		}
		Expect(mockExecutor.StopAndRemoveContainerCallCount()).To(Equal(0))
	})

	It("does not close the channel when prompt load fails after a write event", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mockLoader.LoadReturns(nil, fmt.Errorf("load error"))

		ch := w.Watch(ctx, promptPath, "test-container")

		time.Sleep(100 * time.Millisecond)
		err := os.WriteFile(promptPath, []byte("---\nstatus: executing\n---\n\n# Test\n"), 0600)
		Expect(err).NotTo(HaveOccurred())

		time.Sleep(200 * time.Millisecond)
		select {
		case <-ch:
			Fail("channel should not be closed when load fails")
		default:
		}
		Expect(mockExecutor.StopAndRemoveContainerCallCount()).To(Equal(0))
	})

	It("does not close the channel when write event status is not cancelled", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		executingPF := prompt.NewPromptFile(
			promptPath,
			prompt.Frontmatter{Status: string(prompt.ExecutingPromptStatus)},
			[]byte("# Test\n"),
			libtime.NewCurrentDateTime(),
		)
		mockLoader.LoadReturns(executingPF, nil)

		ch := w.Watch(ctx, promptPath, "test-container")

		time.Sleep(100 * time.Millisecond)
		err := os.WriteFile(promptPath, []byte("---\nstatus: executing\n---\n\n# Test\n"), 0600)
		Expect(err).NotTo(HaveOccurred())

		time.Sleep(200 * time.Millisecond)
		select {
		case <-ch:
			Fail("channel should not be closed when status is not cancelled")
		default:
		}
		Expect(mockExecutor.StopAndRemoveContainerCallCount()).To(Equal(0))
	})

	It("ignores non-Write events such as chmod and does not close the channel", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := w.Watch(ctx, promptPath, "test-container")

		time.Sleep(100 * time.Millisecond)
		// chmod triggers an ATTRIB/CHMOD event, not a WRITE event — should be ignored
		err := os.Chmod(promptPath, 0644)
		Expect(err).NotTo(HaveOccurred())

		time.Sleep(200 * time.Millisecond)
		select {
		case <-ch:
			Fail("channel should not be closed on non-Write events")
		default:
		}
		Expect(mockExecutor.StopAndRemoveContainerCallCount()).To(Equal(0))
		// loader should not have been called since no Write event occurred
		Expect(mockLoader.LoadCallCount()).To(Equal(0))
	})
})
