// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
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

		// Only cancellable statuses can be cancelled.
		switch prompt.PromptStatus(pf.Frontmatter.Status) {
		case prompt.ApprovedPromptStatus,
			prompt.ExecutingPromptStatus,
			prompt.FailedPromptStatus,
			prompt.InReviewPromptStatus,
			prompt.PendingVerificationPromptStatus:
			// allowed — safe to cancel
		default:
			skipped = append(skipped, SkippedFix{
				Category:    finding.Category,
				TargetPaths: []string{path},
				Detail:      "prompt no longer cancellable; current status=" + pf.Frontmatter.Status,
			})
			continue
		}

		pf.MarkCancelled()
		if err := pf.Save(ctx); err != nil {
			failed = append(failed, FailedFix{
				Category:    finding.Category,
				TargetPaths: []string{path},
				Detail:      "save failed: " + err.Error(),
			})
			continue
		}

		if err := f.deps.PromptManager.MoveToCancelled(ctx, path); err != nil {
			failed = append(failed, FailedFix{
				Category:    finding.Category,
				TargetPaths: []string{path},
				Detail:      "move to cancelled failed: " + err.Error(),
			})
			continue
		}

		if err := WriteAuditEntry(ctx, opts.AuditLogPath, AuditEntry{
			Timestamp:   time.Time(f.deps.CurrentDateTimeGetter.Now()),
			Category:    finding.Category,
			Action:      "applied",
			TargetPaths: []string{path},
			Before:      "status=" + pf.Frontmatter.Status,
			After:       "status=cancelled",
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
