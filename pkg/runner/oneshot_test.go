// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/runner"
)

var _ = Describe("OneShotRunner", func() {
	var (
		tempDir       string
		promptsDir    string
		specsDir      string
		mockManager   *mocks.Manager
		mockLocker    *mocks.Locker
		mockProcessor *mocks.Processor
		ctx           context.Context
		cancel        context.CancelFunc
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "oneshot-runner-test-*")
		Expect(err).NotTo(HaveOccurred())

		promptsDir = filepath.Join(tempDir, "prompts")
		err = os.MkdirAll(promptsDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		specsDir = filepath.Join(tempDir, "specs")

		mockManager = &mocks.Manager{}
		mockLocker = &mocks.Locker{}
		mockProcessor = &mocks.Processor{}

		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	newTestOneShotRunner := func(inboxDir, inProgressDir, completedDir string) runner.OneShotRunner {
		return runner.NewOneShotRunner(
			inboxDir,
			inProgressDir,
			completedDir,
			filepath.Join(promptsDir, "logs"),
			filepath.Join(specsDir, "inbox"),
			filepath.Join(specsDir, "in-progress"),
			filepath.Join(specsDir, "completed"),
			filepath.Join(specsDir, "logs"),
			mockManager,
			mockLocker,
			mockProcessor,
		)
	}

	It("should acquire and release lock", func() {
		mockLocker.AcquireReturns(nil)
		mockLocker.ReleaseReturns(nil)
		mockManager.ResetExecutingReturns(nil)
		mockManager.NormalizeFilenamesReturns(nil, nil)
		mockProcessor.ProcessQueueReturns(nil)

		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())

		Expect(mockLocker.AcquireCallCount()).To(Equal(1))
		Expect(mockLocker.ReleaseCallCount()).To(Equal(1))
	})

	It("should call ProcessQueue and return without blocking", func() {
		mockLocker.AcquireReturns(nil)
		mockLocker.ReleaseReturns(nil)
		mockManager.ResetExecutingReturns(nil)
		mockManager.NormalizeFilenamesReturns(nil, nil)
		mockProcessor.ProcessQueueReturns(nil)

		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())

		Expect(mockProcessor.ProcessQueueCallCount()).To(Equal(1))
	})

	It("should reset executing prompts on startup", func() {
		mockLocker.AcquireReturns(nil)
		mockLocker.ReleaseReturns(nil)
		mockManager.ResetExecutingReturns(nil)
		mockManager.NormalizeFilenamesReturns(nil, nil)
		mockProcessor.ProcessQueueReturns(nil)

		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())

		Expect(mockManager.ResetExecutingCallCount()).To(Equal(1))
	})

	It("should return nil when queue is empty", func() {
		mockLocker.AcquireReturns(nil)
		mockLocker.ReleaseReturns(nil)
		mockManager.ResetExecutingReturns(nil)
		mockManager.NormalizeFilenamesReturns(nil, nil)
		mockProcessor.ProcessQueueReturns(nil)

		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())
	})

	It("should not call Process (only ProcessQueue)", func() {
		mockLocker.AcquireReturns(nil)
		mockLocker.ReleaseReturns(nil)
		mockManager.ResetExecutingReturns(nil)
		mockManager.NormalizeFilenamesReturns(nil, nil)
		mockProcessor.ProcessQueueReturns(nil)

		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())

		Expect(mockProcessor.ProcessCallCount()).To(Equal(0))
		Expect(mockProcessor.ProcessQueueCallCount()).To(Equal(1))
	})
})
