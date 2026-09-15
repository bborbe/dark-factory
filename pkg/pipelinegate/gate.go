// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipelinegate

import (
	"context"
	"log/slog"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/doctor"
)

// DefaultEnabled is the default for the gate's enabled control. The gate ships
// unarmed because arming it is an operational decision owned outside this spec;
// flipping this one constant arms it.
const DefaultEnabled = false

//counterfeiter:generate -o ../../mocks/pipeline-gate.go --fake-name PipelineGate . Gate

// Gate refuses daemon startup when the pipeline is not clean.
type Gate interface {
	// Check returns nil when the gate is disabled, skipped, or doctor reports no
	// findings. Returns a findings-naming error when the pipeline is dirty
	// (the caller treats it as terminal).
	Check(ctx context.Context) error
}

// NewGate creates a new Gate.
func NewGate(enabled bool, skip bool, checker doctor.Checker) Gate {
	return &gate{
		enabled: enabled,
		skip:    skip,
		checker: checker,
	}
}

// gate implements Gate.
type gate struct {
	enabled bool
	skip    bool
	checker doctor.Checker
}

// Check runs the gate. Findings are never cached: a pipeline scan is a
// filesystem read, and a cached pass would hide a leftover that landed
// mid-session.
func (g *gate) Check(ctx context.Context) error {
	if !g.enabled {
		slog.Info("pipeline gate disabled")
		return nil
	}
	if g.skip {
		slog.Info("pipeline gate skipped")
		return nil
	}

	findings, err := g.checker.Check(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "pipeline check")
	}
	if len(findings) == 0 {
		return nil
	}
	return errors.Errorf(
		ctx,
		"pipeline not clean: %d finding(s): %s",
		len(findings),
		summarizeFindings(findings),
	)
}

// summarizeFindings renders one "<category>: <detail>" fragment per finding,
// joined with "; ", in the order the checker returned them. Detail carries the
// offending prompt numbers, so the gate names them without re-deriving anything.
func summarizeFindings(findings []doctor.Finding) string {
	fragments := make([]string, 0, len(findings))
	for _, finding := range findings {
		fragments = append(fragments, string(finding.Category)+": "+finding.Detail)
	}
	return strings.Join(fragments, "; ")
}
