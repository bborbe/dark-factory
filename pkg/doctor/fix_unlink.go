// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"time"

	"github.com/bborbe/dark-factory/pkg/specnum"
)

// fixOrphanPromptLink removes the orphan spec reference from the prompt's frontmatter.
func (f *fixer) fixOrphanPromptLink(
	ctx context.Context,
	finding Finding,
	opts ApplyOptions,
) (applied []AppliedFix, failed []FailedFix) {
	orphanSpecID := finding.SpecID
	if orphanSpecID == "" {
		failed = append(failed, FailedFix{
			Category:    finding.Category,
			TargetPaths: finding.TargetPaths,
			Detail:      "missing spec ID in finding",
		})
		return
	}

	for _, path := range finding.TargetPaths {
		af, ff := f.applyOrphanPromptLinkPath(ctx, path, orphanSpecID, finding, opts)
		if af != nil {
			applied = append(applied, *af)
		}
		if ff != nil {
			failed = append(failed, *ff)
		}
	}

	return
}

func (f *fixer) applyOrphanPromptLinkPath(
	ctx context.Context,
	path string,
	orphanSpecID string,
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

	pf, err := f.deps.PromptManager.Load(ctx, path)
	if err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "load failed: " + err.Error(),
		}
	}

	orphanNum := specnum.Parse(orphanSpecID)
	var newSpecs []string
	for _, s := range pf.Frontmatter.Specs {
		if specnum.Parse(s) != orphanNum {
			newSpecs = append(newSpecs, s)
		}
	}
	pf.Frontmatter.Specs = newSpecs

	before := "specs contained " + orphanSpecID
	after := "specs no longer contain " + orphanSpecID

	// Audit BEFORE pf.Save: audit failure leaves the in-memory pf change
	// unsaved (no on-disk mutation); save failure after audit leaves the
	// audit recording the intent + a FailedFix in the result.
	if err := WriteAuditEntry(ctx, opts.AuditLogPath, AuditEntry{
		Timestamp:   time.Time(f.deps.CurrentDateTimeGetter.Now()),
		Category:    finding.Category,
		Action:      "applied",
		TargetPaths: []string{path},
		Before:      before,
		After:       after,
	}); err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "audit log write failed: " + err.Error(),
		}
	}

	if err := pf.Save(ctx); err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "save failed: " + err.Error(),
		}
	}

	return &AppliedFix{
		Category:    finding.Category,
		TargetPaths: []string{path},
		FixCommand:  finding.FixCommand,
		AuditLine:   "",
	}, nil
}
