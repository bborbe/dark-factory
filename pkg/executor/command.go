// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/bborbe/errors"
)

//counterfeiter:generate -o ../../mocks/command-runner.go --fake-name CommandRunner . commandRunner

// commandRunner runs an already-constructed *exec.Cmd, managing the docker
// SIGINT-then-SIGKILL escalation on context cancellation (cmd.Process.Signal
// then cmd.Process.Kill after a grace period). This is the docker stop/kill
// signal protocol and is deliberately NOT routed through pkg/subproc.Runner:
// the runner offers warn+timeout semantics over cmd.Output() but no signal
// escalation, and the docker *exec.Cmd construction lives in executor.go
// (allow-listed in scripts/hotpath-execcheck.sh). See spec 100 Desired
// Behavior 6 / Non-goal "Do NOT replace the docker CLI executor".
type commandRunner interface {
	Run(ctx context.Context, cmd *exec.Cmd) error
}

// defaultCommandRunner runs commands directly, respecting context cancellation.
type defaultCommandRunner struct{}

func (r *defaultCommandRunner) Run(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return errors.Wrap(ctx, err, "start command")
	}

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				_ = cmd.Process.Kill()
			}
		case <-done:
		}
	}()

	if err := cmd.Wait(); err != nil {
		return errors.Wrap(ctx, err, "wait command")
	}
	return nil
}
