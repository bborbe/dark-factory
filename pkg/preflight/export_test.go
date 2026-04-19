// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflight

import (
	"context"
	"time"

	"github.com/bborbe/dark-factory/pkg/notifier"
)

// Exported wrappers for pure helpers — used in unit tests.
var (
	TruncateSHA = truncateSHA
	MinLen      = minLen
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
