// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"log/slog"
	"time"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

// fixOrphanInProgressPrompt cancels prompts that link to completed or rejected specs.
// Only "cancel" is applied (never "complete"), as cancel is safe and reversible.
func (f *fixer) fixOrphanInProgressPrompt(
	ctx context.Context,
	finding Finding,
	opts ApplyOptions,
) (applied []AppliedFix, skipped []SkippedFix, failed []FailedFix) {
	for _, path := range finding.TargetPaths {
		af, sf, ff := f.applyOrphanInProgressPath(ctx, path, finding, opts)
		if af != nil {
			applied = append(applied, *af)
		}
		if sf != nil {
			skipped = append(skipped, *sf)
		}
		if ff != nil {
			failed = append(failed, *ff)
		}
	}
	return
}

func (f *fixer) applyOrphanInProgressPath(
	ctx context.Context,
	path string,
	finding Finding,
	opts ApplyOptions,
) (applied *AppliedFix, skipped *SkippedFix, failed *FailedFix) {
	fl := f.deps.FileLockFactory(path)
	if err := fl.Acquire(ctx, opts.FileLockTimeout); err != nil {
		return nil, nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "lock acquire failed: " + err.Error(),
		}
	}
	defer func() {
		if err := fl.Release(ctx); err != nil {
			slog.Warn("doctor: file lock release failed", "path", path, "error", err.Error())
		}
	}()

	pf, err := f.deps.PromptManager.Load(ctx, path)
	if err != nil {
		return nil, nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "load failed: " + err.Error(),
		}
	}

	switch prompt.PromptStatus(pf.Frontmatter.Status) {
	case prompt.ApprovedPromptStatus,
		prompt.ExecutingPromptStatus,
		prompt.FailedPromptStatus,
		prompt.InReviewPromptStatus,
		prompt.PendingVerificationPromptStatus:
	default:
		return nil, &SkippedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "prompt no longer cancellable; current status=" + pf.Frontmatter.Status,
		}, nil
	}

	// Write audit BEFORE the mutating sequence. If audit fails, no mutation;
	// if subsequent mutation fails, the audit shows intent + a FailedFix is
	// returned, leaving the operator with a reconcilable trail.
	beforeStatus := pf.Frontmatter.Status
	if err := WriteAuditEntry(ctx, opts.AuditLogPath, AuditEntry{
		Timestamp:   time.Time(f.deps.CurrentDateTimeGetter.Now()),
		Category:    finding.Category,
		Action:      "applied",
		TargetPaths: []string{path},
		Before:      "status=" + beforeStatus,
		After:       "status=cancelled",
	}); err != nil {
		return nil, nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "audit log write failed: " + err.Error(),
		}
	}

	pf.MarkCancelled()
	if err := pf.Save(ctx); err != nil {
		return nil, nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "save failed: " + err.Error(),
		}
	}

	if err := f.deps.PromptManager.MoveToCancelled(ctx, path); err != nil {
		return nil, nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "move to cancelled failed: " + err.Error(),
		}
	}

	return &AppliedFix{
		Category:    finding.Category,
		TargetPaths: []string{path},
		FixCommand:  finding.FixCommand,
		AuditLine:   "",
	}, nil, nil
}
