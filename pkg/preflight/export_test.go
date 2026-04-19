// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflight

import (
	"context"
	"time"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/notifier"
)

// Exported wrappers for pure helpers — used in unit tests.
var (
	ResolveExtraMountSrc     = resolveExtraMountSrc
	ResolveHostCacheDir      = resolveHostCacheDir
	DarwinCacheDir           = darwinCacheDir
	LinuxCacheDir            = linuxCacheDir
	TruncateSHA              = truncateSHA
	MinLen                   = minLen
	BuildPreflightDockerArgs = buildPreflightDockerArgs
)

// NewCheckerWithRunner creates a Checker for testing, replacing runInContainer with a fake runner.
// headSHA is returned by the fake SHA fetcher. runner replaces the Docker container execution.
func NewCheckerWithRunner(
	command string,
	interval time.Duration,
	n notifier.Notifier,
	projectName string,
	headSHA string,
	runner func(ctx context.Context) (string, error),
) Checker {
	c := &checker{
		command:     command,
		interval:    interval,
		notifier:    n,
		projectName: projectName,
	}
	c.shaFetcher = func(_ context.Context) (string, error) {
		return headSHA, nil
	}
	c.runner = runner
	return c
}

// NewCheckerWithSHAError creates a Checker for testing where the SHA fetcher returns an error.
func NewCheckerWithSHAError(
	command string,
	interval time.Duration,
	n notifier.Notifier,
	projectName string,
	shaErr error,
	runner func(ctx context.Context) (string, error),
) Checker {
	c := &checker{
		command:     command,
		interval:    interval,
		notifier:    n,
		projectName: projectName,
	}
	c.shaFetcher = func(_ context.Context) (string, error) {
		return "", shaErr
	}
	c.runner = runner
	return c
}

// NewCheckerWithExtraMounts creates a Checker for testing buildPreflightDockerArgs with extra mounts.
func NewCheckerWithExtraMounts(
	projectRoot string,
	containerImage string,
	command string,
	extraMounts []config.ExtraMount,
) Checker {
	c := &checker{
		command:        command,
		interval:       0,
		projectRoot:    projectRoot,
		containerImage: containerImage,
		extraMounts:    extraMounts,
	}
	c.shaFetcher = func(_ context.Context) (string, error) { return "sha", nil }
	c.runner = func(_ context.Context) (string, error) { return "", nil }
	return c
}
