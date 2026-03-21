// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"os/exec"
	"strings"
)

//counterfeiter:generate -o ../../mocks/container-checker.go --fake-name ContainerChecker . ContainerChecker

// ContainerChecker checks whether a Docker container is currently running.
type ContainerChecker interface {
	IsRunning(ctx context.Context, name string) (bool, error)
}

// NewDockerContainerChecker creates a ContainerChecker backed by docker inspect.
func NewDockerContainerChecker() ContainerChecker {
	return &dockerContainerChecker{}
}

// dockerContainerChecker implements ContainerChecker using docker inspect.
type dockerContainerChecker struct{}

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
