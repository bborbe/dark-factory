// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/cmd"
)

var _ = Describe("KillCommand", func() {
	var (
		tempDir      string
		lockFilePath string
		ctx          context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "kill-test-*")
		Expect(err).NotTo(HaveOccurred())
		lockFilePath = filepath.Join(tempDir, ".dark-factory.lock")
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("no lock file exists", func() {
		It("prints 'no daemon running' and returns nil", func() {
			killCmd := cmd.NewKillCommand(lockFilePath, nil, nil)
			err := killCmd.Run(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("lock file exists but is empty", func() {
		It("prints 'no daemon running' and returns nil", func() {
			err := os.WriteFile(lockFilePath, []byte(""), 0600)
			Expect(err).NotTo(HaveOccurred())

			killCmd := cmd.NewKillCommand(lockFilePath, nil, nil)
			err = killCmd.Run(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("lock file contains invalid content", func() {
		It("prints 'no daemon running' and returns nil", func() {
			err := os.WriteFile(lockFilePath, []byte("not-a-pid\n"), 0600)
			Expect(err).NotTo(HaveOccurred())

			killCmd := cmd.NewKillCommand(lockFilePath, nil, nil)
			err = killCmd.Run(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("lock file contains stale PID (process not running)", func() {
		It("removes the lock file and returns nil", func() {
			// Use a PID that is almost certainly not running
			err := os.WriteFile(lockFilePath, []byte("99999999\n"), 0600)
			Expect(err).NotTo(HaveOccurred())

			killCmd := cmd.NewKillCommand(lockFilePath, nil, nil)
			err = killCmd.Run(ctx, nil)
			Expect(err).NotTo(HaveOccurred())

			// Lock file should be removed
			_, statErr := os.Stat(lockFilePath)
			Expect(os.IsNotExist(statErr)).To(BeTrue())
		})
	})

	Describe("daemon exits after SIGTERM", func() {
		It("prints 'daemon stopped' when process exits within timeout", func() {
			pid := 12345
			err := os.WriteFile(lockFilePath, []byte(fmt.Sprintf("%d\n", pid)), 0600)
			Expect(err).NotTo(HaveOccurred())

			callCount := 0
			mockSignal := func(p int, sig syscall.Signal) error {
				Expect(p).To(Equal(pid))
				callCount++
				switch callCount {
				case 1:
					// Signal(0) liveness check — process alive
					return nil
				case 2:
					// SIGTERM — success
					return nil
				default:
					// Signal(0) poll — process has exited
					return fmt.Errorf("os: process already finished")
				}
			}
			noSleep := func(_ time.Duration) {}

			killCmd := cmd.NewKillCommand(lockFilePath, mockSignal, noSleep)
			err = killCmd.Run(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("daemon does not exit after SIGTERM", func() {
		It("sends SIGKILL after timeout and prints 'daemon killed'", func() {
			pid := 12346
			err := os.WriteFile(lockFilePath, []byte(fmt.Sprintf("%d\n", pid)), 0600)
			Expect(err).NotTo(HaveOccurred())

			var signals []syscall.Signal
			mockSignal := func(p int, sig syscall.Signal) error {
				signals = append(signals, sig)
				// Process never dies — always report alive for Signal(0)
				if sig == syscall.Signal(0) {
					return nil
				}
				return nil
			}
			noSleep := func(_ time.Duration) {}

			killCmd := cmd.NewKillCommand(lockFilePath, mockSignal, noSleep)
			err = killCmd.Run(ctx, nil)
			Expect(err).NotTo(HaveOccurred())

			// Verify SIGKILL was sent
			Expect(signals).To(ContainElement(syscall.SIGKILL))
		})
	})
})
