// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflightconditions

import (
	"context"
	stderrors "errors"
	"log/slog"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/preflight"
)

//counterfeiter:generate -o ../../mocks/preflight-conditions.go --fake-name Conditions . Conditions

// Conditions runs all pre-execution skip checks in order: baseline preflight, git index lock,
// dirty-file threshold. Returns ErrPreflightSkip for the baseline-broken case.
type Conditions interface {
	ShouldSkip(ctx context.Context) (skip bool, err error)
}

// ErrPreflightSkip is the canonical sentinel for the preflight-baseline-broken case.
// pkg/processor re-exports this value so existing stderrors.Is(err, processor.ErrPreflightSkip)
// call sites continue to match without rewriting.
var ErrPreflightSkip = stderrors.New("preflight baseline broken — skip cycle")

// GitLockChecker checks whether .git/index.lock exists in the working tree.
type GitLockChecker interface {
	Exists() bool
}

// DirtyFileChecker counts dirty files in a git working tree.
type DirtyFileChecker interface {
	CountDirtyFiles(ctx context.Context) (int, error)
}

// NewConditions creates a Conditions implementation.
// nil arguments disable the respective check.
func NewConditions(
	preflightChecker preflight.Checker,
	gitLockChecker GitLockChecker,
	dirtyFileChecker DirtyFileChecker,
	dirtyFileThreshold int,
) Conditions {
	return &conditions{
		preflightChecker:   preflightChecker,
		gitLockChecker:     gitLockChecker,
		dirtyFileChecker:   dirtyFileChecker,
		dirtyFileThreshold: dirtyFileThreshold,
	}
}

type conditions struct {
	preflightChecker   preflight.Checker
	gitLockChecker     GitLockChecker
	dirtyFileChecker   DirtyFileChecker
	dirtyFileThreshold int
}

// ShouldSkip runs all pre-execution skip checks in order.
// Returns (true, nil) if the prompt should be skipped this cycle (transient conditions).
// Returns (false, ErrPreflightSkip) if the preflight baseline is broken.
func (c *conditions) ShouldSkip(ctx context.Context) (bool, error) {
	if c.preflightChecker != nil {
		ok, err := c.preflightChecker.Check(ctx)
		if err != nil {
			slog.Warn("preflight checker error, skipping cycle", "error", err)
			return false, ErrPreflightSkip
		}
		if !ok {
			slog.Info("preflight: baseline broken — prompt stays queued until baseline is fixed")
			return false, ErrPreflightSkip
		}
	}

	if c.gitLockChecker != nil && c.gitLockChecker.Exists() {
		slog.Warn("git index lock exists, skipping prompt — will retry next cycle")
		return true, nil
	}

	if c.dirtyFileThreshold <= 0 || c.dirtyFileChecker == nil {
		return false, nil
	}
	count, err := c.dirtyFileChecker.CountDirtyFiles(ctx)
	if err != nil {
		return false, errors.Wrap(ctx, err, "count dirty files")
	}
	if count > c.dirtyFileThreshold {
		slog.Warn(
			"dirty file threshold exceeded, skipping prompt",
			"dirtyFiles", count,
			"threshold", c.dirtyFileThreshold,
		)
		return true, nil
	}
	return false, nil
}
