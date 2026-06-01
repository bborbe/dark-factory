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
	// The Detail field encodes the contradiction. e.g.:
	//   "spec in specs/in-progress/ has status completed ..."
	//   "prompt in prompts/in-progress/ has status completed ..."
	// We parse the FixCommand prefix to determine whether this is a spec or prompt.
	fixCmd := finding.FixCommand
	var expectedDir string
	var filename string

	if strings.HasPrefix(fixCmd, "dark-factory spec move ") {
		specID := strings.TrimPrefix(fixCmd, "dark-factory spec move ")
		// Determine expected dir from the finding's Detail by parsing the mismatch description.
		// The Detail for specs is: "spec in X/ has status Y but only statuses {Z} are allowed in that directory"
		expectedDir = f.expectedSpecDir(finding.Detail)
		if expectedDir == "" {
			failed = append(failed, FailedFix{
				Category:    finding.Category,
				TargetPaths: finding.TargetPaths,
				Detail:      "could not determine expected directory from detail",
			})
			return
		}
		filename = specID + ".md"
	} else if strings.HasPrefix(fixCmd, "dark-factory prompt move ") {
		promptID := strings.TrimPrefix(fixCmd, "dark-factory prompt move ")
		filename = promptID + ".md"
		expectedDir = f.expectedPromptDir(finding.Detail)
		if expectedDir == "" {
			failed = append(failed, FailedFix{
				Category:    finding.Category,
				TargetPaths: finding.TargetPaths,
				Detail:      "could not determine expected directory from detail",
			})
			return
		}
	} else {
		failed = append(failed, FailedFix{
			Category:    finding.Category,
			TargetPaths: finding.TargetPaths,
			Detail:      "unknown FixCommand: " + fixCmd,
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

		// Ensure expected dir exists.
		if err := os.MkdirAll(expectedDir, 0750); err != nil {
			failed = append(failed, FailedFix{
				Category:    finding.Category,
				TargetPaths: []string{path},
				Detail:      "create directory failed: " + err.Error(),
			})
			continue
		}

		dest := filepath.Join(expectedDir, filename)
		if err := os.Rename(path, dest); err != nil {
			failed = append(failed, FailedFix{
				Category:    finding.Category,
				TargetPaths: []string{path},
				Detail:      "rename failed: " + err.Error(),
			})
			continue
		}

		if err := WriteAuditEntry(ctx, opts.AuditLogPath, AuditEntry{
			Timestamp:   time.Time(f.deps.CurrentDateTimeGetter.Now()),
			Category:    finding.Category,
			Action:      "applied",
			TargetPaths: []string{path, dest},
			Before:      filepath.Base(path),
			After:       filepath.Base(dest),
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
			TargetPaths: []string{path, dest},
			FixCommand:  finding.FixCommand,
			AuditLine:   "",
		})
	}

	return
}

// expectedSpecDir determines the correct spec directory from the Detail string.
// The Detail for specs always names the current (wrong) directory and the allowed statuses.
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
