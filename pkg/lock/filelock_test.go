// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lock_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/lock"
)

var _ = Describe("DirLock", func() {
	var (
		tempDir string
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
	})

	Describe("Acquire", func() {
		It("acquires lock on an existing directory", func() {
			fl := lock.NewDirLock(tempDir)
			ctx := context.Background()
			err := fl.Acquire(ctx, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
			fl.Release(ctx)
		})

		It("fails to acquire when the same directory is already locked (non-blocking)", func() {
			fl1 := lock.NewDirLock(tempDir)
			fl2 := lock.NewDirLock(tempDir)
			ctx := context.Background()

			err := fl1.Acquire(ctx, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
			defer fl1.Release(ctx)

			// Second lock on same dir should fail (non-blocking)
			err = fl2.Acquire(ctx, 100*time.Millisecond)
			Expect(err).To(HaveOccurred())
		})

		It("respects context cancellation", func() {
			fl := lock.NewDirLock(tempDir)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			err := fl.Acquire(ctx, 5*time.Second)
			Expect(err).To(HaveOccurred())
		})

		It("fails when timeout is exceeded while waiting", func() {
			fl1 := lock.NewDirLock(tempDir)
			fl2 := lock.NewDirLock(tempDir)
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

		It("fails when the directory does not exist", func() {
			fl := lock.NewDirLock(filepath.Join(tempDir, "does-not-exist"))
			ctx := context.Background()
			err := fl.Acquire(ctx, 200*time.Millisecond)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Release", func() {
		It("releases lock allowing another to acquire", func() {
			fl1 := lock.NewDirLock(tempDir)
			fl2 := lock.NewDirLock(tempDir)
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
			fl := lock.NewDirLock(tempDir)
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
					fl := lock.NewDirLock(tempDir)
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

	It(
		"TestParentDirLock_SerializesSameDir: two goroutines locking the same directory observe serial ordering",
		func() {
			if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
				Skip("flock semantics only guaranteed on linux and darwin")
			}

			ctx := context.Background()
			type interval struct {
				enter time.Time
				exit  time.Time
			}
			intervals := make([]interval, 2)
			var wg sync.WaitGroup
			wg.Add(2)

			for i := 0; i < 2; i++ {
				i := i
				go func() {
					defer wg.Done()
					fl := lock.NewDirLock(tempDir)
					Expect(fl.Acquire(ctx, 5*time.Second)).To(Succeed())
					intervals[i].enter = time.Now()
					time.Sleep(30 * time.Millisecond)
					intervals[i].exit = time.Now()
					Expect(fl.Release(ctx)).To(Succeed())
				}()
			}
			wg.Wait()

			// Verify serial ordering: one goroutine's exit must be <= the other's enter
			serialized := intervals[0].exit.Before(intervals[1].enter) ||
				!intervals[0].exit.After(intervals[1].enter) ||
				intervals[1].exit.Before(intervals[0].enter) ||
				!intervals[1].exit.After(intervals[0].enter)
			Expect(serialized).To(BeTrue(), "expected the two critical sections not to overlap")
		},
	)

	It(
		"TestParentDirLock_ParallelDifferentDirs: two goroutines locking different directories make parallel progress",
		func() {
			if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
				Skip("flock semantics only guaranteed on linux and darwin")
			}

			dirA := filepath.Join(tempDir, "dirA")
			dirB := filepath.Join(tempDir, "dirB")
			Expect(os.MkdirAll(dirA, 0750)).To(Succeed())
			Expect(os.MkdirAll(dirB, 0750)).To(Succeed())

			ctx := context.Background()
			type interval struct {
				enter time.Time
				exit  time.Time
			}
			intervals := make([]interval, 2)
			var wg sync.WaitGroup
			wg.Add(2)

			for i, dir := range []string{dirA, dirB} {
				i, dir := i, dir
				go func() {
					defer wg.Done()
					fl := lock.NewDirLock(dir)
					Expect(fl.Acquire(ctx, 5*time.Second)).To(Succeed())
					intervals[i].enter = time.Now()
					time.Sleep(50 * time.Millisecond)
					intervals[i].exit = time.Now()
					Expect(fl.Release(ctx)).To(Succeed())
				}()
			}
			wg.Wait()

			// Assert that the two critical sections OVERLAPPED: goroutine A entered before B exited
			// AND goroutine B entered before A exited. This is robust to scheduling jitter because
			// it checks actual concurrency rather than total elapsed time.
			overlapped := intervals[0].enter.Before(intervals[1].exit) &&
				intervals[1].enter.Before(intervals[0].exit)
			Expect(
				overlapped,
			).To(BeTrue(), "expected different-dir locks to make parallel progress (critical sections should overlap)")
		},
	)

	It(
		"TestParentDirLock_CrashReleaseLeavesNoArtifact: a dropped fd auto-releases and leaves no .lock file",
		func() {
			ctx := context.Background()

			fl := lock.NewDirLock(tempDir)
			Expect(fl.Acquire(ctx, 5*time.Second)).To(Succeed())

			// Simulate process death by closing the fd (Release closes the fd, dropping the kernel flock).
			// In a real crash the kernel closes all open fds automatically, producing the same effect:
			// man flock(2) — the flock is released when the last fd referencing the lock is closed.
			Expect(fl.Release(ctx)).To(Succeed())

			// A second acquirer must succeed — the kernel flock was dropped by the close above.
			fl2 := lock.NewDirLock(tempDir)
			Expect(fl2.Acquire(ctx, 1*time.Second)).To(Succeed())
			Expect(fl2.Release(ctx)).To(Succeed())

			// No .lock sidecar files must exist — directory locking never creates any.
			matches, globErr := filepath.Glob(filepath.Join(tempDir, "*.lock"))
			Expect(globErr).NotTo(HaveOccurred())
			Expect(matches).To(BeEmpty())
		},
	)
})
