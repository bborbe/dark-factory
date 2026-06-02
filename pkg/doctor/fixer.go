// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/project"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

// ApplyOptions controls the Fixer.Apply behavior.
type ApplyOptions struct {
	// Yes skips interactive confirmation prompts.
	Yes bool
	// Stdin defaults to os.Stdin if nil.
	Stdin io.Reader
	// Stdout defaults to os.Stdout if nil.
	Stdout io.Writer
	// Stderr defaults to os.Stderr if nil.
	Stderr io.Writer
	// AuditLogPath defaults to .dark-factory/doctor.log relative to project root.
	AuditLogPath string
	// FileLockTimeout defaults to 5 seconds if zero.
	FileLockTimeout time.Duration
}

// AppliedFix records a fix that was successfully applied.
type AppliedFix struct {
	Category    Category
	TargetPaths []string
	FixCommand  string
	AuditLine   string
}

// SkippedFix records a fix that was skipped.
type SkippedFix struct {
	Category    Category
	TargetPaths []string
	Detail      string
}

// FailedFix records a fix that failed.
type FailedFix struct {
	Category    Category
	TargetPaths []string
	Detail      string
}

// ApplyResult summarises the outcome of Fixer.Apply.
type ApplyResult struct {
	Applied []AppliedFix
	Skipped []SkippedFix
	Failed  []FailedFix
}

// FixerDeps holds the dependencies for the Fixer.
// It reuses the same Deps struct as the Checker (same scanners are needed)
// and adds the mutating-phase dependencies.
type FixerDeps struct {
	Deps
	AutoCompleter   spec.AutoCompleter
	Mover           prompt.FileMover
	FileLockFactory func(path string) lock.FileLock
}

//counterfeiter:generate -o ../../mocks/doctor-fixer.go --fake-name DoctorFixer . Fixer

// Fixer applies fixes for anomalies detected by Checker.
type Fixer interface {
	Apply(ctx context.Context, findings []Finding, opts ApplyOptions) (ApplyResult, error)
}

// NewFixer creates a Fixer with the given dependencies.
func NewFixer(deps FixerDeps) Fixer {
	if deps.FileLockFactory == nil {
		deps.FileLockFactory = lock.NewFileLock
	}
	return &fixer{deps: deps}
}

type fixer struct {
	deps FixerDeps
}

func (f *fixer) Apply(
	ctx context.Context,
	findings []Finding,
	opts ApplyOptions,
) (ApplyResult, error) {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.FileLockTimeout == 0 {
		opts.FileLockTimeout = 5 * time.Second
	}
	if opts.AuditLogPath == "" {
		root, err := project.FindRoot(ctx)
		if err != nil {
			return ApplyResult{}, errors.Wrap(ctx, err, "find project root for audit log path")
		}
		opts.AuditLogPath = root + "/.dark-factory/doctor.log"
	}

	var result ApplyResult
	for _, finding := range findings {
		af, sf, ff := f.applyFinding(ctx, finding, opts)
		result.Applied = append(result.Applied, af...)
		result.Skipped = append(result.Skipped, sf...)
		result.Failed = append(result.Failed, ff...)
	}
	return result, nil
}

func (f *fixer) applyFinding(
	ctx context.Context,
	finding Finding,
	opts ApplyOptions,
) (applied []AppliedFix, skipped []SkippedFix, failed []FailedFix) {
	// Ask for confirmation unless --yes.
	if !opts.Yes {
		fmt.Fprintf(opts.Stdout, "Apply? [y/N] ")
		line, err := f.readLine(opts.Stdin)
		if err != nil || (line != "y" && line != "Y") {
			skipped = append(skipped, SkippedFix{
				Category:    finding.Category,
				TargetPaths: finding.TargetPaths,
				Detail:      "operator declined",
			})
			return
		}
	}

	switch finding.Category {
	case CategoryDuplicateSpecNumbers:
		af, ff := f.fixDuplicateSpecNumbers(ctx, finding, opts)
		return af, nil, ff
	case CategoryPromptedNotSwept:
		af, ff := f.fixPromptedNotSwept(ctx, finding, opts)
		return af, nil, ff
	case CategoryVerifyingStale:
		skipped = append(skipped, SkippedFix{
			Category:    finding.Category,
			TargetPaths: finding.TargetPaths,
			Detail:      "verifying-stale is informational; run `dark-factory spec verify <id>` manually",
		})
		return
	case CategoryOrphanPromptLink:
		af, ff := f.fixOrphanPromptLink(ctx, finding, opts)
		return af, nil, ff
	case CategoryOrphanInProgressPrompt:
		af, sf, ff := f.fixOrphanInProgressPrompt(ctx, finding, opts)
		return af, sf, ff
	case CategoryStatusDirMismatch:
		af, ff := f.fixStatusDirMismatch(ctx, finding, opts)
		return af, nil, ff
	case CategoryParseError:
		skipped = append(skipped, SkippedFix{
			Category:    finding.Category,
			TargetPaths: finding.TargetPaths,
			Detail:      "parse-errors require manual YAML fix",
		})
		return
	default:
		skipped = append(skipped, SkippedFix{
			Category:    finding.Category,
			TargetPaths: finding.TargetPaths,
			Detail:      "unknown category",
		})
		return
	}
}

func (f *fixer) readLine(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text()), scanner.Err()
	}
	return "", scanner.Err()
}
