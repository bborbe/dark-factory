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
	// finding.Detail contains "spec X not found; to fix: dark-factory prompt relink ..."
	// Extract the orphan spec ID from finding.SpecID.
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
		fl := f.deps.FileLockFactory(path)
		if err := fl.Acquire(ctx, opts.FileLockTimeout); err != nil {
			failed = append(failed, FailedFix{
				Category:    finding.Category,
				TargetPaths: []string{path},
				Detail:      "lock acquire failed: " + err.Error(),
			})
			continue
		}
		defer fl.Release(ctx)

		pf, err := f.deps.PromptManager.Load(ctx, path)
		if err != nil {
			failed = append(failed, FailedFix{
				Category:    finding.Category,
				TargetPaths: []string{path},
				Detail:      "load failed: " + err.Error(),
			})
			continue
		}

		// Build a new Specs list without the orphan.
		orphanNum := specnum.Parse(orphanSpecID)
		var newSpecs []string
		for _, s := range pf.Frontmatter.Specs {
			if specnum.Parse(s) != orphanNum {
				newSpecs = append(newSpecs, s)
			}
		}
		pf.Frontmatter.Specs = newSpecs

		if err := pf.Save(ctx); err != nil {
			failed = append(failed, FailedFix{
				Category:    finding.Category,
				TargetPaths: []string{path},
				Detail:      "save failed: " + err.Error(),
			})
			continue
		}

		before := "specs contained " + orphanSpecID
		after := "specs no longer contain " + orphanSpecID

		if err := WriteAuditEntry(ctx, opts.AuditLogPath, AuditEntry{
			Timestamp:   time.Time(f.deps.CurrentDateTimeGetter.Now()),
			Category:    finding.Category,
			Action:      "applied",
			TargetPaths: []string{path},
			Before:      before,
			After:       after,
		}); err != nil {
			failed = append(failed, FailedFix{
				Category:    finding.Category,
				TargetPaths: []string{path},
				Detail:      "audit log write failed: " + err.Error(),
			})
			continue
		}

		applied = append(applied, AppliedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			FixCommand:  finding.FixCommand,
			AuditLine:   "",
		})
	}

	return
}
