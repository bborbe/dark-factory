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

	"github.com/bborbe/dark-factory/pkg/lock"
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

	// Acquire locks on every TargetPath BEFORE reindex (see helper for rationale).
	// `seen` deduplicates directory-scoped locks across the pre-reindex and post-reindex
	// acquire passes — flock is per-fd, so a second acquire on the same directory in
	// the same process would block until timeout (self-deadlock).
	preAcquiredLocks := make([]lock.DirLock, 0, len(finding.TargetPaths))
	seen := make(map[string]struct{})
	defer func() {
		for _, fl := range preAcquiredLocks {
			if err := fl.Release(ctx); err != nil {
				slog.Warn(
					"doctor: directory lock release failed (renumber cycle)",
					"error",
					err.Error(),
				)
			}
		}
	}()
	if ff := f.acquireOldPathLocks(ctx, &preAcquiredLocks, seen, finding, specDirs, opts.FileLockTimeout); ff != nil {
		failed = append(failed, *ff)
		return
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

	relevantRenames := filterRelevantRenames(renames, finding.TargetPaths, specDirs)
	if len(relevantRenames) == 0 {
		return
	}

	if ff := f.acquireNewPathLocks(ctx, &preAcquiredLocks, seen, finding, relevantRenames, opts.FileLockTimeout); ff != nil {
		failed = append(failed, *ff)
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
	// run, so the operator gets a recovery path.
	if _, refsErr := reindex.UpdateSpecRefs(ctx, renames, promptDirs, f.deps.Mover, f.deps.PromptManager); refsErr != nil {
		slog.Warn("doctor: UpdateSpecRefs failed (best-effort, continuing)",
			"error", refsErr.Error(),
			"renames", len(renames))
	}

	return
}

// acquireOldPathLocks locks the parent directory of every candidate TargetPath
// BEFORE reindex so a concurrent writer cannot mutate the colliding file between
// reindex's move and our subsequent load. Locks are held for the entire renumber
// cycle and released when fixDuplicateSpecNumbers returns. `seen` deduplicates
// by directory so the same dir is locked at most once (flock is per-fd; a second
// acquire on the same dir in the same process self-deadlocks).
func (f *fixer) acquireOldPathLocks(
	ctx context.Context,
	locks *[]lock.DirLock,
	seen map[string]struct{},
	finding Finding,
	specDirs []string,
	timeout time.Duration,
) *FailedFix {
	for _, t := range finding.TargetPaths {
		for _, dir := range specDirs {
			candidatePath := filepath.Join(dir, t)
			lockDir := filepath.Dir(candidatePath)
			if _, ok := seen[lockDir]; ok {
				continue
			}
			fl := f.deps.FileLockFactory(lockDir)
			if err := fl.Acquire(ctx, timeout); err != nil {
				return &FailedFix{
					Category:    finding.Category,
					TargetPaths: []string{candidatePath},
					Detail:      "lock acquire failed (pre-reindex): " + err.Error(),
				}
			}
			*locks = append(*locks, fl)
			seen[lockDir] = struct{}{}
		}
	}
	return nil
}

// acquireNewPathLocks locks the parent directory of every NewPath now that
// reindex's picks are known. The freed number slot is a fresh path with no
// pre-acquired lock; a concurrent writer could claim it between reindex and
// our Save. `seen` is shared with acquireOldPathLocks so a dir already locked
// from the old-path pass is not re-locked here.
func (f *fixer) acquireNewPathLocks(
	ctx context.Context,
	locks *[]lock.DirLock,
	seen map[string]struct{},
	finding Finding,
	renames []reindex.Rename,
	timeout time.Duration,
) *FailedFix {
	for _, rn := range renames {
		lockDir := filepath.Dir(rn.NewPath)
		if _, ok := seen[lockDir]; ok {
			continue
		}
		fl := f.deps.FileLockFactory(lockDir)
		if err := fl.Acquire(ctx, timeout); err != nil {
			return &FailedFix{
				Category:    finding.Category,
				TargetPaths: []string{rn.NewPath},
				Detail:      "lock acquire failed (post-reindex NewPath): " + err.Error(),
			}
		}
		*locks = append(*locks, fl)
		seen[lockDir] = struct{}{}
	}
	return nil
}

func filterRelevantRenames(
	renames []reindex.Rename,
	targetPaths, specDirs []string,
) []reindex.Rename {
	if len(renames) == 0 {
		return nil
	}
	oldPathSet := make(map[string]bool)
	for _, t := range targetPaths {
		for _, dir := range specDirs {
			oldPathSet[filepath.Join(dir, t)] = true
		}
	}
	var relevant []reindex.Rename
	for _, r := range renames {
		if oldPathSet[r.OldPath] {
			relevant = append(relevant, r)
		}
	}
	return relevant
}

func (f *fixer) applyDuplicateSpecNumbersRename(
	ctx context.Context,
	rn reindex.Rename,
	finding Finding,
	opts ApplyOptions,
) (applied *AppliedFix, failed *FailedFix) {
	// reindex.Reindex has already moved the file from OldPath → NewPath via the
	// shared FileMover before this method is called. We operate on NewPath here:
	// load the spec from its new location, record PreviousID in the frontmatter,
	// and save it back. The historical "MoveFile after Save" call at this layer
	// was a double-move that worked only against mocks (no-op) and failed against
	// real filesystems (Load couldn't find the already-moved file).
	//
	// Locking: the caller (fixDuplicateSpecNumbers) pre-acquired locks on every
	// candidate OldPath BEFORE reindex. Those locks are held across the entire
	// renumber cycle, so no per-rename NewPath lock is needed here — adding one
	// would risk a deadlock against another process holding NewPath's lock while
	// we hold OldPath's lock.
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

	// Write the audit entry BEFORE Save so a Save failure cannot leave a
	// durable mutation without an audit trail. If audit write fails, no
	// mutation occurs; if Save subsequently fails, the audit entry is the
	// evidence that the operator attempted the rename — the post-state can
	// be reconciled by reading the file (which still has the OLD content).
	now := time.Time(f.deps.CurrentDateTimeGetter.Now())
	entry := AuditEntry{
		Timestamp:   now,
		Category:    finding.Category,
		Action:      "applied",
		TargetPaths: []string{rn.OldPath, rn.NewPath},
		Before:      filepath.Base(rn.OldPath),
		After:       filepath.Base(rn.NewPath),
	}
	auditLine := FormatAuditLine(entry)
	if err := WriteAuditEntry(ctx, opts.AuditLogPath, entry); err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{rn.OldPath},
			Detail:      "audit log write failed: " + err.Error(),
		}
	}

	if err := sf.Save(ctx); err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{rn.OldPath, rn.NewPath},
			Detail:      "save failed: " + err.Error(),
		}
	}

	return &AppliedFix{
		Category:    finding.Category,
		TargetPaths: []string{rn.OldPath, rn.NewPath},
		FixCommand:  finding.FixCommand,
		AuditLine:   auditLine,
	}, nil
}
