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
	checker executor.ExecutionChecker,
	mgr PromptManager,
	n notifier.Notifier,
	projectName string,
	maxPromptDuration time.Duration,
	stopper executor.ExecutionStopper,
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
		false,
	)
}

// CheckExecutingPromptsSkipForTest exposes checkExecutingPrompts with an explicit
// skipContainerReconcile flag for external test packages (backend: local path).
func CheckExecutingPromptsSkipForTest(
	ctx context.Context,
	inProgressDir string,
	checker executor.ExecutionChecker,
	mgr PromptManager,
	n notifier.Notifier,
	projectName string,
	maxPromptDuration time.Duration,
	stopper executor.ExecutionStopper,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	skipContainerReconcile bool,
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
		skipContainerReconcile,
	)
}

// CheckGeneratingSpecsForTest exposes checkGeneratingSpecs for external test packages.
func CheckGeneratingSpecsForTest(
	ctx context.Context,
	specsInProgressDir string,
	checker executor.ExecutionChecker,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	projectName string,
) error {
	return checkGeneratingSpecs(
		ctx,
		specsInProgressDir,
		checker,
		currentDateTimeGetter,
		projectName,
		false,
	)
}

// CheckGeneratingSpecsSkipForTest exposes checkGeneratingSpecs with an explicit
// skipContainerReconcile flag for external test packages (backend: local path).
func CheckGeneratingSpecsSkipForTest(
	ctx context.Context,
	specsInProgressDir string,
	checker executor.ExecutionChecker,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	projectName string,
	skipContainerReconcile bool,
) error {
	return checkGeneratingSpecs(
		ctx,
		specsInProgressDir,
		checker,
		currentDateTimeGetter,
		projectName,
		skipContainerReconcile,
	)
}

// RunHealthCheckLoopForTest exposes runHealthCheckLoop for external test packages.
func RunHealthCheckLoopForTest(
	ctx context.Context,
	interval time.Duration,
	inProgressDir string,
	specsInProgressDir string,
	checker executor.ExecutionChecker,
	mgr PromptManager,
	n notifier.Notifier,
	projectName string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	maxPromptDuration time.Duration,
	stopper executor.ExecutionStopper,
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
		false,
	)
}
