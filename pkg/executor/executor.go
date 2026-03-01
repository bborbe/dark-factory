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
	Execute(ctx context.Context, promptContent string, logFile string, containerName string) error
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
func (e *DockerExecutor) Execute(
	ctx context.Context,
	promptContent string,
	logFile string,
	containerName string,
) error {
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

	// Prepare log file
	logFileHandle, err := PrepareLogFile(ctx, logFile)
	if err != nil {
		return err
	}
	defer logFileHandle.Close()

	// Create temp file with prompt content
	promptFilePath, cleanup, err := CreatePromptTempFile(ctx, promptContent)
	if err != nil {
		return err
	}
	defer cleanup()

	// Build and run docker command
	cmd := BuildDockerCommand(ctx, containerName, promptFilePath, projectRoot, home)

	// Pipe stdout/stderr to both terminal and log file
	cmd.Stdout = io.MultiWriter(os.Stdout, logFileHandle)
	cmd.Stderr = io.MultiWriter(os.Stderr, logFileHandle)

	// Run and wait for completion
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "docker run failed")
	}

	return nil
}

// PrepareLogFile creates the log directory and opens the log file for writing.
func PrepareLogFile(ctx context.Context, logFile string) (*os.File, error) {
	// Create log file directory if it doesn't exist
	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return nil, errors.Wrap(ctx, err, "create log directory")
	}

	// Open log file for writing (create/truncate)
	// #nosec G304 -- logFile is derived from prompt filename, not user input
	logFileHandle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "open log file")
	}

	return logFileHandle, nil
}

// CreatePromptTempFile creates a temp file with the prompt content and returns the path and cleanup function.
func CreatePromptTempFile(ctx context.Context, promptContent string) (string, func(), error) {
	// Write prompt to temp file (avoids shell/docker escaping issues with -e flag)
	promptFile, err := os.CreateTemp("", "dark-factory-prompt-*.md")
	if err != nil {
		return "", nil, errors.Wrap(ctx, err, "create prompt temp file")
	}

	cleanup := func() {
		promptFile.Close()
		_ = os.Remove(promptFile.Name())
	}

	if _, err := promptFile.WriteString(promptContent); err != nil {
		cleanup()
		return "", nil, errors.Wrap(ctx, err, "write prompt temp file")
	}

	if err := promptFile.Close(); err != nil {
		cleanup()
		return "", nil, errors.Wrap(ctx, err, "close prompt temp file")
	}

	return promptFile.Name(), cleanup, nil
}

// BuildDockerCommand builds the docker run command with all necessary arguments.
func BuildDockerCommand(
	ctx context.Context,
	containerName string,
	promptFilePath string,
	projectRoot string,
	home string,
) *exec.Cmd {
	// Build docker run command
	// Mount prompt as file to avoid shell escaping issues with -e flag
	// #nosec G204 -- promptContent is user-provided by design
	return exec.CommandContext(
		ctx,
		"docker", "run", "--rm",
		"--name", containerName,
		"--cap-add=NET_ADMIN", "--cap-add=NET_RAW",
		"-e", "YOLO_PROMPT_FILE=/tmp/prompt.md",
		"-v", promptFilePath+":/tmp/prompt.md:ro",
		"-v", projectRoot+":/workspace",
		"-v", home+"/.claude-yolo:/home/node/.claude",
		"-v", home+"/go/pkg:/home/node/go/pkg",
		"docker.io/bborbe/claude-yolo:latest",
	)
}
