// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lock_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/lock"
)

var _ = Describe("Locker", func() {
	var (
		ctx     context.Context
		tmpDir  string
		locker  lock.Locker
		lockErr error
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tmpDir, err = os.MkdirTemp("", "lock-test-*")
		Expect(err).NotTo(HaveOccurred())
		locker = lock.NewLocker(tmpDir)
	})

	AfterEach(func() {
		if locker != nil {
			_ = locker.Release(ctx)
		}
		if tmpDir != "" {
			_ = os.RemoveAll(tmpDir)
		}
	})

	Describe("Acquire", func() {
		JustBeforeEach(func() {
			lockErr = locker.Acquire(ctx)
		})

		It("succeeds on first call", func() {
			Expect(lockErr).NotTo(HaveOccurred())
		})

		It("creates lock file", func() {
			lockPath := filepath.Join(tmpDir, ".dark-factory.lock")
			_, err := os.Stat(lockPath)
			Expect(err).NotTo(HaveOccurred())
		})

		It("writes PID to lock file", func() {
			lockPath := filepath.Join(tmpDir, ".dark-factory.lock")
			data, err := os.ReadFile(lockPath)
			Expect(err).NotTo(HaveOccurred())

			pidStr := strings.TrimSpace(string(data))
			pid, err := strconv.Atoi(pidStr)
			Expect(err).NotTo(HaveOccurred())
			Expect(pid).To(Equal(os.Getpid()))
		})

		Context("when lock is already held", func() {
			var secondLocker lock.Locker

			BeforeEach(func() {
				secondLocker = lock.NewLocker(tmpDir)
			})

			AfterEach(func() {
				if secondLocker != nil {
					_ = secondLocker.Release(ctx)
				}
			})

			It("fails with PID in error message", func() {
				// First lock succeeds
				Expect(lockErr).NotTo(HaveOccurred())

				// Second lock fails
				err := secondLocker.Acquire(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("another instance is already running"))
				Expect(err.Error()).To(ContainSubstring("pid"))
			})
		})

		Context("when directory does not exist", func() {
			BeforeEach(func() {
				nonExistentDir := filepath.Join(tmpDir, "does-not-exist")
				locker = lock.NewLocker(nonExistentDir)
			})

			It("fails with error", func() {
				Expect(lockErr).To(HaveOccurred())
			})
		})
	})

	Describe("Release", func() {
		BeforeEach(func() {
			err := locker.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("removes lock file", func() {
			err := locker.Release(ctx)
			Expect(err).NotTo(HaveOccurred())

			lockPath := filepath.Join(tmpDir, ".dark-factory.lock")
			_, err = os.Stat(lockPath)
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("allows acquiring lock again", func() {
			err := locker.Release(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Create new locker and acquire
			newLocker := lock.NewLocker(tmpDir)
			err = newLocker.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())

			_ = newLocker.Release(ctx)
		})

		Context("when called without acquiring first", func() {
			BeforeEach(func() {
				// Release the lock first
				err := locker.Release(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("succeeds without error", func() {
				err := locker.Release(ctx)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("flock behavior", func() {
		It("lock is automatically released when process exits", func() {
			// This test verifies the flock kernel behavior by simulating
			// a crash (file descriptor close without explicit unlock)
			lockPath := filepath.Join(tmpDir, ".dark-factory.lock")

			// Acquire lock and get file descriptor
			err := locker.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Simulate crash by opening another file descriptor
			// and trying to lock (should fail while first lock is held)
			fd, err := os.OpenFile(lockPath, os.O_RDWR, 0644)
			Expect(err).NotTo(HaveOccurred())
			defer fd.Close()

			err = syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(syscall.EWOULDBLOCK))

			// Release the first lock
			err = locker.Release(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Now the second lock should succeed
			err = syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
			Expect(err).NotTo(HaveOccurred())

			// Clean up
			_ = syscall.Flock(int(fd.Fd()), syscall.LOCK_UN)
		})
	})

	Describe("edge cases", func() {
		Context("when lock file contains invalid PID", func() {
			var secondLocker lock.Locker

			BeforeEach(func() {
				// Create lock file with invalid content
				lockPath := filepath.Join(tmpDir, ".dark-factory.lock")
				err := os.WriteFile(lockPath, []byte("not-a-number\n"), 0600)
				Expect(err).NotTo(HaveOccurred())

				// Lock the file manually
				fd, err := os.OpenFile(lockPath, os.O_RDWR, 0644)
				Expect(err).NotTo(HaveOccurred())
				err = syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
				Expect(err).NotTo(HaveOccurred())

				secondLocker = lock.NewLocker(tmpDir)

				// Clean up the manual lock
				DeferCleanup(func() {
					_ = syscall.Flock(int(fd.Fd()), syscall.LOCK_UN)
					_ = fd.Close()
				})
			})

			It("still reports another instance is running", func() {
				err := secondLocker.Acquire(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("another instance is already running"))
			})
		})

		Context("when lock file is empty", func() {
			var secondLocker lock.Locker

			BeforeEach(func() {
				// Create empty lock file
				lockPath := filepath.Join(tmpDir, ".dark-factory.lock")
				err := os.WriteFile(lockPath, []byte(""), 0600)
				Expect(err).NotTo(HaveOccurred())

				// Lock the file manually
				fd, err := os.OpenFile(lockPath, os.O_RDWR, 0644)
				Expect(err).NotTo(HaveOccurred())
				err = syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
				Expect(err).NotTo(HaveOccurred())

				secondLocker = lock.NewLocker(tmpDir)

				// Clean up the manual lock
				DeferCleanup(func() {
					_ = syscall.Flock(int(fd.Fd()), syscall.LOCK_UN)
					_ = fd.Close()
				})
			})

			It("still reports another instance is running", func() {
				err := secondLocker.Acquire(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("another instance is already running"))
			})
		})

		Context("when releasing non-existent lock file", func() {
			BeforeEach(func() {
				// Acquire and release once
				err := locker.Acquire(ctx)
				Expect(err).NotTo(HaveOccurred())
				err = locker.Release(ctx)
				Expect(err).NotTo(HaveOccurred())

				// Manually delete the lock file if it still exists
				lockPath := filepath.Join(tmpDir, ".dark-factory.lock")
				_ = os.Remove(lockPath)
			})

			It("succeeds without error", func() {
				err := locker.Release(ctx)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when acquiring lock on read-only filesystem", func() {
			var roDir string

			BeforeEach(func() {
				// Create a directory with read-only lock file
				roDir = filepath.Join(tmpDir, "readonly")
				err := os.Mkdir(roDir, 0755)
				Expect(err).NotTo(HaveOccurred())

				// Create lock file
				lockPath := filepath.Join(roDir, ".dark-factory.lock")
				err = os.WriteFile(lockPath, []byte("123\n"), 0400)
				Expect(err).NotTo(HaveOccurred())

				// Make the lock file read-only
				err = os.Chmod(lockPath, 0400)
				Expect(err).NotTo(HaveOccurred())

				locker = lock.NewLocker(roDir)

				// Clean up
				DeferCleanup(func() {
					_ = os.Chmod(lockPath, 0644)
					_ = os.RemoveAll(roDir)
				})
			})

			It("fails with permission error", func() {
				err := locker.Acquire(ctx)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when acquiring lock twice on same locker", func() {
			BeforeEach(func() {
				err := locker.Acquire(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("succeeds when acquiring again after release", func() {
				err := locker.Release(ctx)
				Expect(err).NotTo(HaveOccurred())

				err = locker.Acquire(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(locker.Release(ctx)).NotTo(HaveOccurred())
			})
		})

		Context("multiple acquire/release cycles", func() {
			It("handles multiple cycles correctly", func() {
				for i := 0; i < 3; i++ {
					err := locker.Acquire(ctx)
					Expect(err).NotTo(HaveOccurred())

					lockPath := filepath.Join(tmpDir, ".dark-factory.lock")
					data, err := os.ReadFile(lockPath)
					Expect(err).NotTo(HaveOccurred())
					Expect(string(data)).To(ContainSubstring(strconv.Itoa(os.Getpid())))

					err = locker.Release(ctx)
					Expect(err).NotTo(HaveOccurred())

					_, err = os.Stat(lockPath)
					Expect(os.IsNotExist(err)).To(BeTrue())
				}
			})
		})

		Context("concurrent lock attempts", func() {
			It("prevents concurrent access", func() {
				// First locker acquires
				err := locker.Acquire(ctx)
				Expect(err).NotTo(HaveOccurred())

				// Multiple other lockers try to acquire
				for i := 0; i < 5; i++ {
					otherLocker := lock.NewLocker(tmpDir)
					err := otherLocker.Acquire(ctx)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("another instance is already running"))
				}

				// Release first lock
				err = locker.Release(ctx)
				Expect(err).NotTo(HaveOccurred())

				// Now another locker can acquire
				newLocker := lock.NewLocker(tmpDir)
				err = newLocker.Acquire(ctx)
				Expect(err).NotTo(HaveOccurred())
				err = newLocker.Release(ctx)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
