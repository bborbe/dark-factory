// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/doctor"
)

// counterfeiter:generate -o ../../mocks/doctor-command.go --fake-name DoctorCommand . DoctorCommand

// DoctorCommand executes the doctor subcommand.
type DoctorCommand interface {
	Run(ctx context.Context, args []string) error
}

// doctorCommand implements DoctorCommand.
type doctorCommand struct {
	checker             doctor.Checker
	fixer               doctor.Fixer
	verifyingStaleHours int
}

// NewDoctorCommand creates a new DoctorCommand.
func NewDoctorCommand(
	checker doctor.Checker,
	fixer doctor.Fixer,
	verifyingStaleHours int,
) DoctorCommand {
	return &doctorCommand{
		checker:             checker,
		fixer:               fixer,
		verifyingStaleHours: verifyingStaleHours,
	}
}

// Run executes the doctor command.
func (d *doctorCommand) Run(ctx context.Context, args []string) error {
	fixMode := false
	yesMode := false

	for _, arg := range args {
		switch arg {
		case "--fix":
			fixMode = true
		case "--yes":
			yesMode = true
		case "--help", "-h":
			DoctorHelp()
			return nil
		default:
			return errors.Errorf(ctx, "unknown flag: %q", arg)
		}
	}

	findings, err := d.checker.Check(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "doctor check failed")
	}

	if len(findings) == 0 {
		fmt.Fprintln(os.Stdout, "no findings")
		return nil
	}

	// Group findings by category for display.
	byCategory := groupFindingsByCategory(findings)
	for cat, catFindings := range byCategory {
		fmt.Fprintf(os.Stdout, "%s\n", cat)
		for _, f := range catFindings {
			targets := strings.Join(f.TargetPaths, " ")
			fmt.Fprintf(os.Stdout, "  %s  %s\n", targets, f.FixCommand)
		}
	}

	if !fixMode {
		return errors.Errorf(
			ctx,
			"doctor found %d finding(s); re-run with --fix to apply",
			len(findings),
		)
	}

	result, err := d.fixer.Apply(ctx, findings, doctor.ApplyOptions{
		Yes:             yesMode,
		AuditLogPath:    ".dark-factory/doctor.log",
		FileLockTimeout: 5 * time.Second,
	})
	if err != nil {
		return errors.Wrap(ctx, err, "fixer apply failed")
	}

	// Print result summary.
	if len(result.Applied) > 0 {
		fmt.Fprintf(os.Stdout, "applied %d fix(es)\n", len(result.Applied))
	}
	if len(result.Skipped) > 0 {
		fmt.Fprintf(os.Stdout, "skipped %d fix(es)\n", len(result.Skipped))
	}
	if len(result.Failed) > 0 {
		fmt.Fprintf(os.Stderr, "failed %d fix(es)\n", len(result.Failed))
		for _, ff := range result.Failed {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", ff.Category, ff.Detail)
		}
	}

	if len(result.Failed) > 0 {
		return errors.Errorf(ctx, "fixer had %d failure(s)", len(result.Failed))
	}

	return nil
}

func groupFindingsByCategory(findings []doctor.Finding) map[doctor.Category][]doctor.Finding {
	result := make(map[doctor.Category][]doctor.Finding)
	for _, f := range findings {
		result[f.Category] = append(result[f.Category], f)
	}
	// Sort categories for stable output.
	cats := make([]doctor.Category, 0, len(result))
	for cat := range result {
		cats = append(cats, cat)
	}
	sort.Slice(cats, func(i, j int) bool {
		return string(cats[i]) < string(cats[j])
	})
	return result
}

// DoctorHelp prints the doctor command help to stdout.
func DoctorHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory doctor [--fix] [--yes] [--verifying-stale-hours=N]\n\n"+
			"Detect state anomalies in spec and prompt files (and optionally fix them).\n\n"+
			"Flags:\n"+
			"  --fix                     Apply fixes for detected anomalies\n"+
			"  --yes                     Skip confirmation prompts (use with --fix)\n"+
			"  --verifying-stale-hours=N  Hours threshold for verifying-stale detection (default: 24)\n"+
			"  --help, -h                Show this help\n",
	)
}
