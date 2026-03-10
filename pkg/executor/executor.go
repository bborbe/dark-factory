// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"
)

//counterfeiter:generate -o ../../mocks/executor.go --fake-name Executor . Executor

// Executor executes a prompt.
type Executor interface {
	Execute(ctx context.Context, promptContent string, logFile string, containerName string) error
}

// dockerExecutor implements Executor using Docker.
type dockerExecutor struct {
	containerImage string
	projectName    string
	model          string
	netrcFile      string
	gitconfigFile  string
	commandRunner  commandRunner
}

// NewDockerExecutor creates a new Executor using Docker with the specified container image.
func NewDockerExecutor(
	containerImage string,
	projectName string,
	model string,
	netrcFile string,
	gitconfigFile string,
) Executor {
	return &dockerExecutor{
		containerImage: containerImage,
		projectName:    projectName,
		model:          model,
		netrcFile:      netrcFile,
		gitconfigFile:  gitconfigFile,
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

	// Remove any existing container with this name (handles interrupted previous runs)
	e.removeContainerIfExists(ctx, containerName)

	// Extract prompt basename from containerName (format: projectName-basename)
	promptBaseName := extractPromptBaseName(containerName, e.projectName)

	// Resolve Claude config dir (env var override or default ~/.claude)
	claudeConfigDir := resolveClaudeConfigDir(home)

	// Validate Claude auth before starting Docker
	if err := validateClaudeAuth(ctx, claudeConfigDir); err != nil {
		return err
	}

	// Build and run docker command
	cmd := e.buildDockerCommand(
		ctx,
		containerName,
		promptFilePath,
		projectRoot,
		claudeConfigDir,
		promptBaseName,
		home,
	)

	slog.Debug("docker command prepared",
		"image", e.containerImage,
		"containerName", containerName,
		"workspaceMount", projectRoot+":/workspace",
		"configMount", claudeConfigDir+":/home/node/.claude",
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
	claudeConfigDir string,
	promptBaseName string,
	home string,
) *exec.Cmd {
	// Build docker run command args
	// Mount prompt as file to avoid shell escaping issues with -e flag
	args := []string{
		"run", "--rm",
		"--name", containerName,
		"--label", "dark-factory.project=" + e.projectName,
		"--label", "dark-factory.prompt=" + promptBaseName,
		"--cap-add=NET_ADMIN", "--cap-add=NET_RAW",
	}
	args = append(args,
		"-e", "YOLO_PROMPT_FILE=/tmp/prompt.md",
		"-e", "ANTHROPIC_MODEL="+e.model,
		"-v", promptFilePath+":/tmp/prompt.md:ro",
		"-v", projectRoot+":/workspace",
		"-v", claudeConfigDir+":/home/node/.claude",
		"-v", home+"/go/pkg:/home/node/go/pkg",
	)
	if e.netrcFile != "" {
		resolved := e.netrcFile
		if strings.HasPrefix(resolved, "~/") {
			resolved = home + resolved[1:]
		}
		args = append(args, "-v", resolved+":/home/node/.netrc:ro")
	}
	if e.gitconfigFile != "" {
		resolved := e.gitconfigFile
		if strings.HasPrefix(resolved, "~/") {
			resolved = home + resolved[1:]
		}
		args = append(args, "-v", resolved+":/home/node/.gitconfig")
	}
	args = append(args, e.containerImage)
	// #nosec G204 -- promptContent is user-provided by design
	return exec.CommandContext(ctx, "docker", args...)
}

// validateClaudeAuth checks that the Claude config directory contains a valid OAuth token.
// If ANTHROPIC_API_KEY is set, the check is skipped (API key auth does not need OAuth).
// Supports both legacy (.claude.json oauthAccount.accessToken) and current
// (.credentials.json claudeAiOauth.accessToken) token locations.
func validateClaudeAuth(_ context.Context, configDir string) error {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return nil
	}

	// Check current location: .credentials.json (Claude Code v2.x+)
	credentialsFile := filepath.Join(configDir, ".credentials.json")
	// #nosec G304 -- credentialsFile is derived from resolved config dir, not user input
	if data, err := os.ReadFile(credentialsFile); err == nil {
		var creds struct {
			ClaudeAiOauth *struct {
				AccessToken string `json:"accessToken"`
			} `json:"claudeAiOauth"`
		}
		if err := json.Unmarshal(data, &creds); err == nil &&
			creds.ClaudeAiOauth != nil && creds.ClaudeAiOauth.AccessToken != "" {
			return nil
		}
	}

	// Fallback: check legacy location: .claude.json (Claude Code v1.x)
	configFile := filepath.Join(configDir, ".claude.json")
	// #nosec G304 -- configFile is derived from resolved config dir, not user input
	if data, err := os.ReadFile(configFile); err == nil {
		var cfg struct {
			OAuthAccount *struct {
				AccessToken string `json:"accessToken"`
			} `json:"oauthAccount"`
		}
		if err := json.Unmarshal(data, &cfg); err == nil &&
			cfg.OAuthAccount != nil && cfg.OAuthAccount.AccessToken != "" {
			return nil
		}
	}

	return fmt.Errorf(
		"Claude OAuth token missing or expired in %s\n\nFix: Run 'CLAUDE_CONFIG_DIR=%s claude' and use /login",
		configDir,
		configDir,
	)
}

// resolveClaudeConfigDir returns the Claude config directory to mount in the container.
// It reads DARK_FACTORY_CLAUDE_CONFIG_DIR from the environment; if empty, defaults to ~/.claude.
// Supports ~ prefix and $HOME/$variable expansion.
func resolveClaudeConfigDir(home string) string {
	dir := os.Getenv("DARK_FACTORY_CLAUDE_CONFIG_DIR")
	if dir == "" {
		return home + "/.claude"
	}
	// Expand leading ~ to home directory
	if dir == "~" {
		dir = home
	} else if strings.HasPrefix(dir, "~/") {
		dir = home + dir[1:]
	}
	// Expand $HOME and other environment variables
	dir = os.ExpandEnv(dir)
	return dir
}

// removeContainerIfExists removes a container by name if it exists, ignoring errors.
// docker rm -f is idempotent: it returns non-zero if the container doesn't exist, which is fine.
func (e *dockerExecutor) removeContainerIfExists(ctx context.Context, containerName string) {
	// #nosec G204 -- containerName is derived from prompt filename, not user input
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerName)
	if err := e.commandRunner.Run(ctx, cmd); err != nil {
		// Non-zero exit is expected when container doesn't exist
		slog.Debug("docker rm -f", "containerName", containerName, "error", err)
	}
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
