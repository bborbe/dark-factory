// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflight

import (
	"context"

	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/notifier"
)

// NewCheckerWithRunner creates a Checker for testing, replacing runInContainer with a fake runner.
func NewCheckerWithRunner(
	command string,
	interval libtime.Duration,
	n notifier.Notifier,
	projectName string,
	runner func(ctx context.Context) (string, error),
) Checker {
	c := &checker{
		command:               command,
		interval:              interval,
		notifier:              n,
		projectName:           projectName,
		currentDateTimeGetter: libtime.NewCurrentDateTime(),
	}
	c.runner = runner
	return c
}

// RunInContainerForTest exposes the runInContainer method for the
// stderr-capture regression test (scenario 010 caught a regression where
// stderr was silently dropped after spec 100's subproc.Runner migration).
func RunInContainerForTest(c Checker, ctx context.Context) (string, error) {
	//nolint:forcetypeassert // test-only helper; NewChecker always returns *checker
	cc := c.(*checker)
	return cc.runInContainer(ctx)
}
