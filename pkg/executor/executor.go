// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bborbe/errors"
)

// Executor executes a prompt.
//
//counterfeiter:generate -o ../../mocks/executor.go --fake-name FakeExecutor . Executor
type Executor interface {
	Execute(ctx context.Context, promptContent string, logFile string) error
}

// DockerExecutor implements Executor using Docker.
type DockerExecutor struct{}

// NewDockerExecutor creates a new DockerExecutor.
func NewDockerExecutor() *DockerExecutor {
	return &DockerExecutor{}
}

// Execute runs the claude-yolo Docker container with the given prompt content.
// It blocks until the container exits and returns an error if the exit code is non-zero.
// Output is streamed to both terminal and the specified log file.
func (e *DockerExecutor) Execute(ctx context.Context, promptContent string, logFile string) error {
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

	// Create log file directory if it doesn't exist
	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return errors.Wrap(ctx, err, "create log directory")
	}

	// Open log file for writing (create/truncate)
	// #nosec G304 -- logFile is derived from prompt filename, not user input
	logFileHandle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return errors.Wrap(ctx, err, "open log file")
	}
	defer logFileHandle.Close()

	// Write prompt to temp file (avoids shell/docker escaping issues with -e flag)
	promptFile, err := os.CreateTemp("", "dark-factory-prompt-*.md")
	if err != nil {
		return errors.Wrap(ctx, err, "create prompt temp file")
	}
	defer func() { _ = os.Remove(promptFile.Name()) }()

	if _, err := promptFile.WriteString(promptContent); err != nil {
		promptFile.Close()
		return errors.Wrap(ctx, err, "write prompt temp file")
	}
	promptFile.Close()

	// Build docker run command
	// Mount prompt as file to avoid shell escaping issues with -e flag
	// #nosec G204 -- promptContent is user-provided by design
	cmd := exec.CommandContext(
		ctx,
		"docker", "run", "--rm",
		"--cap-add=NET_ADMIN", "--cap-add=NET_RAW",
		"-e", "YOLO_PROMPT_FILE=/tmp/prompt.md",
		"-v", promptFile.Name()+":/tmp/prompt.md:ro",
		"-v", projectRoot+":/workspace",
		"-v", home+"/.claude-yolo:/home/node/.claude",
		"-v", home+"/go/pkg:/home/node/go/pkg",
		"docker.io/bborbe/claude-yolo:latest",
	)

	// Pipe stdout/stderr to both terminal and log file
	cmd.Stdout = io.MultiWriter(os.Stdout, logFileHandle)
	cmd.Stderr = io.MultiWriter(os.Stderr, logFileHandle)

	// Run and wait for completion
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "docker run failed")
	}

	return nil
}
