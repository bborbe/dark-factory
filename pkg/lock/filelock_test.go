// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lock_test

import (
	"context"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/lock"
)

var _ = Describe("FileLock", func() {
	var (
		tempDir  string
		lockPath string
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		lockPath = filepath.Join(tempDir, "test.lock")
	})

	Describe("Acquire", func() {
		It("acquires lock on a new file", func() {
			fl := lock.NewFileLock(lockPath)
			ctx := context.Background()
			err := fl.Acquire(ctx, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
			fl.Release(ctx)
		})

		It("fails to acquire when already locked", func() {
			fl1 := lock.NewFileLock(lockPath)
			fl2 := lock.NewFileLock(lockPath)
			ctx := context.Background()

			err := fl1.Acquire(ctx, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
			defer fl1.Release(ctx)

			// Second lock should fail (non-blocking)
			err = fl2.Acquire(ctx, 100*time.Millisecond)
			Expect(err).To(HaveOccurred())
		})

		It("respects context cancellation", func() {
			fl := lock.NewFileLock(lockPath)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			err := fl.Acquire(ctx, 5*time.Second)
			Expect(err).To(HaveOccurred())
			// Should get context.DeadlineExceeded or similar
		})

		It("fails when timeout is exceeded while waiting", func() {
			fl1 := lock.NewFileLock(lockPath)
			fl2 := lock.NewFileLock(lockPath)
			ctx := context.Background()

			err := fl1.Acquire(ctx, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())
			defer fl1.Release(ctx)

			// Use a short timeout
			start := time.Now()
			err = fl2.Acquire(ctx, 200*time.Millisecond)
			Expect(err).To(HaveOccurred())
			Expect(time.Since(start)).To(BeNumerically(">=", 180*time.Millisecond))
		})
	})

	Describe("Release", func() {
		It("releases lock allowing another to acquire", func() {
			fl1 := lock.NewFileLock(lockPath)
			fl2 := lock.NewFileLock(lockPath)
			ctx := context.Background()

			err := fl1.Acquire(ctx, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())

			err = fl1.Release(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Now fl2 should be able to acquire
			err = fl2.Acquire(ctx, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
			fl2.Release(ctx)
		})

		It("succeeds when not holding the lock", func() {
			fl := lock.NewFileLock(lockPath)
			ctx := context.Background()
			// Release without acquiring should not error
			err := fl.Release(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("concurrent access", func() {
		It("only one acquirer succeeds at a time", func() {
			ctx := context.Background()
			results := make(chan bool, 10)

			for i := 0; i < 10; i++ {
				go func() {
					fl := lock.NewFileLock(lockPath)
					err := fl.Acquire(ctx, 2*time.Second)
					if err == nil {
						time.Sleep(50 * time.Millisecond)
						fl.Release(ctx)
						results <- true
					} else {
						results <- false
					}
				}()
			}

			successCount := 0
			for i := 0; i < 10; i++ {
				if <-results {
					successCount++
				}
			}
			// All should eventually succeed as locks are released
			Expect(successCount).To(Equal(10))
		})
	})
})
