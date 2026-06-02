// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// fixStatusDirMismatch moves a file to the directory consistent with its status.
func (f *fixer) fixStatusDirMismatch(
	ctx context.Context,
	finding Finding,
	opts ApplyOptions,
) (applied []AppliedFix, failed []FailedFix) {
	expectedDir, filename, dispatchErr := f.dispatchStatusDirMismatch(finding)
	if dispatchErr != "" {
		failed = append(failed, FailedFix{
			Category:    finding.Category,
			TargetPaths: finding.TargetPaths,
			Detail:      dispatchErr,
		})
		return
	}

	for _, path := range finding.TargetPaths {
		af, ff := f.applyStatusDirMismatchPath(ctx, path, expectedDir, filename, finding, opts)
		if af != nil {
			applied = append(applied, *af)
		}
		if ff != nil {
			failed = append(failed, *ff)
		}
	}

	return
}

// dispatchStatusDirMismatch returns (expectedDir, filename, "") on success
// or ("", "", errorDetail) when the FixCommand cannot be resolved.
func (f *fixer) dispatchStatusDirMismatch(finding Finding) (string, string, string) {
	fixCmd := finding.FixCommand
	if specID, ok := strings.CutPrefix(fixCmd, "dark-factory spec move "); ok {
		expectedDir := f.expectedSpecDir(finding.Detail)
		if expectedDir == "" {
			return "", "", "could not determine expected directory from detail"
		}
		return expectedDir, specID + ".md", ""
	}
	if promptID, ok := strings.CutPrefix(fixCmd, "dark-factory prompt move "); ok {
		expectedDir := f.expectedPromptDir(finding.Detail)
		if expectedDir == "" {
			return "", "", "could not determine expected directory from detail"
		}
		return expectedDir, promptID + ".md", ""
	}
	return "", "", "unknown FixCommand: " + fixCmd
}

func (f *fixer) applyStatusDirMismatchPath(
	ctx context.Context,
	path string,
	expectedDir string,
	filename string,
	finding Finding,
	opts ApplyOptions,
) (applied *AppliedFix, failed *FailedFix) {
	fl := f.deps.FileLockFactory(path)
	if err := fl.Acquire(ctx, opts.FileLockTimeout); err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "lock acquire failed: " + err.Error(),
		}
	}
	defer releaseLock(ctx, fl, path)

	if err := os.MkdirAll(expectedDir, 0750); err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "create directory failed: " + err.Error(),
		}
	}

	dest := filepath.Join(expectedDir, filename)

	// Audit BEFORE the mutating os.Rename: audit failure leaves the file in
	// place with no orphan trail; rename failure leaves the file in place
	// with the audit recording the attempt.
	if err := WriteAuditEntry(ctx, opts.AuditLogPath, AuditEntry{
		Timestamp:   time.Time(f.deps.CurrentDateTimeGetter.Now()),
		Category:    finding.Category,
		Action:      "applied",
		TargetPaths: []string{path, dest},
		Before:      filepath.Base(path),
		After:       filepath.Base(dest),
	}); err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "audit log write failed: " + err.Error(),
		}
	}

	if err := os.Rename(path, dest); err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "rename failed: " + err.Error(),
		}
	}

	return &AppliedFix{
		Category:    finding.Category,
		TargetPaths: []string{path, dest},
		FixCommand:  finding.FixCommand,
		AuditLine:   "",
	}, nil
}

// expectedSpecDir determines the correct spec directory from the Detail string.
func (f *fixer) expectedSpecDir(detail string) string {
	if strings.Contains(detail, "specs/in-progress/") {
		if strings.Contains(detail, "status completed") {
			return f.deps.SpecsCompletedDir
		}
		if strings.Contains(detail, "status rejected") {
			return f.deps.SpecsRejectedDir
		}
		return f.deps.SpecsInProgressDir
	}
	if strings.Contains(detail, "specs/completed/") {
		return f.deps.SpecsCompletedDir
	}
	if strings.Contains(detail, "specs/rejected/") {
		return f.deps.SpecsRejectedDir
	}
	return ""
}

// expectedPromptDir determines the correct prompt directory from the Detail string.
func (f *fixer) expectedPromptDir(detail string) string {
	if strings.Contains(detail, "prompts/in-progress/") {
		if strings.Contains(detail, "status completed") ||
			strings.Contains(detail, "status rejected") {
			return f.deps.PromptsCompletedDir
		}
		return f.deps.PromptsInProgressDir
	}
	if strings.Contains(detail, "prompts/completed/") {
		return f.deps.PromptsCompletedDir
	}
	if strings.Contains(detail, "prompts/cancelled/") {
		return f.deps.PromptsCancelledDir
	}
	return ""
}
