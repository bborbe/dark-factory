// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
)

// fixPromptedNotSwept calls AutoCompleter.CheckAndComplete for the spec, which
// transitions it from "prompted" to "verifying" when all linked prompts are terminal.
func (f *fixer) fixPromptedNotSwept(
	ctx context.Context,
	finding Finding,
	opts ApplyOptions,
) (applied []AppliedFix, failed []FailedFix) {
	// TargetPaths is a single path: the spec file.
	if len(finding.TargetPaths) == 0 {
		return
	}
	specPath := finding.TargetPaths[0]
	specID := strings.TrimSuffix(filepath.Base(specPath), ".md")

	// Acquire per-file lock for consistency with the other fix_* functions
	// (fix_renumber, fix_status_dir_mismatch, fix_orphan_in_progress, fix_unlink).
	// AutoCompleter may be internally safe, but a doctor --fix run that races
	// against another writer on the same spec should serialize on the file lock.
	fl := f.deps.FileLockFactory(specPath)
	if err := fl.Acquire(ctx, opts.FileLockTimeout); err != nil {
		failed = append(failed, FailedFix{
			Category:    finding.Category,
			TargetPaths: finding.TargetPaths,
			Detail:      "lock acquire failed: " + err.Error(),
		})
		return
	}
	defer func() {
		if err := fl.Release(ctx); err != nil {
			slog.Warn("doctor: file lock release failed", "path", specPath, "error", err.Error())
		}
	}()

	// Audit BEFORE AutoCompleter.CheckAndComplete: a CheckAndComplete failure
	// leaves the spec unchanged (still status=prompted); an audit failure
	// before any mutation leaves the operator with no orphan state.
	if err := WriteAuditEntry(ctx, opts.AuditLogPath, AuditEntry{
		Timestamp:   time.Time(f.deps.CurrentDateTimeGetter.Now()),
		Category:    finding.Category,
		Action:      "applied",
		TargetPaths: finding.TargetPaths,
		Before:      "status=prompted",
		After:       "status=verifying",
	}); err != nil {
		failed = append(failed, FailedFix{
			Category:    finding.Category,
			TargetPaths: finding.TargetPaths,
			Detail:      "audit log write failed: " + err.Error(),
		})
		return
	}

	if err := f.deps.AutoCompleter.CheckAndComplete(ctx, specID); err != nil {
		failed = append(failed, FailedFix{
			Category:    finding.Category,
			TargetPaths: finding.TargetPaths,
			Detail:      err.Error(),
		})
		return
	}

	applied = append(applied, AppliedFix{
		Category:    finding.Category,
		TargetPaths: finding.TargetPaths,
		FixCommand:  finding.FixCommand,
		AuditLine:   "",
	})
	return
}
