// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/bborbe/errors"
)

//counterfeiter:generate -o ../../mocks/container-checker.go --fake-name ContainerChecker . ContainerChecker

// ContainerChecker checks whether a Docker container is currently running.
type ContainerChecker interface {
	IsRunning(ctx context.Context, name string) (bool, error)
	// WaitUntilRunning blocks until the named container is in the running state,
	// the timeout elapses, or ctx is cancelled.
	WaitUntilRunning(ctx context.Context, name string, timeout time.Duration) error
}

// NewDockerContainerChecker creates a ContainerChecker backed by docker inspect.
func NewDockerContainerChecker() ContainerChecker {
	return &dockerContainerChecker{}
}

// dockerContainerChecker implements ContainerChecker using docker inspect.
type dockerContainerChecker struct{}

// WaitUntilRunning polls docker inspect every 2 seconds until the named container
// is running, the timeout expires, or ctx is cancelled.
func (c *dockerContainerChecker) WaitUntilRunning(
	ctx context.Context,
	name string,
	timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)
	for {
		running, err := c.IsRunning(ctx, name)
		if err != nil {
			return errors.Wrapf(ctx, err, "check container running")
		}
		if running {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.Errorf(ctx, "container %s did not start within %s", name, timeout)
		}
		select {
		case <-ctx.Done():
			return errors.Wrapf(ctx, ctx.Err(), "wait until running cancelled")
		case <-time.After(2 * time.Second):
		}
	}
}

// IsRunning returns true if the named container is currently running.
// If the container does not exist, it returns false with no error.
func (c *dockerContainerChecker) IsRunning(ctx context.Context, name string) (bool, error) {
	// #nosec G204 -- containerName is generated internally from prompt filename, not user input
	cmd := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Running}}", name)
	var out strings.Builder
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		// Non-zero exit means container does not exist — treat as not running
		return false, nil
	}
	return strings.TrimSpace(out.String()) == "true", nil
}

//counterfeiter:generate -o ../../mocks/container-counter.go --fake-name ContainerCounter . ContainerCounter

// ContainerCounter counts running dark-factory containers system-wide.
type ContainerCounter interface {
	CountRunning(ctx context.Context) (int, error)
}

// NewDockerContainerCounter creates a ContainerCounter that uses docker ps with label filtering.
func NewDockerContainerCounter() ContainerCounter {
	return &dockerContainerCounter{}
}

// dockerContainerCounter implements ContainerCounter using docker ps.
type dockerContainerCounter struct{}

// CountRunning returns the number of currently running dark-factory containers system-wide.
// It filters by the label dark-factory.project which is set on every container.
func (c *dockerContainerCounter) CountRunning(ctx context.Context) (int, error) {
	// #nosec G204 -- filter value is a hardcoded label key, not user input
	cmd := exec.CommandContext(
		ctx,
		"docker", "ps",
		"--filter", "label=dark-factory.project",
		"--format", "{{.Names}}",
	)
	var out strings.Builder
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0, errors.Wrapf(ctx, err, "docker ps for container count")
	}
	output := strings.TrimSpace(out.String())
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
