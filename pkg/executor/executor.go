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
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/bborbe/errors"
	"github.com/bborbe/run"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/formatter"
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
	fmtr formatter.Formatter,
	hideGit bool,
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
		formatter:             fmtr,
		hideGit:               hideGit,
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
	formatter             formatter.Formatter
	hideGit               bool // mask /workspace/.git from the container
}

// Execute runs the claude-yolo Docker container with the given prompt content.
// It blocks until the container exits and returns an error if the exit code is non-zero.
// Two log files are written: a raw JSONL file (container stdout verbatim) and
// a human-readable formatted log file.
func (e *dockerExecutor) Execute(
	ctx context.Context,
	promptContent string,
	logFile string,
	containerName string,
) error {
	projectRoot, err := os.Getwd()
	if err != nil {
		return errors.Wrap(ctx, err, "get working directory")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return errors.Wrap(ctx, err, "get home directory")
	}
	logFileHandle, err := prepareLogFile(ctx, logFile)
	if err != nil {
		return errors.Wrap(ctx, err, "prepare log file")
	}
	defer logFileHandle.Close()
	rawFileHandle, err := prepareRawLogFile(ctx, rawLogPath(logFile))
	if err != nil {
		return err // error already names the raw log path
	}
	defer rawFileHandle.Close()
	promptFilePath, cleanup, err := createPromptTempFile(ctx, promptContent)
	if err != nil {
		return errors.Wrap(ctx, err, "create prompt temp file")
	}
	defer cleanup()
	slog.Debug("prompt prepared for execution",
		"contentSize", len(promptContent), "tempFile", promptFilePath)
	e.removeContainerIfExists(ctx, containerName)
	promptBaseName := extractPromptBaseName(containerName, e.projectName)
	claudeConfigDir := e.claudeDir
	if err := validateClaudeAuth(ctx, claudeConfigDir); err != nil {
		return err
	}
	cmd := e.buildDockerCommand(
		ctx, containerName, promptFilePath, projectRoot, claudeConfigDir, promptBaseName, home)
	slog.Debug("docker command prepared",
		"image", e.containerImage, "containerName", containerName,
		"workspaceMount", projectRoot+":/workspace",
		"configMount", claudeConfigDir+":/home/node/.claude")
	if runErr := e.runWithFormatterPipeline(
		ctx, cmd, rawFileHandle, logFileHandle,
		e.buildRunFuncs(cmd, logFile, containerName), "formatter error",
	); runErr != nil {
		return errors.Wrap(ctx, runErr, "docker run failed")
	}
	return nil
}

// runWithFormatterPipeline wires cmd.Stdout through the formatter pipeline, runs the provided
// run.Funcs via run.CancelOnFirstFinish, and waits for the formatter goroutine to finish.
// cmd.Stderr is connected to both os.Stderr and logWriter.
// pw is closed via wrapFirstFuncWithPipeClose when the first run.Func returns, signalling EOF.
func (e *dockerExecutor) runWithFormatterPipeline(
	ctx context.Context,
	cmd *exec.Cmd,
	rawWriter io.Writer,
	logWriter io.Writer,
	runFuncs []run.Func,
	fmtErrMsg string,
) error {
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = io.MultiWriter(os.Stderr, logWriter)
	fmtDone := make(chan error, 1)
	go func() {
		fmtDone <- e.formatter.ProcessStream(ctx, pr, rawWriter, io.MultiWriter(os.Stdout, logWriter))
	}()
	runErr := run.CancelOnFirstFinish(ctx, wrapFirstFuncWithPipeClose(runFuncs, pw)...)
	if fmtErr := <-fmtDone; fmtErr != nil {
		slog.Warn(fmtErrMsg, "error", fmtErr)
	}
	return runErr
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
// Two log files are written: a raw JSONL file (container stdout verbatim) and
// a human-readable formatted log file.
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
	rawFileHandle, err := prepareRawLogFile(ctx, rawLogPath(logFile))
	if err != nil {
		return err // error already names the raw log path
	}
	defer rawFileHandle.Close()
	// docker logs --follow replays all output from container start and blocks until exit
	// #nosec G204 -- containerName is generated internally from prompt filename
	cmd := exec.CommandContext(ctx, "docker", "logs", "--follow", containerName)
	slog.Info("reattaching to running container", "containerName", containerName,
		"maxPromptDuration", maxPromptDuration)
	if runErr := e.runWithFormatterPipeline(
		ctx, cmd, rawFileHandle, logFileHandle,
		e.buildRunFuncsWithTimeout(cmd, logFile, containerName, maxPromptDuration),
		"formatter error on reattach",
	); runErr != nil {
		return errors.Wrap(ctx, runErr, "reattach failed")
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

// rawLogPath returns the raw JSONL log path corresponding to the given formatted log path.
// Example: "prompts/log/042.log" → "prompts/log/042.jsonl"
func rawLogPath(logFile string) string {
	ext := filepath.Ext(logFile)
	if ext == "" {
		return logFile + ".jsonl"
	}
	return strings.TrimSuffix(logFile, ext) + ".jsonl"
}

// prepareRawLogFile opens the raw JSONL log file for writing.
// The raw log path is the formatted log path with the extension replaced by ".jsonl".
// Returns a non-nil error naming the raw log path if the file cannot be opened.
func prepareRawLogFile(ctx context.Context, rawLogFile string) (*os.File, error) {
	logDir := filepath.Dir(rawLogFile)
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return nil, errors.Wrapf(ctx, err, "create log directory for raw log %s", rawLogFile)
	}
	// #nosec G304 -- rawLogFile is derived from prompt filename, not user input
	f, err := os.OpenFile(rawLogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "open raw log file %s", rawLogFile)
	}
	return f, nil
}

// wrapFirstFuncWithPipeClose wraps the first run.Func in fns so that pw is closed
// when that function returns (regardless of error). The remaining functions are returned unchanged.
// The first function is always the commandRunner.Run wrapper from buildRunFuncsWithTimeout.
// pw.Close() signals EOF to the formatter goroutine on both success and cancellation paths —
// so <-fmtDone will not hang when run.CancelOnFirstFinish cancels the runner mid-stream.
func wrapFirstFuncWithPipeClose(fns []run.Func, pw *io.PipeWriter) []run.Func {
	if len(fns) == 0 {
		return fns
	}
	wrapped := make([]run.Func, len(fns))
	copy(wrapped, fns)
	original := fns[0]
	wrapped[0] = func(ctx context.Context) error {
		defer pw.Close()
		return original(ctx)
	}
	return wrapped
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
		"-e", "YOLO_OUTPUT=json",
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
		src = resolveExtraMountSrc(src, os.Getenv, runtime.GOOS)
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
	args = append(args, e.buildHideGitArgs(projectRoot)...)
	args = append(args, e.containerImage)
	// #nosec G204 -- promptContent is user-provided by design
	return exec.CommandContext(ctx, "docker", args...)
}

// buildHideGitArgs returns the Docker mount args needed to mask .git from the container.
// When hideGit is false it returns nil (no args added). When true it inspects projectRoot/.git:
//   - directory → tmpfs overlay hides host .git contents (anonymous volumes don't mask bind-mount subdirs)
//   - file (worktree pointer) → /dev/null bind ("-v /dev/null:/workspace/.git")
//   - missing → no args (nothing to hide)
//   - stat error (non-ENOENT) → no args, logged at debug
func (e *dockerExecutor) buildHideGitArgs(projectRoot string) []string {
	if !e.hideGit {
		return nil
	}
	gitPath := filepath.Join(projectRoot, ".git")
	fi, statErr := os.Stat(gitPath)
	if statErr != nil {
		if !os.IsNotExist(statErr) {
			slog.Debug("hideGit: stat .git failed, skipping mount mask", "error", statErr)
		}
		return nil
	}
	if fi.IsDir() {
		// Normal repo or clone: use tmpfs to overlay the .git directory.
		// Anonymous volumes (-v /workspace/.git) cannot mask subdirectories of bind mounts.
		return []string{"--tmpfs", "/workspace/.git:rw,size=1k"}
	}
	// Worktree pointer file or submodule: bind /dev/null over the pointer file.
	return []string{"-v", "/dev/null:/workspace/.git"}
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

// resolveExtraMountSrc expands env vars in src using lookupEnv, with a
// platform-appropriate default for HOST_CACHE_DIR when lookupEnv returns
// empty for it. Pure function: no globals, no os.Getenv, no os.Setenv.
//
// goos is runtime.GOOS at call site.
// Defaults for HOST_CACHE_DIR:
//
//	darwin           → $HOME/Library/Caches
//	other + XDG set  → $XDG_CACHE_HOME
//	other + no XDG   → $HOME/.cache
//
// If HOME is empty and no fallback is possible, returns empty for that var.
func resolveExtraMountSrc(src string, lookupEnv func(string) string, goos string) string {
	mapper := func(name string) string {
		if name == "HOST_CACHE_DIR" {
			return resolveHostCacheDir(lookupEnv, goos)
		}
		return lookupEnv(name)
	}
	return os.Expand(src, mapper)
}

// resolveHostCacheDir returns the value for HOST_CACHE_DIR using lookupEnv and goos.
// If the variable is explicitly set, that value is used. Otherwise a platform default is returned.
func resolveHostCacheDir(lookupEnv func(string) string, goos string) string {
	if v := lookupEnv("HOST_CACHE_DIR"); v != "" {
		return v
	}
	home := lookupEnv("HOME")
	if goos == "darwin" {
		return darwinCacheDir(home)
	}
	return linuxCacheDir(lookupEnv, home)
}

// darwinCacheDir returns the macOS user cache directory for the given home path.
func darwinCacheDir(home string) string {
	if home == "" {
		return ""
	}
	return home + "/Library/Caches"
}

// linuxCacheDir returns the Linux/other user cache directory using XDG_CACHE_HOME or home fallback.
func linuxCacheDir(lookupEnv func(string) string, home string) string {
	if xdg := lookupEnv("XDG_CACHE_HOME"); xdg != "" {
		return xdg
	}
	if home == "" {
		return ""
	}
	return home + "/.cache"
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
