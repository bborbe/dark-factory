// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bborbe/errors"
)

//counterfeiter:generate -o ../../mocks/kill-command.go --fake-name KillCommand . KillCommand

// KillCommand executes the kill subcommand.
type KillCommand interface {
	Run(ctx context.Context, args []string) error
}

// killCommand implements KillCommand.
type killCommand struct {
	lockFilePath string
	signalFunc   func(pid int, sig syscall.Signal) error
	sleepFunc    func(d time.Duration)
}

// NewKillCommand creates a new KillCommand.
// signalFunc and sleepFunc may be nil; defaults are used in that case.
func NewKillCommand(
	lockFilePath string,
	signalFunc func(pid int, sig syscall.Signal) error,
	sleepFunc func(d time.Duration),
) KillCommand {
	if signalFunc == nil {
		signalFunc = func(pid int, sig syscall.Signal) error {
			proc, err := os.FindProcess(pid)
			if err != nil {
				return err
			}
			return proc.Signal(sig)
		}
	}
	if sleepFunc == nil {
		sleepFunc = time.Sleep
	}
	return &killCommand{
		lockFilePath: lockFilePath,
		signalFunc:   signalFunc,
		sleepFunc:    sleepFunc,
	}
}

// Run executes the kill command.
func (k *killCommand) Run(ctx context.Context, _ []string) error {
	// Read lock file
	data, err := os.ReadFile(k.lockFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("no daemon running")
			return nil
		}
		return errors.Wrap(ctx, err, "read lock file")
	}

	pidStr := strings.TrimSpace(string(data))
	if pidStr == "" {
		fmt.Println("no daemon running")
		return nil
	}

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		fmt.Println("no daemon running")
		return nil
	}

	// Check if process is alive
	if err := k.signalFunc(pid, syscall.Signal(0)); err != nil {
		fmt.Printf("no daemon running (stale lock file)\n")
		_ = os.Remove(k.lockFilePath)
		return nil
	}

	// Send SIGTERM
	if err := k.signalFunc(pid, syscall.SIGTERM); err != nil {
		fmt.Printf("no daemon running (stale lock file)\n")
		_ = os.Remove(k.lockFilePath)
		return nil
	}

	// Wait up to 5 seconds for process to exit
	const maxPolls = 10
	const pollInterval = 500 * time.Millisecond
	for i := 0; i < maxPolls; i++ {
		k.sleepFunc(pollInterval)
		if err := k.signalFunc(pid, syscall.Signal(0)); err != nil {
			// Process exited
			fmt.Printf("daemon stopped (pid %d)\n", pid)
			return nil
		}
	}

	// Process still running after 5s — send SIGKILL
	_ = k.signalFunc(pid, syscall.SIGKILL)
	fmt.Printf("daemon killed (pid %d)\n", pid)
	return nil
}
