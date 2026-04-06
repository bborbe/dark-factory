// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bborbe/errors"
	"github.com/bborbe/run"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/report"
)

//counterfeiter:generate -o ../../mocks/executor.go --fake-name Executor . Executor

// Executor executes a prompt.
type Executor interface {
	Execute(ctx context.Context, promptContent string, logFile string, containerName string) error
	// Reattach connects to a running container's output stream and waits for it to exit.
	// It does not create a new container. The log file is overwritten from the beginning
	// of the container's output (docker logs replays all output from container start).
	// maxPromptDuration is the remaining allowed run time; 0 disables the timeout.
	// Returns nil when the container exits successfully.
	Reattach(
		ctx context.Context,
		logFile string,
		containerName string,
		maxPromptDuration time.Duration,
	) error
	StopAndRemoveContainer(ctx context.Context, containerName string)
}

// NewDockerExecutor creates a new Executor using Docker with the specified container image.
func NewDockerExecutor(
	containerImage string,
	projectName string,
	model string,
	netrcFile string,
	gitconfigFile string,
	env map[string]string,
	extraMounts []config.ExtraMount,
	claudeDir string,
	maxPromptDuration time.Duration,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) Executor {
	return &dockerExecutor{
		containerImage:        containerImage,
		projectName:           projectName,
		model:                 model,
		netrcFile:             netrcFile,
		gitconfigFile:         gitconfigFile,
		env:                   env,
		extraMounts:           extraMounts,
		claudeDir:             claudeDir,
		commandRunner:         &defaultCommandRunner{},
		maxPromptDuration:     maxPromptDuration,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// dockerExecutor implements Executor using Docker.
type dockerExecutor struct {
	containerImage        string
	projectName           string
	model                 string
	netrcFile             string
	gitconfigFile         string
	env                   map[string]string
	extraMounts           []config.ExtraMount
	claudeDir             string
	commandRunner         commandRunner
	maxPromptDuration     time.Duration // 0 = disabled
	currentDateTimeGetter libtime.CurrentDateTimeGetter
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

	// Use the configured Claude config dir
	claudeConfigDir := e.claudeDir

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
		"configMount", claudeConfigDir+":/home/node/.claude")

	// Pipe stdout/stderr to both terminal and log file
	cmd.Stdout = io.MultiWriter(os.Stdout, logFileHandle)
	cmd.Stderr = io.MultiWriter(os.Stderr, logFileHandle)

	// Run container and watcher in parallel; whichever finishes first cancels the other.
	if err := run.CancelOnFirstFinish(ctx, e.buildRunFuncs(cmd, logFile, containerName)...); err != nil {
		return errors.Wrap(ctx, err, "docker run failed")
	}

	return nil
}

// buildRunFuncs returns the set of parallel functions for Execute using the configured maxPromptDuration.
func (e *dockerExecutor) buildRunFuncs(
	cmd *exec.Cmd,
	logFile string,
	containerName string,
) []run.Func {
	return e.buildRunFuncsWithTimeout(cmd, logFile, containerName, e.maxPromptDuration)
}

// buildRunFuncsWithTimeout returns the set of parallel functions with an explicit timeout.
// timeout=0 disables the timeout killer.
func (e *dockerExecutor) buildRunFuncsWithTimeout(
	cmd *exec.Cmd,
	logFile string,
	containerName string,
	timeout time.Duration,
) []run.Func {
	getter := e.currentDateTimeGetter
	funcs := []run.Func{
		func(ctx context.Context) error {
			return e.commandRunner.Run(ctx, cmd)
		},
		func(ctx context.Context) error {
			return watchForCompletionReport(
				ctx,
				logFile,
				containerName,
				2*time.Minute,
				10*time.Second,
				e.commandRunner,
				getter,
			)
		},
	}
	if timeout > 0 {
		d := timeout
		funcs = append(funcs, func(ctx context.Context) error {
			return timeoutKiller(ctx, d, containerName, e.commandRunner, getter)
		})
	}
	return funcs
}

// Reattach connects to a running container's output stream and waits for it to exit.
// It does not create a new container. The log file is overwritten from the beginning
// of the container's output (docker logs replays all output from container start).
// maxPromptDuration is the remaining allowed run time; 0 disables the timeout.
func (e *dockerExecutor) Reattach(
	ctx context.Context,
	logFile string,
	containerName string,
	maxPromptDuration time.Duration,
) error {
	logFileHandle, err := prepareLogFile(ctx, logFile)
	if err != nil {
		return errors.Wrap(ctx, err, "prepare log file for reattach")
	}
	defer logFileHandle.Close()

	// docker logs --follow replays all output from container start and blocks until exit
	// #nosec G204 -- containerName is generated internally from prompt filename
	cmd := exec.CommandContext(ctx, "docker", "logs", "--follow", containerName)
	cmd.Stdout = io.MultiWriter(os.Stdout, logFileHandle)
	cmd.Stderr = io.MultiWriter(os.Stderr, logFileHandle)

	slog.Info("reattaching to running container", "containerName", containerName,
		"maxPromptDuration", maxPromptDuration)

	if err := run.CancelOnFirstFinish(ctx, e.buildRunFuncsWithTimeout(cmd, logFile, containerName, maxPromptDuration)...); err != nil {
		return errors.Wrap(ctx, err, "reattach failed")
	}
	return nil
}

// waitUntilDeadline blocks until the deadline is reached or ctx is cancelled.
// tickInterval controls how often the wall-clock is checked.
// Returns true if the deadline was reached, false if ctx was cancelled.
func waitUntilDeadline(
	ctx context.Context,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	deadline time.Time,
	tickInterval time.Duration,
) bool {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if !time.Time(currentDateTimeGetter.Now()).Before(deadline) {
				return true
			}
		}
	}
}

// timeoutKiller waits for the given deadline and then stops the container cleanly.
// Returns nil if ctx is cancelled before the deadline (normal container exit).
// The error message includes the duration so callers can surface it as lastFailReason.
func timeoutKiller(
	ctx context.Context,
	duration time.Duration,
	containerName string,
	runner commandRunner,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	deadline := time.Time(currentDateTimeGetter.Now()).Add(duration)
	if !waitUntilDeadline(ctx, currentDateTimeGetter, deadline, 30*time.Second) {
		return nil // ctx cancelled — normal container exit
	}
	slog.Warn("container exceeded maxPromptDuration, stopping",
		"containerName", containerName,
		"duration", duration)
	// #nosec G204 -- containerName is generated internally from prompt filename
	stopCmd := exec.CommandContext(ctx, "docker", "stop", containerName)
	if err := runner.Run(ctx, stopCmd); err != nil {
		slog.Warn("docker stop failed after timeout, attempting force kill",
			"containerName", containerName, "error", err)
		// #nosec G204 -- containerName is generated internally
		killCmd := exec.CommandContext(ctx, "docker", "kill", containerName)
		if killErr := runner.Run(ctx, killCmd); killErr != nil {
			slog.Error("docker kill also failed after timeout",
				"containerName", containerName, "error", killErr)
		}
	}
	return errors.Errorf(ctx, "prompt timed out after %s", duration)
}

// watchForCompletionReport polls the log file for the completion report marker.
// Once the marker is found, it waits for gracePeriod and then stops the container.
// Returns nil if ctx is cancelled (normal container exit) or after stopping the container.
func watchForCompletionReport(
	ctx context.Context,
	logFile string,
	containerName string,
	gracePeriod time.Duration,
	pollInterval time.Duration,
	runner commandRunner,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// #nosec G304 -- logFile is derived from prompt filename, not user input
			content, err := os.ReadFile(logFile)
			if err != nil {
				slog.Debug("watchForCompletionReport: failed to read log file", "error", err)
				continue
			}
			if strings.Contains(string(content), report.MarkerEnd) {
				slog.Info(
					"stopping stuck container: completion report found but container still running",
					"containerName",
					containerName,
				)
				if !waitUntilDeadline(
					ctx,
					currentDateTimeGetter,
					time.Time(currentDateTimeGetter.Now()).Add(gracePeriod),
					30*time.Second,
				) {
					return nil
				}
				// #nosec G204 -- containerName is derived from prompt filename, not user input
				stopCmd := exec.CommandContext(ctx, "docker", "stop", containerName)
				if err := runner.Run(ctx, stopCmd); err != nil {
					slog.Debug("docker stop", "containerName", containerName, "error", err)
				}
				return nil
			}
		}
	}
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
	// Write prompt to a restricted temp directory to prevent other local processes from reading it
	tmpDir, err := os.MkdirTemp("", "dark-factory-*")
	if err != nil {
		return "", nil, errors.Wrap(ctx, err, "create temp directory")
	}

	promptFile, err := os.CreateTemp(tmpDir, "prompt-*.md")
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, errors.Wrap(ctx, err, "create prompt temp file")
	}

	cleanup := func() {
		_ = promptFile.Close()
		_ = os.RemoveAll(tmpDir)
	}

	if _, err := promptFile.WriteString(promptContent); err != nil {
		cleanup()
		return "", nil, errors.Wrap(ctx, err, "write prompt temp file")
	}

	if err := promptFile.Close(); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, errors.Wrap(ctx, err, "close prompt temp file")
	}

	// Make readable by container user (node/uid 1000) since Docker mounts preserve host permissions
	if err := os.Chmod(tmpDir, 0755); err != nil { // #nosec G302 -- intentional: container user needs read access
		_ = os.RemoveAll(tmpDir)
		return "", nil, errors.Wrap(ctx, err, "chmod temp directory")
	}
	if err := os.Chmod(promptFile.Name(), 0644); err != nil { // #nosec G302 -- intentional: container user needs read access
		_ = os.RemoveAll(tmpDir)
		return "", nil, errors.Wrap(ctx, err, "chmod prompt temp file")
	}

	cleanup = func() {
		_ = os.RemoveAll(tmpDir)
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
	)
	if len(e.env) > 0 {
		keys := make([]string, 0, len(e.env))
		for k := range e.env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, "-e", k+"="+e.env[k])
		}
	}
	args = append(args,
		"-v", promptFilePath+":/tmp/prompt.md:ro",
		"-v", projectRoot+":/workspace",
		"-v", claudeConfigDir+":/home/node/.claude",
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
		args = append(args, "-v", resolved+":/home/node/.gitconfig-extra:ro")
	}
	for _, m := range e.extraMounts {
		src := m.Src
		src = os.ExpandEnv(src)
		if strings.HasPrefix(src, "~/") {
			src = home + src[1:]
		} else if !filepath.IsAbs(src) {
			src = filepath.Join(projectRoot, src)
		}
		if _, err := os.Stat(src); err != nil {
			slog.Warn("extraMounts: src path does not exist, skipping", "src", src, "dst", m.Dst)
			continue
		}
		mount := src + ":" + m.Dst
		if m.IsReadonly() {
			mount += ":ro"
		}
		args = append(args, "-v", mount)
	}
	args = append(args, e.containerImage)
	// #nosec G204 -- promptContent is user-provided by design
	return exec.CommandContext(ctx, "docker", args...)
}

// validateClaudeAuth checks that the Claude config directory contains a valid OAuth token.
// If ANTHROPIC_API_KEY is set, the check is skipped (API key auth does not need OAuth).
// Supports both legacy (.claude.json oauthAccount.accessToken) and current
// (.credentials.json claudeAiOauth.accessToken) token locations.
func validateClaudeAuth(ctx context.Context, configDir string) error {
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

	return errors.Errorf( //nolint:staticcheck // ST1005: intentional capitalization for user-facing message
		ctx,
		"Claude OAuth token missing or expired in %s\n\nFix: Run 'CLAUDE_CONFIG_DIR=%s claude' and use /login",
		configDir,
		configDir,
	)
}

// StopAndRemoveContainer gracefully stops a container (SIGTERM + 10s timeout) then removes it.
func (e *dockerExecutor) StopAndRemoveContainer(ctx context.Context, containerName string) {
	// #nosec G204 -- containerName is generated internally
	stopCmd := exec.CommandContext(ctx, "docker", "stop", containerName)
	if err := e.commandRunner.Run(ctx, stopCmd); err != nil {
		slog.Debug("docker stop", "container", containerName, "error", err)
	}
	e.removeContainerIfExists(ctx, containerName)
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
