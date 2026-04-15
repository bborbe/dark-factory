// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package subproc provides bounded-duration subprocess execution for
// short-lived read-only commands (git status, docker ps). Each call emits a
// warning to stderr after warnAfter and is cancelled at timeout.
package subproc

import (
	"context"
	"log/slog"
	"os/exec"
	"time"

	"github.com/bborbe/errors"
	"github.com/bborbe/run"
)

//counterfeiter:generate -o ../../mocks/subproc-runner.go --fake-name SubprocRunner . Runner

// Default thresholds for RunWithWarnAndTimeout.
const (
	DefaultWarnAfter = 3 * time.Second
	DefaultTimeout   = 10 * time.Second
)

// Runner runs short subprocesses with warn + timeout semantics.
type Runner interface {
	// RunWithWarnAndTimeout runs `name args...` bounded by the configured
	// warnAfter/timeout. On timeout it returns a non-nil error and the
	// caller should treat the operation as skipped.
	//
	// op is a human-readable operation label used in warn/skip messages
	// (e.g. "git status --porcelain").
	RunWithWarnAndTimeout(
		ctx context.Context,
		op string,
		name string,
		args ...string,
	) ([]byte, error)

	// RunWithWarnAndTimeoutDir is identical to RunWithWarnAndTimeout but sets
	// cmd.Dir = dir before running, preserving the working directory.
	RunWithWarnAndTimeoutDir(
		ctx context.Context,
		op string,
		dir string,
		name string,
		args ...string,
	) ([]byte, error)
}

// NewRunner returns a Runner using the default 3s/10s thresholds.
func NewRunner() Runner {
	return &runner{warnAfter: DefaultWarnAfter, timeout: DefaultTimeout}
}

// NewRunnerWithThresholds returns a Runner with custom thresholds (for tests).
func NewRunnerWithThresholds(warnAfter, timeout time.Duration) Runner {
	return &runner{warnAfter: warnAfter, timeout: timeout}
}

type runner struct {
	warnAfter time.Duration
	timeout   time.Duration
}

// RunWithWarnAndTimeout runs a subprocess with warn+timeout semantics and no working directory.
func (r *runner) RunWithWarnAndTimeout(
	ctx context.Context,
	op string,
	name string,
	args ...string,
) ([]byte, error) {
	return r.runInternal(ctx, op, "", name, args...)
}

// RunWithWarnAndTimeoutDir runs a subprocess with warn+timeout semantics in the given directory.
func (r *runner) RunWithWarnAndTimeoutDir(
	ctx context.Context,
	op string,
	dir string,
	name string,
	args ...string,
) ([]byte, error) {
	return r.runInternal(ctx, op, dir, name, args...)
}

func (r *runner) runInternal(
	ctx context.Context,
	op string,
	dir string,
	name string,
	args ...string,
) ([]byte, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	var output []byte
	var cmdErr error

	err := run.CancelOnFirstFinishWait(
		cmdCtx,
		// Subprocess runner — finishes first in the fast path, which cancels the warn goroutine via ctx.
		func(ctx context.Context) error {
			// #nosec G204 -- name and args are from trusted internal call sites, not user input
			cmd := exec.CommandContext(ctx, name, args...)
			if dir != "" {
				cmd.Dir = dir
			}
			output, cmdErr = cmd.Output()
			return nil // surface cmd error via cmdErr, not here, so both funcs exit cleanly
		},
		// Warn-after-threshold watcher — exits immediately when cmdCtx is cancelled (by cmd finishing or timeout).
		func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(r.warnAfter):
				slog.Warn("subprocess slow", "op", op, "threshold", r.warnAfter)
			}
			<-ctx.Done() // block until cmd finishes or ctx times out
			return nil
		},
	)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "run subprocess")
	}

	// Detect timeout: cmdCtx deadline exceeded means our 10s kicked in.
	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		slog.Warn("subprocess skipped", "op", op, "timeout", r.timeout)
		return nil, context.DeadlineExceeded
	}
	if cmdErr != nil {
		return nil, errors.Wrapf(ctx, cmdErr, "%s failed", op)
	}
	return output, nil
}
