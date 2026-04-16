// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"os"
	"os/exec"
	"time"

	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/config"
)

// CommandRunnerForTest is an exported alias for the unexported commandRunner interface.
type CommandRunnerForTest = commandRunner

// NewDefaultCommandRunnerForTest exposes defaultCommandRunner for external test packages.
func NewDefaultCommandRunnerForTest() CommandRunnerForTest {
	return &defaultCommandRunner{}
}

// NewDockerExecutorWithRunnerForTest creates a dockerExecutor with an injected commandRunner for testing.
func NewDockerExecutorWithRunnerForTest(
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
	runner CommandRunnerForTest,
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
		commandRunner:         runner,
		maxPromptDuration:     maxPromptDuration,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// BuildDockerCommandForTest exposes buildDockerCommand for external test packages.
func BuildDockerCommandForTest(
	ctx context.Context,
	containerImage string,
	projectName string,
	model string,
	netrcFile string,
	gitconfigFile string,
	env map[string]string,
	extraMounts []config.ExtraMount,
	containerName string,
	promptFilePath string,
	projectRoot string,
	claudeConfigDir string,
	promptBaseName string,
	home string,
) *exec.Cmd {
	e := &dockerExecutor{
		containerImage: containerImage,
		projectName:    projectName,
		model:          model,
		netrcFile:      netrcFile,
		gitconfigFile:  gitconfigFile,
		env:            env,
		extraMounts:    extraMounts,
	}
	return e.buildDockerCommand(
		ctx,
		containerName,
		promptFilePath,
		projectRoot,
		claudeConfigDir,
		promptBaseName,
		home,
	)
}

// BuildDockerCommandWithWorktreeModeForTest exposes buildDockerCommand with worktreeMode for external test packages.
func BuildDockerCommandWithWorktreeModeForTest(
	ctx context.Context,
	containerImage string,
	projectName string,
	model string,
	netrcFile string,
	gitconfigFile string,
	env map[string]string,
	extraMounts []config.ExtraMount,
	containerName string,
	promptFilePath string,
	projectRoot string,
	claudeConfigDir string,
	promptBaseName string,
	home string,
	worktreeMode bool,
) *exec.Cmd {
	e := &dockerExecutor{
		containerImage: containerImage,
		projectName:    projectName,
		model:          model,
		netrcFile:      netrcFile,
		gitconfigFile:  gitconfigFile,
		env:            env,
		extraMounts:    extraMounts,
		hideGitDir:     worktreeMode,
	}
	return e.buildDockerCommand(
		ctx,
		containerName,
		promptFilePath,
		projectRoot,
		claudeConfigDir,
		promptBaseName,
		home,
	)
}

// PrepareLogFileForTest exposes prepareLogFile for external test packages.
func PrepareLogFileForTest(ctx context.Context, logFile string) (*os.File, error) {
	return prepareLogFile(ctx, logFile)
}

// CreatePromptTempFileForTest exposes createPromptTempFile for external test packages.
func CreatePromptTempFileForTest(
	ctx context.Context,
	promptContent string,
) (string, func(), error) {
	return createPromptTempFile(ctx, promptContent)
}

// ExtractPromptBaseNameForTest exposes extractPromptBaseName for external test packages.
func ExtractPromptBaseNameForTest(containerName string, projectName string) string {
	return extractPromptBaseName(containerName, projectName)
}

// WatchForCompletionReportForTest exposes watchForCompletionReport for external test packages.
func WatchForCompletionReportForTest(
	ctx context.Context,
	logFile string,
	containerName string,
	gracePeriod time.Duration,
	pollInterval time.Duration,
	runner CommandRunnerForTest,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	return watchForCompletionReport(
		ctx,
		logFile,
		containerName,
		gracePeriod,
		pollInterval,
		runner,
		currentDateTimeGetter,
	)
}

// TimeoutKillerForTest exposes timeoutKiller for external test packages.
func TimeoutKillerForTest(
	ctx context.Context,
	duration time.Duration,
	containerName string,
	runner CommandRunnerForTest,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	return timeoutKiller(ctx, duration, containerName, runner, currentDateTimeGetter)
}

// WaitUntilDeadlineForTest exposes waitUntilDeadline for external test packages.
func WaitUntilDeadlineForTest(
	ctx context.Context,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	deadline time.Time,
	tickInterval time.Duration,
) bool {
	return waitUntilDeadline(ctx, currentDateTimeGetter, deadline, tickInterval)
}

// ValidateClaudeAuthForTest exposes validateClaudeAuth for external test packages.
func ValidateClaudeAuthForTest(ctx context.Context, configDir string) error {
	return validateClaudeAuth(ctx, configDir)
}

// ResolveExtraMountSrcForTest exposes resolveExtraMountSrc for external test packages.
func ResolveExtraMountSrcForTest(src string, lookupEnv func(string) string, goos string) string {
	return resolveExtraMountSrc(src, lookupEnv, goos)
}
