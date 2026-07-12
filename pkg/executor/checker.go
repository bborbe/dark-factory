// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	stderrors "errors"
	"os/exec"
	"strings"
	"time"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/subproc"
)

// ErrDockerDaemonUnavailable signals that the Docker daemon could not be reached
// (socket missing, daemon not running). Callers MUST NOT treat this as
// "container not running" — the container's actual state is unknown.
var ErrDockerDaemonUnavailable = stderrors.New("docker daemon unavailable")

// isDockerDaemonUnavailable returns true when docker stderr indicates the
// daemon socket is unreachable (host docker stopped, socket path missing).
func isDockerDaemonUnavailable(stderr string) bool {
	return strings.Contains(stderr, "Cannot connect to the Docker daemon") ||
		strings.Contains(stderr, "Is the docker daemon running")
}

//counterfeiter:generate -o ../../mocks/execution-checker.go --fake-name ExecutionChecker . ExecutionChecker

// ExecutionChecker checks whether a unit of execution (identified by executionID) is currently running.
type ExecutionChecker interface {
	IsRunning(ctx context.Context, executionID string) (bool, error)
	// WaitUntilRunning blocks until the named execution is in the running state,
	// the timeout elapses, or ctx is cancelled.
	WaitUntilRunning(ctx context.Context, executionID string, timeout time.Duration) error
}

// NewDockerExecutionChecker creates an ExecutionChecker backed by docker inspect.
func NewDockerExecutionChecker(
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) ExecutionChecker {
	return &dockerContainerChecker{currentDateTimeGetter: currentDateTimeGetter}
}

// dockerContainerChecker implements ExecutionChecker using docker inspect.
type dockerContainerChecker struct {
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// WaitUntilRunning polls docker inspect every 2 seconds until the named execution
// is running, the timeout expires, or ctx is cancelled.
func (c *dockerContainerChecker) WaitUntilRunning(
	ctx context.Context,
	executionID string,
	timeout time.Duration,
) error {
	deadline := time.Time(c.currentDateTimeGetter.Now()).Add(timeout)
	for {
		running, err := c.IsRunning(ctx, executionID)
		if err != nil {
			return errors.Wrap(ctx, err, "check container running")
		}
		if running {
			return nil
		}
		if time.Time(c.currentDateTimeGetter.Now()).After(deadline) {
			return errors.Errorf(ctx, "container %s did not start within %s", executionID, timeout)
		}
		select {
		case <-ctx.Done():
			return errors.Wrapf(ctx, ctx.Err(), "wait until running cancelled")
		case <-time.After(2 * time.Second):
		}
	}
}

// IsRunning returns true if the named execution is currently running.
// If the container does not exist, it returns false with no error.
// If the Docker daemon is unreachable, it returns ErrDockerDaemonUnavailable —
// the container's state is unknown and callers must NOT treat this as "absent".
func (c *dockerContainerChecker) IsRunning(ctx context.Context, executionID string) (bool, error) {
	// #nosec G204 -- containerName is generated internally from prompt filename, not user input
	cmd := exec.CommandContext(
		ctx,
		"docker",
		"inspect",
		"--format",
		"{{.State.Running}}",
		executionID,
	)
	var out, stderr strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if isDockerDaemonUnavailable(stderr.String()) {
			return false, ErrDockerDaemonUnavailable
		}
		// Non-zero exit for any other reason means the container does not exist.
		return false, nil
	}
	return strings.TrimSpace(out.String()) == "true", nil
}

//counterfeiter:generate -o ../../mocks/container-counter.go --fake-name ContainerCounter . ContainerCounter

// ContainerCounter counts running dark-factory containers system-wide.
type ContainerCounter interface {
	CountRunning(ctx context.Context) (int, error)
}

// NewNoopContainerCounter creates a ContainerCounter that always reports zero
// running containers without invoking docker. Used by the local backend, where
// there are no containers to count and `docker ps` must never run (no docker
// daemon is required when backend: local).
func NewNoopContainerCounter() ContainerCounter {
	return &noopContainerCounter{}
}

// noopContainerCounter implements ContainerCounter for the local backend.
type noopContainerCounter struct{}

// CountRunning always returns (0, nil) — the local backend spawns no containers,
// so there is nothing to count and no docker daemon is contacted.
func (c *noopContainerCounter) CountRunning(ctx context.Context) (int, error) {
	return 0, nil
}

// NewDockerContainerCounter creates a ContainerCounter that uses docker ps with label filtering.
func NewDockerContainerCounter(runner subproc.Runner) ContainerCounter {
	return &dockerContainerCounter{runner: runner}
}

// dockerContainerCounter implements ContainerCounter using docker ps.
type dockerContainerCounter struct {
	runner subproc.Runner
}

// CountRunning returns the number of currently running dark-factory containers system-wide.
// It filters by the label dark-factory.project which is set on every container.
func (c *dockerContainerCounter) CountRunning(ctx context.Context) (int, error) {
	out, err := c.runner.RunWithWarnAndTimeout(
		ctx,
		"docker ps --filter label=dark-factory.project",
		"docker",
		"ps",
		"--filter",
		"label=dark-factory.project",
		"--format",
		"{{.Names}}",
	)
	if errors.Is(err, context.DeadlineExceeded) {
		return 0, err // return sentinel unwrapped so caller's errors.Is check still works
	}
	if err != nil {
		return 0, errors.Wrap(ctx, err, "docker ps for container count")
	}
	output := strings.TrimSpace(string(out))
	if output == "" {
		return 0, nil
	}
	lines := strings.Split(output, "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count, nil
}
