// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"os/exec"

	"github.com/bborbe/errors"
)

//counterfeiter:generate -o ../../mocks/container-stopper.go --fake-name ContainerStopper . ContainerStopper

// ContainerStopper stops a running Docker container by name.
type ContainerStopper interface {
	StopContainer(ctx context.Context, name string) error
}

// NewDockerContainerStopper creates a ContainerStopper backed by docker stop.
func NewDockerContainerStopper() ContainerStopper {
	return &dockerContainerStopper{}
}

// dockerContainerStopper implements ContainerStopper using docker stop.
type dockerContainerStopper struct{}

// StopContainer sends SIGTERM to the named container and waits for it to exit.
func (s *dockerContainerStopper) StopContainer(ctx context.Context, name string) error {
	// #nosec G204 -- name is generated internally from prompt filename, not user input
	cmd := exec.CommandContext(ctx, "docker", "stop", name)
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(ctx, err, "docker stop %s", name)
	}
	return nil
}
