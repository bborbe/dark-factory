// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflight

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/config"
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
	command        string
	interval       time.Duration
	projectRoot    string
	containerImage string
	extraMounts    []config.ExtraMount
	notifier       notifier.Notifier
	projectName    string
	cache          *cacheEntry
	runner         runnerFn
	shaFetcher     shaFetcherFn
}

// NewChecker creates a new preflight Checker.
// command is the shell command to run (empty string disables preflight).
// interval is how long a cached green result is valid for the same git SHA (0 disables caching).
// projectRoot is the absolute path of the project directory mounted as /workspace.
// containerImage is the Docker image to use (same as the YOLO executor).
// extraMounts are additional volume mounts applied to the preflight container.
// n is used to notify humans when the baseline is broken.
func NewChecker(
	command string,
	interval time.Duration,
	projectRoot string,
	containerImage string,
	extraMounts []config.ExtraMount,
	n notifier.Notifier,
	projectName string,
) Checker {
	c := &checker{
		command:        command,
		interval:       interval,
		projectRoot:    projectRoot,
		containerImage: containerImage,
		extraMounts:    extraMounts,
		notifier:       n,
		projectName:    projectName,
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

// buildPreflightDockerArgs constructs the docker run argument list for the preflight container.
// Pure function: no I/O, no globals — all inputs are parameters.
func buildPreflightDockerArgs(
	projectRoot string,
	containerImage string,
	command string,
	extraMounts []config.ExtraMount,
	home string,
	lookupEnv func(string) string,
	goos string,
) []string {
	args := []string{
		"run", "--rm",
		"-v", projectRoot + ":/workspace",
		"-w", "/workspace",
	}

	for _, m := range extraMounts {
		src := resolveExtraMountSrc(m.Src, lookupEnv, goos)
		if strings.HasPrefix(src, "~/") {
			src = home + src[1:]
		} else if !filepath.IsAbs(src) {
			src = filepath.Join(projectRoot, src)
		}
		if _, statErr := os.Stat(src); statErr != nil {
			slog.Warn(
				"preflight: extraMounts src does not exist, skipping",
				"src",
				src,
				"dst",
				m.Dst,
			)
			continue
		}
		mount := src + ":" + m.Dst
		if m.IsReadonly() {
			mount += ":ro"
		}
		args = append(args, "-v", mount)
	}

	args = append(args, containerImage)
	args = append(args, "sh", "-c", command)
	return args
}

// resolveExtraMountSrc expands env vars in src using lookupEnv, with a
// platform-appropriate default for HOST_CACHE_DIR when lookupEnv returns
// empty for it. Copied from pkg/executor/executor.go — kept private to avoid coupling.
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
