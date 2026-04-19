// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflight

import (
	"context"
	"log/slog"
	"os/exec"
	"strings"
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

// cacheEntry stores the result of the last preflight run.
type cacheEntry struct {
	sha       string
	checkedAt time.Time
	ok        bool
	output    string
}

// runnerFn is a function that executes a command and returns its combined output.
type runnerFn func(ctx context.Context) (string, error)

// shaFetcherFn is a function that returns the current HEAD commit SHA.
type shaFetcherFn func(ctx context.Context) (string, error)

// checker implements Checker.
type checker struct {
	command     string
	interval    time.Duration
	projectRoot string
	notifier    notifier.Notifier
	projectName string
	cache       *cacheEntry
	runner      runnerFn
	shaFetcher  shaFetcherFn
}

// NewChecker creates a new preflight Checker.
// command is the shell command to run (empty string disables preflight).
// interval is how long a cached green result is valid for the same git SHA (0 disables caching).
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
	c.shaFetcher = c.getHeadSHA
	return c
}

// Check verifies the project baseline before prompt execution.
func (c *checker) Check(ctx context.Context) (bool, error) {
	if c.command == "" {
		return true, nil
	}

	sha, err := c.shaFetcher(ctx)
	if err != nil {
		slog.Warn("preflight: could not get HEAD SHA, skipping cache", "error", err)
		sha = ""
	}

	// Cache hit: same SHA and within interval
	if c.cache != nil && sha != "" && c.cache.sha == sha &&
		c.interval > 0 && time.Since(c.cache.checkedAt) < c.interval {
		slog.Debug("preflight: cache hit", "sha", sha[:minLen(sha, 12)], "ok", c.cache.ok)
		return c.cache.ok, nil
	}

	slog.Info("preflight: running baseline check", "command", c.command, "sha", truncateSHA(sha))

	output, runErr := c.runner(ctx)
	ok := runErr == nil

	c.cache = &cacheEntry{
		sha:       sha,
		checkedAt: time.Now(),
		ok:        ok,
		output:    output,
	}

	if ok {
		slog.Info("preflight: baseline check passed", "sha", truncateSHA(sha))
		return true, nil
	}

	slog.Error("preflight: baseline check FAILED — prompts will not start until baseline is fixed",
		"command", c.command,
		"sha", truncateSHA(sha),
		"output", output,
		"error", runErr,
	)
	_ = c.notifier.Notify(ctx, notifier.Event{
		ProjectName: c.projectName,
		EventType:   "preflight_failed",
	})
	return false, nil
}

// getHeadSHA returns the current HEAD commit SHA using git rev-parse.
func (c *checker) getHeadSHA(ctx context.Context) (string, error) {
	// #nosec G204 -- fixed args; projectRoot is from trusted config
	cmd := exec.CommandContext(ctx, "git", "-C", c.projectRoot, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", errors.Wrap(ctx, err, "git rev-parse HEAD")
	}
	return strings.TrimSpace(string(output)), nil
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

// truncateSHA returns the first 12 characters of sha for logging, or the full sha if shorter.
func truncateSHA(sha string) string {
	return sha[:minLen(sha, 12)]
}

// minLen returns the minimum of len(s) and n.
func minLen(s string, n int) int {
	if len(s) < n {
		return len(s)
	}
	return n
}
