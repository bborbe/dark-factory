// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	stderrors "errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/bborbe/errors"
	"github.com/bborbe/run"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/formatter"
	log "github.com/bborbe/dark-factory/pkg/log"
)

// ErrClaudeNotFound signals that the `claude` binary is not on PATH.
// backend: local requires claude in the environment; the local executor
// NEVER falls back to docker.
var ErrClaudeNotFound = stderrors.New("claude not found on PATH")

// ErrReattachUnsupported signals that the local backend cannot reattach to a
// prior execution: a local subprocess dies with the dark-factory process, so
// there is nothing to reattach to. The caller must recover by re-running the
// prompt (safe because execution commits per prompt).
var ErrReattachUnsupported = stderrors.New("reattach unsupported for local backend")

// localSubprocessExecutor implements Executor by running claude as a local
// subprocess in the current working directory (already the checked-out repo).
// No docker run, no bind mounts. Intended ONLY for already-isolated callers
// (see docs/execution-backends.md) — it runs claude with the full credentials
// and filesystem of the dark-factory process.
type localSubprocessExecutor struct {
	model                 string
	maxPromptDuration     time.Duration
	currentDateTimeGetter libtime.CurrentDateTimeGetter
	formatter             formatter.Formatter
	commandRunner         commandRunner

	mu         sync.Mutex
	runningCmd *exec.Cmd
}

// NewLocalSubprocessExecutor creates an Executor that runs claude locally.
func NewLocalSubprocessExecutor(
	model string,
	maxPromptDuration time.Duration,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	fmtr formatter.Formatter,
) Executor {
	return &localSubprocessExecutor{
		model:                 model,
		maxPromptDuration:     maxPromptDuration,
		currentDateTimeGetter: currentDateTimeGetter,
		formatter:             fmtr,
		commandRunner:         &defaultCommandRunner{},
	}
}

// Execute runs the claude binary as a local subprocess in the current working directory.
// It blocks until the subprocess exits and returns an error if the exit code is non-zero.
// Two log files are written: a raw JSONL file (stdout verbatim) and
// a human-readable formatted log file.
func (e *localSubprocessExecutor) Execute(
	ctx context.Context,
	promptContent string,
	logFile string,
	executionID string,
) error {
	// Fail fast if claude is not on PATH — before creating any log/temp files.
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return errors.Wrapf(
			ctx,
			ErrClaudeNotFound,
			"backend: local requires claude in the environment; %v",
			err,
		)
	}

	logFileHandle, err := prepareLogFile(ctx, logFile)
	if err != nil {
		return errors.Wrap(ctx, err, "prepare log file")
	}
	defer logFileHandle.Close()
	rawFileHandle, err := prepareRawLogFile(ctx, rawLogPath(logFile))
	if err != nil {
		return errors.Wrap(ctx, err, "prepare raw log file")
	}
	defer rawFileHandle.Close()

	promptFilePath, cleanup, err := createPromptTempFile(ctx, promptContent)
	if err != nil {
		return errors.Wrap(ctx, err, "create prompt temp file")
	}
	defer cleanup()

	log.From(ctx).Debug("local claude execution prepared",
		"model", e.model,
		"prompt_file", promptFilePath,
		"execution_id", executionID,
	)

	cmd := e.buildCommand(ctx, claudePath, promptFilePath)

	if runErr := e.runWithFormatterPipeline(
		ctx, cmd, rawFileHandle, logFileHandle,
		e.buildRunFuncs(cmd),
		"local claude run failed",
	); runErr != nil {
		return errors.Wrap(ctx, runErr, "local claude run failed")
	}
	return nil
}

// buildCommand builds the exec.Cmd for a local claude run.
func (e *localSubprocessExecutor) buildCommand(
	ctx context.Context,
	claudePath, promptFilePath string,
) *exec.Cmd {
	args := []string{
		"--dangerously-skip-permissions",
		"--output-format", "stream-json",
		"--verbose",
		"--print",
	}
	if e.model != "" {
		args = append([]string{"--model", e.model}, args...)
	}
	// #nosec G204 -- claudePath is resolved from PATH via exec.LookPath; args are static flags from config, not user input
	cmd := exec.CommandContext(ctx, claudePath, args...)
	cmd.Dir = "" // inherit cwd (already the checked-out repo)

	// Set process group so we can kill the whole group on stop.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	promptFile, err := os.Open(
		promptFilePath,
	) // #nosec G304 -- path from createPromptTempFile, not user input
	if err != nil {
		// Caller will wrap and return this error; cmd is still valid for inspection.
		// Set cmd.ProcessState so callers that guard on nil process don't panic.
		cmd.ProcessState = &os.ProcessState{}
		return cmd
	}
	cmd.Stdin = promptFile
	// Do NOT close promptFile here — cmd.Stdin pipe is closed by Go's exec.Cmd
	// when the process exits (cmd.Wait() drains and closes it). The temp file
	// cleanup (promoteFile cleanup) runs via deferred cleanup() in Execute.
	return cmd
}

// buildRunFuncs returns the run.Funcs for Execute, including an optional timeout killer.
func (e *localSubprocessExecutor) buildRunFuncs(cmd *exec.Cmd) []run.Func {
	funcs := []run.Func{func(ctx context.Context) error {
		e.mu.Lock()
		e.runningCmd = cmd
		e.mu.Unlock()
		defer func() {
			e.mu.Lock()
			e.runningCmd = nil
			e.mu.Unlock()
		}()
		return e.commandRunner.Run(ctx, cmd)
	}}
	if e.maxPromptDuration > 0 {
		d := e.maxPromptDuration
		funcs = append(funcs, func(ctx context.Context) error {
			deadline := time.Time(e.currentDateTimeGetter.Now()).Add(d)
			if !waitUntilDeadline(ctx, e.currentDateTimeGetter, deadline, 100*time.Millisecond) {
				return nil // ctx cancelled — normal subprocess exit
			}
			log.From(ctx).Warn("local subprocess exceeded maxPromptDuration, stopping",
				"duration", d)
			e.stopProcessGroup(cmd)
			return errors.Errorf(ctx, "prompt timed out after %s", d)
		})
	}
	return funcs
}

// runWithFormatterPipeline wires cmd.Stdout through the formatter pipeline, runs the provided
// run.Funcs via run.CancelOnFirstFinish, and waits for the formatter goroutine to finish.
// cmd.Stderr is connected to both os.Stderr and logWriter.
// pw is closed via wrapFirstFuncWithPipeClose when the first run.Func returns, signalling EOF.
func (e *localSubprocessExecutor) runWithFormatterPipeline(
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
		log.From(ctx).Warn(fmtErrMsg, "error", fmtErr)
	}
	return runErr
}

// localStopGracePeriod is how long stopProcessGroup waits after SIGTERM before
// escalating to SIGKILL.
const localStopGracePeriod = 10 * time.Second

// stopProcessGroup sends SIGTERM to the child's process group, then polls for the
// group to disappear (up to localStopGracePeriod) before escalating to SIGKILL.
// It does NOT call cmd.Wait()/Process.Wait() — the commandRunner owns reaping via
// cmd.Wait(); a concurrent Wait here would race (os/exec forbids it). Group
// liveness is probed with signal 0 (ESRCH => the group is gone).
func (e *localSubprocessExecutor) stopProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pgid := cmd.Process.Pid
	// Negative PID targets the whole process group.
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	deadline := time.Now().Add(localStopGracePeriod)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-pgid, 0); err != nil {
			return // group gone (ESRCH) — SIGTERM was sufficient
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}

// Reattach returns ErrReattachUnsupported because a local subprocess dies with the
// dark-factory process — there is nothing to reattach to.
func (e *localSubprocessExecutor) Reattach(
	ctx context.Context,
	logFile string,
	executionID string,
	maxPromptDuration time.Duration,
) error {
	return errors.Wrap(ctx, ErrReattachUnsupported, "local backend does not support reattach")
}

// StopAndRemoveContainer terminates the running subprocess and its children by sending
// SIGTERM to the process group, then SIGKILL after a grace period.
func (e *localSubprocessExecutor) StopAndRemoveContainer(ctx context.Context, executionID string) {
	e.mu.Lock()
	cmd := e.runningCmd
	e.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	log.From(ctx).Debug("stopping local subprocess", "execution_id", executionID)
	e.stopProcessGroup(cmd)
}

// localSubprocessExecutionChecker implements ExecutionChecker for the local backend.
type localSubprocessExecutionChecker struct {
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewLocalSubprocessExecutionChecker creates an ExecutionChecker for the local backend.
// A local subprocess never survives a dark-factory restart and is not inspectable
// across restarts, so IsRunning always returns false and WaitUntilRunning returns nil
// immediately (the Execute call blocks in-process, so "wait until running" is trivially satisfied).
func NewLocalSubprocessExecutionChecker(
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) ExecutionChecker {
	return &localSubprocessExecutionChecker{currentDateTimeGetter: currentDateTimeGetter}
}

// IsRunning always returns false because a local subprocess is not recoverable
// after a dark-factory restart — it dies with the process.
func (c *localSubprocessExecutionChecker) IsRunning(
	ctx context.Context,
	executionID string,
) (bool, error) {
	return false, nil
}

// WaitUntilRunning returns nil immediately because the local Execute call blocks
// in-process: by the time Execute returns, the subprocess has already run to
// completion or been stopped. There is no background container to wait for.
func (c *localSubprocessExecutionChecker) WaitUntilRunning(
	ctx context.Context,
	executionID string,
	timeout time.Duration,
) error {
	return nil
}

// localSubprocessExecutionStopper implements ExecutionStopper for the local backend.
type localSubprocessExecutionStopper struct{}

// NewLocalSubprocessExecutionStopper creates an ExecutionStopper for the local backend.
// Because the local backend's cancellation is driven through the executor's
// StopAndRemoveContainer (which holds the child process handle via mutex), a
// standalone stopper keyed only by executionID cannot reach the child. This is
// a no-op; local cancellation flows through context cancellation or
// StopAndRemoveContainer instead.
func NewLocalSubprocessExecutionStopper() ExecutionStopper {
	return &localSubprocessExecutionStopper{}
}

// StopContainer is a no-op for the local backend. Cancellation flows through
// the executor's StopAndRemoveContainer / context cancellation instead.
func (s *localSubprocessExecutionStopper) StopContainer(
	ctx context.Context,
	executionID string,
) error {
	return nil
}
