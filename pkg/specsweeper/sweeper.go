// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package specsweeper

//go:generate go run -mod=mod github.com/maxbrunsfeld/counterfeiter/v6 -generate

import (
	"context"
	"log/slog"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/spec-sweeper.go --fake-name Sweeper . Sweeper

// Sweeper transitions specs in `prompted` status whose linked prompts are all complete
// to `verifying`. Self-healing safety net for the per-prompt auto-complete path.
type Sweeper interface {
	// Sweep returns the number of specs that transitioned to verifying.
	// The count is consumed by the processor's runSweepTick to drive the
	// NothingToDoCallback (no-progress detection in one-shot mode).
	Sweep(ctx context.Context) (transitioned int, err error)
}

// NewSweeper creates a new Sweeper that scans specs and transitions prompted ones
// whose linked prompts are all complete to verifying.
func NewSweeper(specLister spec.Lister, autoCompleter spec.AutoCompleter) Sweeper {
	return &sweeper{
		specLister:    specLister,
		autoCompleter: autoCompleter,
	}
}

type sweeper struct {
	specLister    spec.Lister
	autoCompleter spec.AutoCompleter
}

// Sweep scans all specs and calls CheckAndComplete for any in "prompted" status.
// Returns the count of specs that transitioned and any fatal error.
// This catches specs that were stuck in prompted state across daemon restarts.
func (s *sweeper) Sweep(ctx context.Context) (int, error) {
	specs, err := s.specLister.List(ctx)
	if err != nil {
		return 0, errors.Wrap(ctx, err, "list specs")
	}

	count := 0
	for _, sf := range specs {
		if sf.Frontmatter.Status != string(spec.StatusPrompted) {
			continue
		}
		slog.Info("startup: checking prompted spec", "spec", sf.Name)
		if err := s.autoCompleter.CheckAndComplete(ctx, sf.Name); err != nil {
			return count, errors.Wrap(ctx, err, "check and complete spec")
		}
		count++
	}

	return count, nil
}
