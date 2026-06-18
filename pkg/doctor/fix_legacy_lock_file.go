// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// fixLegacyLockFile removes *.lock sidecar files left by the abandoned per-file
// locking scheme (spec 097). No directory lock is acquired — removing a stale
// empty sidecar needs no coordination.
func (f *fixer) fixLegacyLockFile(
	ctx context.Context,
	finding Finding,
	opts ApplyOptions,
) (applied []AppliedFix, failed []FailedFix) {
	for _, path := range finding.TargetPaths {
		af, ff := f.applyLegacyLockFilePath(ctx, path, finding, opts)
		if af != nil {
			applied = append(applied, *af)
		}
		if ff != nil {
			failed = append(failed, *ff)
		}
	}
	return
}

func (f *fixer) applyLegacyLockFilePath(
	ctx context.Context,
	path string,
	finding Finding,
	opts ApplyOptions,
) (applied *AppliedFix, failed *FailedFix) {
	// Audit BEFORE the mutation so a remove failure still leaves a record of intent.
	if err := WriteAuditEntry(ctx, opts.AuditLogPath, AuditEntry{
		Timestamp:   time.Time(f.deps.CurrentDateTimeGetter.Now()),
		Category:    finding.Category,
		Action:      "applied",
		TargetPaths: []string{path},
		Before:      filepath.Base(path),
		After:       "removed",
	}); err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "audit log write failed: " + err.Error(),
		}
	}

	if err := os.Remove(path); err != nil {
		// Idempotent: a file already gone (concurrent doctor run / re-run) is success, not failure.
		if os.IsNotExist(err) {
			return &AppliedFix{
				Category:    finding.Category,
				TargetPaths: []string{path},
				FixCommand:  finding.FixCommand,
			}, nil
		}
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "remove failed: " + err.Error(),
		}
	}

	return &AppliedFix{
		Category:    finding.Category,
		TargetPaths: []string{path},
		FixCommand:  finding.FixCommand,
	}, nil
}
