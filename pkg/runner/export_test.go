// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

import (
	"context"
	"time"

	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/notifier"
)

// StartupDepsForTest re-exports StartupDeps for external test packages.
type StartupDepsForTest = StartupDeps

// RunStartupSequenceForTest exposes startupSequence for external test packages.
func RunStartupSequenceForTest(ctx context.Context, deps StartupDeps) error {
	return startupSequence(ctx, deps)
}

// CheckExecutingPromptsForTest exposes checkExecutingPrompts for external test packages.
func CheckExecutingPromptsForTest(
	ctx context.Context,
	inProgressDir string,
	checker executor.ContainerChecker,
	mgr PromptManager,
	n notifier.Notifier,
	projectName string,
	maxPromptDuration time.Duration,
	stopper executor.ContainerStopper,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	return checkExecutingPrompts(
		ctx,
		inProgressDir,
		checker,
		mgr,
		n,
		projectName,
		maxPromptDuration,
		stopper,
		currentDateTimeGetter,
	)
}

// CheckGeneratingSpecsForTest exposes checkGeneratingSpecs for external test packages.
func CheckGeneratingSpecsForTest(
	ctx context.Context,
	specsInProgressDir string,
	checker executor.ContainerChecker,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	return checkGeneratingSpecs(ctx, specsInProgressDir, checker, currentDateTimeGetter)
}

// RunHealthCheckLoopForTest exposes runHealthCheckLoop for external test packages.
func RunHealthCheckLoopForTest(
	ctx context.Context,
	interval time.Duration,
	inProgressDir string,
	specsInProgressDir string,
	checker executor.ContainerChecker,
	mgr PromptManager,
	n notifier.Notifier,
	projectName string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	maxPromptDuration time.Duration,
	stopper executor.ContainerStopper,
) error {
	return runHealthCheckLoop(
		ctx,
		interval,
		inProgressDir,
		specsInProgressDir,
		checker,
		mgr,
		n,
		projectName,
		currentDateTimeGetter,
		maxPromptDuration,
		stopper,
	)
}
