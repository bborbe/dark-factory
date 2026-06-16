// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/cmd/healthcheck"
)

//counterfeiter:generate -o ../../mocks/healthcheck-command.go --fake-name HealthcheckCommand . HealthcheckCommand

// HealthcheckCommand executes the healthcheck subcommand. It runs the
// configured probes (Docker, image, boot, Claude, mount, gh, notifications)
// in fixed order and fails fast on the first error. The factory decides
// which probes to include based on config (pr: true gates gh, presence of
// any notification channel gates notifications); --no-claude trims the
// Claude probe at run time.
type HealthcheckCommand interface {
	Run(ctx context.Context, args []string) error
}

// Probes is the bundle of probes the healthcheck command iterates over.
// Each entry is one category of the pipeline-execution stack. The slice
// order is the execution order; callers should preserve the documented
// order (docker, image, boot, claude, mount, gh, notifications) for a
// stable operator-facing sequence.
type Probes []healthcheck.Probe

// NewHealthcheckCommand creates a new HealthcheckCommand. The factory is
// construction-only: callers pre-build the probe list in the desired order
// and pass it in. The command does no business logic beyond flag parsing
// and fail-fast iteration.
func NewHealthcheckCommand(probes Probes) HealthcheckCommand {
	return &healthcheckCommand{probes: probes}
}

// healthcheckCommand implements HealthcheckCommand.
type healthcheckCommand struct {
	probes Probes
}

// claudeProbeName is the probe category name trimmed when --no-claude is
// supplied. Other probes have no flag override.
const claudeProbeName = "claude"

// Run executes the healthcheck command. Flags:
//
//	--no-claude   omit the Claude session probe from the iteration list
//	--help, -h    intercepted by main.go before reaching Run
//
// Unknown flags are rejected. The remaining probes run in the order they
// were passed to NewHealthcheckCommand and fail-fast: the first probe that
// returns a non-nil error short-circuits the sequence and the failure
// category is printed to stdout.
func (h *healthcheckCommand) Run(ctx context.Context, args []string) error {
	skipClaude := false
	for _, arg := range args {
		switch arg {
		case "--no-claude":
			skipClaude = true
		default:
			return errors.Errorf(ctx, "unknown flag: %q", arg)
		}
	}

	probes := h.probes
	if skipClaude {
		probes = make(Probes, 0, len(h.probes))
		for _, p := range h.probes {
			if p.Name() == claudeProbeName {
				continue
			}
			probes = append(probes, p)
		}
	}

	for _, p := range probes {
		slog.Info("healthcheck probe starting", "probe", p.Name())
		if err := p.Run(ctx); err != nil {
			slog.Error("healthcheck probe failed", "probe", p.Name(), "error", err)
			printFailureTable(p.Name(), err)
			return errors.Wrapf(ctx, err, "healthcheck probe %q failed", p.Name())
		}
	}

	fmt.Fprintln(os.Stdout, "all probes passed")
	return nil
}

// printFailureTable writes a one-row failure table to stdout. The shape
// mirrors `dark-factory doctor`'s category-on-its-own-line convention so
// operators read both commands with the same mental model.
func printFailureTable(category string, err error) {
	fmt.Fprintf(os.Stdout, "%s\n", category)
	detail := err.Error()
	// Defang any leading "category: " prefix the wrapped error may include so
	// the detail line stands alone.
	detail = strings.TrimSpace(detail)
	fmt.Fprintf(os.Stdout, "  %s\n", detail)
}

// HealthcheckHelp prints the healthcheck command help to stdout.
func HealthcheckHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory healthcheck [--no-claude]\n\n"+
			"Probe the full pipeline-execution stack (Docker, image, boot, Claude, mount, gh, notifications) and exit 0 on full pass.\n\n"+
			"Flags:\n"+
			"  --no-claude   Skip the Claude session probe (the only token-spending probe)\n"+
			"  --help, -h    Show this help\n",
	)
}
