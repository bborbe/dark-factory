// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/cmd/healthcheck"
)

//counterfeiter:generate -o ../../mocks/healthcheck-command.go --fake-name HealthcheckCommand . HealthcheckCommand

// HealthcheckCommand executes the healthcheck subcommand. It runs the four
// local probes (Docker, image, boot, mount) in fixed order and fails fast on
// the first error.
type HealthcheckCommand interface {
	Run(ctx context.Context, args []string) error
}

// Probes is the bundle of local probes the healthcheck command iterates over.
// Each entry is one category of the pipeline-execution stack. The slice order
// is the execution order; callers should preserve the documented order
// (docker, image, boot, mount) for a stable operator-facing sequence.
type Probes []healthcheck.Probe

// NewHealthcheckCommand creates a new HealthcheckCommand. The factory is
// construction-only: callers pre-build the probe list in the desired order
// and pass it in. The command does no business logic beyond flag parsing and
// fail-fast iteration.
func NewHealthcheckCommand(probes Probes) HealthcheckCommand {
	return &healthcheckCommand{probes: probes}
}

// healthcheckCommand implements HealthcheckCommand.
type healthcheckCommand struct {
	probes Probes
}

// Run executes the healthcheck command. Flags:
//
//	--no-claude   (consumed; reserved for prompt 2b)
//	--help, -h    intercepted by main.go before reaching Run
//
// Unknown flags are rejected. The four local probes run in fixed order and
// fail-fast: the first probe that returns a non-nil error short-circuits the
// sequence and the failure category is printed to stdout.
func (h *healthcheckCommand) Run(ctx context.Context, args []string) error {
	for _, arg := range args {
		switch arg {
		case "--no-claude":
			// Consumed; the four local probes in this prompt do not invoke Claude.
			// Prompt 2b adds the Claude probe and honors this flag.
			continue
		default:
			return errors.Errorf(ctx, "unknown flag: %q", arg)
		}
	}

	for _, p := range h.probes {
		if err := p.Run(ctx); err != nil {
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
