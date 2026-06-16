// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healthcheckgate

import (
	"context"
	"log/slog"
	"time"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/notifier"
)

//counterfeiter:generate -o ../../mocks/healthcheck-gate.go --fake-name HealthcheckGate . Gate

// Gate runs the healthcheck probe sequence at daemon startup with success-only caching.
type Gate interface {
	// Check runs the gate. Returns nil when the probes pass, when a fresh
	// cached success exists, or when the gate is disabled. Returns a
	// category-naming error when the probes fail (caller treats as terminal).
	Check(ctx context.Context) error
}

//counterfeiter:generate -o ../../mocks/healthcheck-gate-command.go --fake-name HealthcheckGateCommand . HealthcheckCommand

// HealthcheckCommand runs the probe sequence. Satisfied by cmd.HealthcheckCommand.
type HealthcheckCommand interface {
	Run(ctx context.Context, args []string) error
}

// NewGate creates a new Gate.
func NewGate(
	enabled bool,
	skip bool,
	interval time.Duration,
	healthcheck HealthcheckCommand,
	cache Cache,
	n notifier.Notifier,
	projectName string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) Gate {
	return &gate{
		enabled:               enabled,
		skip:                  skip,
		interval:              libtime.Duration(interval),
		healthcheck:           healthcheck,
		cache:                 cache,
		notifier:              n,
		projectName:           projectName,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// gate implements Gate.
type gate struct {
	enabled               bool
	skip                  bool
	interval              libtime.Duration
	healthcheck           HealthcheckCommand
	cache                 Cache
	notifier              notifier.Notifier
	projectName           string
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// Check runs the gate.
func (g *gate) Check(ctx context.Context) error {
	if !g.enabled {
		slog.Info("healthcheck gate disabled")
		return nil
	}
	if g.skip {
		slog.Info("healthcheck skipped via --skip-healthcheck")
		return nil
	}

	now := time.Time(g.currentDateTimeGetter.Now())
	if g.cache.Fresh(ctx, g.projectName, time.Duration(g.interval), now) {
		slog.Info("healthcheck cache hit, skipping")
		return nil
	}

	slog.Info("healthcheck startup gate starting")

	if err := g.healthcheck.Run(ctx, []string{}); err != nil {
		_ = g.notifier.Notify(ctx, notifier.Event{
			ProjectName: g.projectName,
			EventType:   "healthcheck_failed",
		})
		return errors.Wrap(ctx, err, "healthcheck failed")
	}

	end := time.Time(g.currentDateTimeGetter.Now())
	elapsed := end.Sub(now)
	g.cache.Write(ctx, g.projectName, now)
	slog.Info("healthcheck startup gate ok", "elapsed", elapsed)
	return nil
}
