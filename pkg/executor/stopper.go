// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"os/exec"

	"github.com/bborbe/errors"
)

//counterfeiter:generate -o ../../mocks/execution-stopper.go --fake-name ExecutionStopper . ExecutionStopper

// ExecutionStopper stops a running unit of execution by its executionID.
type ExecutionStopper interface {
	StopContainer(ctx context.Context, executionID string) error
}

// NewDockerExecutionStopper creates an ExecutionStopper backed by docker stop.
func NewDockerExecutionStopper() ExecutionStopper {
	return &dockerContainerStopper{}
}

// dockerContainerStopper implements ExecutionStopper using docker stop.
type dockerContainerStopper struct{}

// StopContainer sends SIGTERM to the named execution and waits for it to exit.
func (s *dockerContainerStopper) StopContainer(ctx context.Context, executionID string) error {
	// #nosec G204 -- name is generated internally from prompt filename, not user input
	cmd := exec.CommandContext(ctx, "docker", "stop", executionID)
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(ctx, err, "docker stop %s", executionID)
	}
	return nil
}
