// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bborbe/errors"
)

// Executor executes a prompt.
//
//counterfeiter:generate -o ../../mocks/executor.go --fake-name Executor . Executor
type Executor interface {
	Execute(ctx context.Context, promptContent string, logFile string, containerName string) error
}

// dockerExecutor implements Executor using Docker.
type dockerExecutor struct {
	containerImage string
	projectName    string
	model          string
	commandRunner  commandRunner
}

// NewDockerExecutor creates a new Executor using Docker with the specified container image.
func NewDockerExecutor(containerImage string, projectName string, model string) Executor {
	return &dockerExecutor{
		containerImage: containerImage,
		projectName:    projectName,
		model:          model,
		commandRunner:  &defaultCommandRunner{},
	}
}

// Execute runs the claude-yolo Docker container with the given prompt content.
// It blocks until the container exits and returns an error if the exit code is non-zero.
// Output is streamed to both terminal and the specified log file.
func (e *dockerExecutor) Execute(
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
	logFileHandle, err := prepareLogFile(ctx, logFile)
	if err != nil {
		return errors.Wrap(ctx, err, "prepare log file")
	}
	defer logFileHandle.Close()

	// Create temp file with prompt content
	promptFilePath, cleanup, err := createPromptTempFile(ctx, promptContent)
	if err != nil {
		return errors.Wrap(ctx, err, "create prompt temp file")
	}
	defer cleanup()

	slog.Debug(
		"prompt prepared for execution",
		"contentSize",
		len(promptContent),
		"tempFile",
		promptFilePath,
	)

	// Extract prompt basename from containerName (format: projectName-basename)
	promptBaseName := extractPromptBaseName(containerName, e.projectName)

	// Build and run docker command
	cmd := e.buildDockerCommand(
		ctx,
		containerName,
		promptFilePath,
		projectRoot,
		home,
		promptBaseName,
	)

	slog.Debug("docker command prepared",
		"image", e.containerImage,
		"containerName", containerName,
		"workspaceMount", projectRoot+":/workspace",
		"configMount", home+"/.claude-yolo:/home/node/.claude",
		"goPkgMount", home+"/go/pkg:/home/node/go/pkg")

	// Pipe stdout/stderr to both terminal and log file
	cmd.Stdout = io.MultiWriter(os.Stdout, logFileHandle)
	cmd.Stderr = io.MultiWriter(os.Stderr, logFileHandle)

	// Run and wait for completion
	if err := e.commandRunner.Run(ctx, cmd); err != nil {
		return errors.Wrap(ctx, err, "docker run failed")
	}

	return nil
}

// prepareLogFile creates the log directory and opens the log file for writing.
func prepareLogFile(ctx context.Context, logFile string) (*os.File, error) {
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

// createPromptTempFile creates a temp file with the prompt content and returns the path and cleanup function.
func createPromptTempFile(ctx context.Context, promptContent string) (string, func(), error) {
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

// buildDockerCommand builds the docker run command with all necessary arguments.
func (e *dockerExecutor) buildDockerCommand(
	ctx context.Context,
	containerName string,
	promptFilePath string,
	projectRoot string,
	home string,
	promptBaseName string,
) *exec.Cmd {
	// Build docker run command
	// Mount prompt as file to avoid shell escaping issues with -e flag
	// #nosec G204 -- promptContent is user-provided by design
	return exec.CommandContext(
		ctx,
		"docker", "run", "--rm",
		"--name", containerName,
		"--label", "dark-factory.project="+e.projectName,
		"--label", "dark-factory.prompt="+promptBaseName,
		"--cap-add=NET_ADMIN", "--cap-add=NET_RAW",
		"-e", "YOLO_PROMPT_FILE=/tmp/prompt.md",
		"-e", "ANTHROPIC_MODEL="+e.model,
		"-v", promptFilePath+":/tmp/prompt.md:ro",
		"-v", projectRoot+":/workspace",
		"-v", home+"/.claude-yolo:/home/node/.claude",
		"-v", home+"/go/pkg:/home/node/go/pkg",
		e.containerImage,
	)
}

// extractPromptBaseName extracts the prompt basename from the containerName.
// containerName is in the format "projectName-basename".
func extractPromptBaseName(containerName string, projectName string) string {
	prefix := projectName + "-"
	if len(containerName) > len(prefix) && containerName[:len(prefix)] == prefix {
		return containerName[len(prefix):]
	}
	// Fallback if format doesn't match (should not happen)
	return containerName
}
