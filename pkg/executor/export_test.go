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
	"github.com/bborbe/dark-factory/pkg/formatter"
	"github.com/bborbe/dark-factory/pkg/launchpolicy"
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
	fmtr formatter.Formatter,
	hideGit bool,
) Executor {
	// projectRoot defaults to "" in the test helper; tests that depend on it
	// already pass their own ProjectRoot via the buildCmd helper, which calls
	// BuildDockerCommandForTest with explicit projectRoot. Here we only
	// construct the executor instance — the policy's projectRoot field is not
	// dereferenced until Execute is called, which the runner-injection tests
	// drive directly.
	policy := launchpolicy.NewPolicy(
		containerImage,
		projectName,
		"", // projectRoot — set per-call in tests via the executor's Execute path
		claudeDir,
		"", // home
		env,
		extraMounts,
		netrcFile,
		gitconfigFile,
		hideGit,
	)
	return &dockerExecutor{
		policy:                policy,
		model:                 model,
		commandRunner:         runner,
		maxPromptDuration:     maxPromptDuration,
		currentDateTimeGetter: currentDateTimeGetter,
		formatter:             fmtr,
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
	hideGit bool,
) *exec.Cmd {
	policy := launchpolicy.NewPolicy(
		containerImage,
		projectName,
		projectRoot,
		claudeConfigDir,
		home,
		env,
		extraMounts,
		netrcFile,
		gitconfigFile,
		hideGit,
	)
	e := &dockerExecutor{
		policy: policy,
		model:  model,
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

// BuildDockerCommandFromPolicyForTest is the policy-injection test helper used
// by the spec-098 regression-lock test. It accepts a Policy directly so a
// test can construct a Policy with a synthetic capability set via
// Policy.WithCapAddForTest and prove the cap propagates to the executor's
// argv. Production callers MUST use NewDockerExecutor; this helper is for
// tests only.
func BuildDockerCommandFromPolicyForTest(
	ctx context.Context,
	policy launchpolicy.Policy,
	model string,
	containerName string,
	promptFilePath string,
	projectRoot string,
	claudeConfigDir string,
	promptBaseName string,
	home string,
) *exec.Cmd {
	e := &dockerExecutor{
		policy: policy,
		model:  model,
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

// PrepareRawLogFileForTest exposes prepareRawLogFile for external test packages.
func PrepareRawLogFileForTest(ctx context.Context, rawLogFile string) (*os.File, error) {
	return prepareRawLogFile(ctx, rawLogFile)
}

// RawLogPathForTest exposes rawLogPath for external test packages.
func RawLogPathForTest(logFile string) string {
	return rawLogPath(logFile)
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
func ValidateClaudeAuthForTest(ctx context.Context, configDir string, env map[string]string) error {
	return validateClaudeAuth(ctx, configDir, env)
}

// ResolveExtraMountSrcForTest exposes resolveExtraMountSrc for external test packages.
func ResolveExtraMountSrcForTest(src string, lookupEnv func(string) string, goos string) string {
	return resolveExtraMountSrc(src, lookupEnv, goos)
}
