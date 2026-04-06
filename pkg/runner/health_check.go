// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

// runHealthCheckLoop runs periodic container health checks every interval.
// It checks prompts in executing state and specs in generating state.
// Returns nil when ctx is cancelled (clean shutdown).
func runHealthCheckLoop(
	ctx context.Context,
	interval time.Duration,
	inProgressDir string,
	specsInProgressDir string,
	checker executor.ContainerChecker,
	mgr prompt.Manager,
	n notifier.Notifier,
	projectName string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	maxPromptDuration time.Duration,
	stopper executor.ContainerStopper,
) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			slog.Debug("running container health check")
			if err := checkExecutingPrompts(ctx, inProgressDir, checker, mgr, n, projectName, maxPromptDuration, stopper, currentDateTimeGetter); err != nil {
				slog.Warn("health check for executing prompts failed", "error", err)
			}
			if err := checkGeneratingSpecs(ctx, specsInProgressDir, checker, currentDateTimeGetter); err != nil {
				slog.Warn("health check for generating specs failed", "error", err)
			}
		}
	}
}

// checkExecutingPrompts scans inProgressDir for prompts in executing state and resets any
// whose container is no longer running. If maxPromptDuration > 0, it also stops and marks
// failed any prompt that has been running longer than maxPromptDuration.
func checkExecutingPrompts(
	ctx context.Context,
	inProgressDir string,
	checker executor.ContainerChecker,
	mgr prompt.Manager,
	n notifier.Notifier,
	projectName string,
	maxPromptDuration time.Duration,
	stopper executor.ContainerStopper,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	entries, err := os.ReadDir(inProgressDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(ctx, err, "read in-progress dir for health check")
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		checkExecutingPrompt(
			ctx,
			inProgressDir,
			entry,
			checker,
			mgr,
			n,
			projectName,
			maxPromptDuration,
			stopper,
			currentDateTimeGetter,
		)
	}
	return nil
}

// checkExecutingPrompt checks a single prompt file and handles container-gone or timeout cases.
func checkExecutingPrompt(
	ctx context.Context,
	inProgressDir string,
	entry os.DirEntry,
	checker executor.ContainerChecker,
	mgr prompt.Manager,
	n notifier.Notifier,
	projectName string,
	maxPromptDuration time.Duration,
	stopper executor.ContainerStopper,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) {
	path := filepath.Join(inProgressDir, entry.Name())
	pf, err := mgr.Load(ctx, path)
	if err != nil || pf == nil {
		return
	}
	if prompt.PromptStatus(pf.Frontmatter.Status) != prompt.ExecutingPromptStatus {
		return
	}
	containerName := pf.Frontmatter.Container
	running, err := checker.IsRunning(ctx, containerName)
	if err != nil {
		slog.Warn("health check: failed to check prompt container, skipping",
			"file", entry.Name(), "container", containerName, "error", err)
		return
	}
	if !running {
		resetGonePrompt(ctx, pf, entry, n, projectName)
		return
	}
	slog.Debug(
		"health check: prompt container running",
		"file",
		entry.Name(),
		"container",
		containerName,
	)
	if isTimedOut(ctx, pf, maxPromptDuration, currentDateTimeGetter) {
		stopTimedOutPrompt(
			ctx,
			pf,
			entry,
			containerName,
			n,
			projectName,
			stopper,
			maxPromptDuration,
		)
	}
}

// isTimedOut returns true when maxPromptDuration is set and the prompt has exceeded it.
func isTimedOut(
	ctx context.Context,
	pf *prompt.PromptFile,
	maxPromptDuration time.Duration,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) bool {
	if maxPromptDuration == 0 || pf.Frontmatter.Started == "" {
		return false
	}
	started, err := libtime.ParseDateTime(ctx, pf.Frontmatter.Started)
	if err != nil {
		return false
	}
	now := currentDateTimeGetter.Now()
	elapsed := time.Time(now).Sub(time.Time(*started))
	return elapsed > maxPromptDuration
}

// stopTimedOutPrompt stops the container and marks the prompt failed.
func stopTimedOutPrompt(
	ctx context.Context,
	pf *prompt.PromptFile,
	entry os.DirEntry,
	containerName string,
	n notifier.Notifier,
	projectName string,
	stopper executor.ContainerStopper,
	maxPromptDuration time.Duration,
) {
	slog.Warn("health check: prompt exceeded maxPromptDuration, stopping",
		"file", entry.Name(),
		"container", containerName,
		"started", pf.Frontmatter.Started,
		"maxPromptDuration", maxPromptDuration,
	)
	if stopper != nil {
		if err := stopper.StopContainer(ctx, containerName); err != nil {
			slog.Warn("health check: failed to stop timed-out container",
				"container", containerName, "error", err)
		}
	}
	pf.MarkFailed()
	pf.SetLastFailReason(fmt.Sprintf("exceeded maxPromptDuration (%s)", maxPromptDuration))
	if err := pf.Save(ctx); err != nil {
		slog.Warn("health check: failed to save timed-out prompt",
			"file", entry.Name(), "error", err)
	}
	if n != nil {
		_ = n.Notify(ctx, notifier.Event{
			ProjectName: projectName,
			EventType:   "prompt_timeout",
			PromptName:  entry.Name(),
		})
	}
}

// resetGonePrompt resets a prompt whose container is no longer running.
func resetGonePrompt(
	ctx context.Context,
	pf *prompt.PromptFile,
	entry os.DirEntry,
	n notifier.Notifier,
	projectName string,
) {
	slog.Warn("health check: prompt container gone, resetting to approved",
		"file", entry.Name(), "container", pf.Frontmatter.Container)
	if n != nil {
		_ = n.Notify(ctx, notifier.Event{
			ProjectName: projectName,
			EventType:   "stuck_container",
			PromptName:  entry.Name(),
		})
	}
	pf.MarkApproved()
	if err := pf.Save(ctx); err != nil {
		slog.Warn("health check: failed to save reset prompt",
			"file", entry.Name(), "error", err)
	}
}

// checkGeneratingSpecs scans specsInProgressDir for specs in generating state and resets any
// whose generation container is no longer running.
func checkGeneratingSpecs(
	ctx context.Context,
	specsInProgressDir string,
	checker executor.ContainerChecker,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	entries, err := os.ReadDir(specsInProgressDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(ctx, err, "read specs in-progress dir for health check")
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(specsInProgressDir, entry.Name())
		sf, err := spec.Load(ctx, path, currentDateTimeGetter)
		if err != nil || sf == nil {
			continue
		}
		if spec.Status(sf.Frontmatter.Status) != spec.StatusGenerating {
			continue
		}
		specBasename := strings.TrimSuffix(entry.Name(), ".md")
		containerName := "dark-factory-gen-" + specBasename
		running, err := checker.IsRunning(ctx, containerName)
		if err != nil {
			slog.Warn("health check: failed to check spec container, skipping",
				"file", entry.Name(), "container", containerName, "error", err)
			continue
		}
		if running {
			slog.Debug(
				"health check: spec container running",
				"file",
				entry.Name(),
				"container",
				containerName,
			)
			continue
		}
		slog.Warn("health check: spec generation container gone, resetting to approved",
			"file", entry.Name(), "container", containerName)
		sf.SetStatus(string(spec.StatusApproved))
		if err := sf.Save(ctx); err != nil {
			slog.Warn("health check: failed to save reset spec",
				"file", entry.Name(), "error", err)
		}
	}
	return nil
}
