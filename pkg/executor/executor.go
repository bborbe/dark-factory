// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"os"
	"os/exec"

	"github.com/bborbe/errors"
)

// Execute runs the claude-yolo Docker container with the given prompt content.
// It blocks until the container exits and returns an error if the exit code is non-zero.
func Execute(ctx context.Context, promptContent string) error {
	// Get project root (current working directory)
	projectRoot, err := os.Getwd()
	if err != nil {
		return errors.Wrap(ctx, err, "get working directory")
	}

	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return errors.Wrap(ctx, err, "get home directory")
	}

	// Build docker run command
	// #nosec G204 -- promptContent is user-provided by design
	cmd := exec.CommandContext(
		ctx,
		"docker", "run", "--rm",
		"--cap-add=NET_ADMIN", "--cap-add=NET_RAW",
		"-e", "YOLO_PROMPT="+promptContent,
		"-v", projectRoot+":/workspace",
		"-v", home+"/.claude-yolo:/home/node/.claude",
		"-v", home+"/go/pkg:/home/node/go/pkg",
		"docker.io/bborbe/claude-yolo:latest",
	)

	// Pipe stdout/stderr directly
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run and wait for completion
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "docker run failed")
	}

	return nil
}
