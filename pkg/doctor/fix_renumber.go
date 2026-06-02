// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/bborbe/dark-factory/pkg/reindex"
	"github.com/bborbe/dark-factory/pkg/spec"
	"github.com/bborbe/dark-factory/pkg/specnum"
)

// fixDuplicateSpecNumbers renames conflicting spec files to free numbers and
// updates linked prompt frontmatter references.
func (f *fixer) fixDuplicateSpecNumbers(
	ctx context.Context,
	finding Finding,
	opts ApplyOptions,
) (applied []AppliedFix, failed []FailedFix) {
	if len(finding.TargetPaths) == 0 {
		return
	}

	specDirs := []string{
		f.deps.SpecsInProgressDir,
		f.deps.SpecsInboxDir,
		f.deps.SpecsCompletedDir,
		f.deps.SpecsRejectedDir,
	}
	promptDirs := []string{
		f.deps.PromptsInProgressDir,
		f.deps.PromptsInboxDir,
		f.deps.PromptsCompletedDir,
		f.deps.PromptsCancelledDir,
	}

	r := reindex.NewReindexer(specDirs, f.deps.Mover)
	renames, err := r.Reindex(ctx)
	if err != nil {
		failed = append(failed, FailedFix{
			Category:    finding.Category,
			TargetPaths: finding.TargetPaths,
			Detail:      "reindex failed: " + err.Error(),
		})
		return
	}

	if len(renames) == 0 {
		return
	}

	oldPathSet := make(map[string]bool)
	for _, t := range finding.TargetPaths {
		for _, dir := range specDirs {
			path := filepath.Join(dir, t)
			oldPathSet[path] = true
		}
	}

	var relevantRenames []reindex.Rename
	for _, r := range renames {
		if oldPathSet[r.OldPath] {
			relevantRenames = append(relevantRenames, r)
		}
	}

	if len(relevantRenames) == 0 {
		return
	}

	for _, rn := range relevantRenames {
		af, ff := f.applyDuplicateSpecNumbersRename(ctx, rn, finding, opts)
		if af != nil {
			applied = append(applied, *af)
		}
		if ff != nil {
			failed = append(failed, *ff)
		}
	}

	// UpdateSpecRefs failures are best-effort: the renames have already succeeded
	// on disk, so the renumber itself is durable. Stale prompt spec-refs are
	// surfaced by the orphan-prompt-link detector on the next `dark-factory doctor`
	// run, so the operator gets a recovery path. Log the error so it is
	// diagnosable from daemon logs / CI output; do not propagate.
	if _, refsErr := reindex.UpdateSpecRefs(ctx, renames, promptDirs, f.deps.Mover, f.deps.PromptManager); refsErr != nil {
		slog.Warn("doctor: UpdateSpecRefs failed (best-effort, continuing)",
			"error", refsErr.Error(),
			"renames", len(renames))
	}

	return
}

func (f *fixer) applyDuplicateSpecNumbersRename(
	ctx context.Context,
	rn reindex.Rename,
	finding Finding,
	opts ApplyOptions,
) (applied *AppliedFix, failed *FailedFix) {
	// reindex.Reindex has already moved the file from OldPath → NewPath via the
	// shared FileMover before this method is called. We operate on NewPath here:
	// lock it, load the spec from its new location, record PreviousID in the
	// frontmatter, and save it back. The historical "MoveFile after Save" call
	// at this layer was a double-move that worked only against mocks (no-op) and
	// failed against real filesystems (Load couldn't find the already-moved file).
	fl := f.deps.FileLockFactory(rn.NewPath)
	if err := fl.Acquire(ctx, opts.FileLockTimeout); err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{rn.OldPath, rn.NewPath},
			Detail:      "lock acquire failed: " + err.Error(),
		}
	}
	defer fl.Release(ctx)

	oldNum := specnum.Parse(strings.TrimSuffix(filepath.Base(rn.OldPath), ".md"))
	sf, err := spec.Load(ctx, rn.NewPath, f.deps.CurrentDateTimeGetter)
	if err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{rn.OldPath, rn.NewPath},
			Detail:      "load failed: " + err.Error(),
		}
	}

	sf.Frontmatter.PreviousID = fmt.Sprintf("%03d", oldNum)
	if err := sf.Save(ctx); err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{rn.OldPath, rn.NewPath},
			Detail:      "save failed: " + err.Error(),
		}
	}

	auditLine := renderAuditLine(
		time.Time(f.deps.CurrentDateTimeGetter.Now()),
		finding,
		rn.OldPath,
		rn.NewPath,
	)
	if err := WriteAuditEntry(ctx, opts.AuditLogPath, AuditEntry{
		Timestamp:   time.Time(f.deps.CurrentDateTimeGetter.Now()),
		Category:    finding.Category,
		Action:      "applied",
		TargetPaths: []string{rn.OldPath, rn.NewPath},
		Before:      filepath.Base(rn.OldPath),
		After:       filepath.Base(rn.NewPath),
	}); err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{rn.OldPath},
			Detail:      "audit log write failed: " + err.Error(),
		}
	}

	return &AppliedFix{
		Category:    finding.Category,
		TargetPaths: []string{rn.OldPath, rn.NewPath},
		FixCommand:  finding.FixCommand,
		AuditLine:   auditLine,
	}, nil
}

func renderAuditLine(now time.Time, finding Finding, before, after string) string {
	return fmt.Sprintf(
		"%s\t%s\tapplied\t%s %s\t%s\t%s\n",
		now.Format(time.RFC3339),
		finding.Category,
		before,
		after,
		filepath.Base(before),
		filepath.Base(after),
	)
}
