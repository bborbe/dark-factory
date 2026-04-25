// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflight

import (
	"context"
	"time"

	"github.com/bborbe/dark-factory/pkg/notifier"
)

// NewCheckerWithRunner creates a Checker for testing, replacing runInContainer with a fake runner.
func NewCheckerWithRunner(
	command string,
	interval time.Duration,
	n notifier.Notifier,
	projectName string,
	runner func(ctx context.Context) (string, error),
) Checker {
	c := &checker{
		command:     command,
		interval:    interval,
		notifier:    n,
		projectName: projectName,
	}
	c.runner = runner
	return c
}
