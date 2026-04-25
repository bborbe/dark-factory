// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflight

import (
	"context"
	"log/slog"
	"os/exec"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/notifier"
)

//counterfeiter:generate -o ../../mocks/preflight-checker.go --fake-name PreflightChecker . Checker

// Checker verifies the project baseline before each prompt execution.
type Checker interface {
	// Check returns true when the baseline is green and the prompt may proceed.
	// Returns false when the baseline is broken or preflight is disabled (empty command).
	// Docker execution errors and non-zero exit codes are both treated as "broken baseline":
	// they are logged and cause false to be returned, never propagated as Go errors.
	Check(ctx context.Context) (bool, error)
}

// cacheEntry stores the result of the last successful preflight run.
type cacheEntry struct {
	checkedAt time.Time
}

// runnerFn is a function that executes a command and returns its combined output.
type runnerFn func(ctx context.Context) (string, error)

// checker implements Checker.
type checker struct {
	command     string
	interval    time.Duration
	projectRoot string
	notifier    notifier.Notifier
	projectName string
	cache       *cacheEntry
	runner      runnerFn
}

// NewChecker creates a new preflight Checker.
// command is the shell command to run (empty string disables preflight).
// interval is how long a cached green result is valid (0 disables caching).
// projectRoot is the absolute path of the project directory.
// n is used to notify humans when the baseline is broken.
// projectName is the project identifier used in notifications.
func NewChecker(
	command string,
	interval time.Duration,
	projectRoot string,
	n notifier.Notifier,
	projectName string,
) Checker {
	c := &checker{
		command:     command,
		interval:    interval,
		projectRoot: projectRoot,
		notifier:    n,
		projectName: projectName,
	}
	c.runner = c.runInContainer
	return c
}

// Check verifies the project baseline before prompt execution.
func (c *checker) Check(ctx context.Context) (bool, error) {
	if c.command == "" {
		return true, nil
	}

	// Cache hit: a successful preflight is reused for `interval` after it ran,
	// regardless of git activity. Failed results are not cached — operator fixes
	// must be picked up on the next Check call.
	if c.cache != nil && c.interval > 0 &&
		time.Since(c.cache.checkedAt) < c.interval {
		slog.Debug("preflight: cache hit (time-based)",
			"age", time.Since(c.cache.checkedAt).Round(time.Second),
			"interval", c.interval,
		)
		return true, nil
	}

	slog.Info("preflight: running baseline check", "command", c.command)

	output, runErr := c.runner(ctx)
	ok := runErr == nil

	if ok {
		c.cache = &cacheEntry{
			checkedAt: time.Now(),
		}
		slog.Info("preflight: baseline check passed")
		return true, nil
	}

	// Failure: do not cache — operator may fix the issue between calls,
	// and we want the next Check to re-run the command.
	slog.Error("preflight: baseline check FAILED — prompts will not start until baseline is fixed",
		"command", c.command,
		"output", output,
		"error", runErr,
	)
	_ = c.notifier.Notify(ctx, notifier.Event{
		ProjectName: c.projectName,
		EventType:   "preflight_failed",
	})
	return false, nil
}

// runInContainer executes the preflight command on the host (NOT a container).
// The name is retained for backwards compatibility; `make precommit`-style baseline
// checks are safe to run on host — containerization is only needed to sandbox Claude
// with --dangerously-skip-permissions, which preflight does not use.
// Returns combined stdout+stderr output and nil on success, or output + error on failure.
func (c *checker) runInContainer(ctx context.Context) (string, error) {
	// #nosec G204 -- command is from trusted project config (.dark-factory.yaml)
	cmd := exec.CommandContext(ctx, "sh", "-c", c.command)
	cmd.Dir = c.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), errors.Wrap(ctx, err, "preflight command exited non-zero")
	}
	return string(output), nil
}
