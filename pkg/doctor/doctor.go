// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"os"
	"sort"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

// Category describes the type of anomaly detected.
type Category string

// CategoryDuplicateSpecNumbers indicates two or more spec files share the same numeric prefix.
const CategoryDuplicateSpecNumbers Category = "duplicate-spec-numbers"

// CategoryPromptedNotSwept indicates a spec is in "prompted" status but all linked prompts are terminal.
const CategoryPromptedNotSwept Category = "prompted-but-not-swept"

// CategoryVerifyingStale indicates a spec is in "verifying" status with a stale or missing timestamp.
const CategoryVerifyingStale Category = "verifying-stale"

// CategoryOrphanPromptLink indicates a prompt references a spec that does not exist.
const CategoryOrphanPromptLink Category = "orphan-prompt-link"

// CategoryOrphanInProgressPrompt indicates a prompt in in-progress/ links to a completed or rejected spec.
const CategoryOrphanInProgressPrompt Category = "orphan-in-progress-prompt"

// CategoryStatusDirMismatch indicates a file resides in a directory inconsistent with its status field.
const CategoryStatusDirMismatch Category = "status-dir-mismatch"

// CategoryParseError indicates a file's YAML frontmatter could not be parsed.
const CategoryParseError Category = "parse-errors"

// Finding represents a detected anomaly in spec or prompt files.
type Finding struct {
	// Category is the type of anomaly detected.
	Category Category
	// TargetPaths are the file paths involved in the anomaly (always sorted lexicographically).
	TargetPaths []string
	// SpecID is the numeric spec identifier associated with the finding (empty when not a spec finding).
	SpecID string
	// Detail is a human-readable one-line description of the anomaly.
	Detail string
	// FixCommand is the copy-paste command to fix the anomaly.
	FixCommand string
}

// counterfeiter:generate -o ../../mocks/doctor-checker.go --fake-name DoctorChecker . Checker

// Checker detects state anomalies in spec and prompt files.
type Checker interface {
	Check(ctx context.Context) ([]Finding, error)
}

// Deps holds the directory paths and scanners required by the Checker.
type Deps struct {
	SpecsInboxDir         string
	SpecsInProgressDir    string
	SpecsCompletedDir     string
	SpecsRejectedDir      string
	PromptsInboxDir       string
	PromptsInProgressDir  string
	PromptsCompletedDir   string
	PromptsCancelledDir   string
	SpecLister            spec.Lister
	PromptManager         *prompt.Manager
	CurrentDateTimeGetter libtime.CurrentDateTimeGetter
	VerifyingStaleHours   int
}

// NewChecker creates a Checker with the given dependencies.
func NewChecker(deps Deps) Checker {
	return &checker{deps: deps}
}

type checker struct {
	deps Deps
}

// Check runs all six detectors and returns the concatenated findings.
// Parse-error findings are appended last.
func (c *checker) Check(ctx context.Context) ([]Finding, error) {
	// Verify the project is initialized.
	if _, err := os.Stat(c.deps.SpecsInProgressDir); os.IsNotExist(err) {
		return nil, errors.Errorf(
			ctx,
			"not a dark-factory project: missing %s",
			c.deps.SpecsInProgressDir,
		)
	}

	all := []Finding{}

	duplicates, err := c.detectDuplicateSpecNumbers(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "detect duplicate spec numbers")
	}
	all = append(all, duplicates...)

	promptedNotSwept, err := c.detectPromptedNotSwept(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "detect prompted-not-swept")
	}
	all = append(all, promptedNotSwept...)

	verifyingStale, err := c.detectVerifyingStale(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "detect verifying-stale")
	}
	all = append(all, verifyingStale...)

	orphanLinks, err := c.detectOrphanPromptLinks(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "detect orphan prompt links")
	}
	all = append(all, orphanLinks...)

	orphanInProgress, err := c.detectOrphanInProgressPrompts(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "detect orphan in-progress prompts")
	}
	all = append(all, orphanInProgress...)

	statusMismatches, err := c.detectStatusDirMismatches(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "detect status-dir-mismatches")
	}
	all = append(all, statusMismatches...)

	parseErrors, err := c.scanParseErrors(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "scan parse errors")
	}
	all = append(all, parseErrors...)

	// Sort TargetPaths within each finding.
	for i := range all {
		if len(all[i].TargetPaths) > 0 {
			sort.Strings(all[i].TargetPaths)
		}
	}

	return all, nil
}
