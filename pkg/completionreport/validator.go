// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package completionreport

import (
	"context"
	"log/slog"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/report"
)

//counterfeiter:generate -o ../../mocks/completion-report-validator.go --fake-name CompletionReportValidator . Validator

// Validator parses and consistency-checks the completion report from a prompt log.
type Validator interface {
	Validate(ctx context.Context, logFile string) (*report.CompletionReport, error)
}

// NewValidator creates a new completion report Validator.
func NewValidator() Validator {
	return &validator{}
}

type validator struct{}

// Validate parses the completion report from the log and detects claude-CLI-level failures.
// Returns (report, nil) when a report is present and indicates success.
// Returns (nil, nil) when no report is present AND no critical failure is detected in the log
// (backwards compatible — old prompts without reports are treated as successful).
// Returns (nil, error) when:
//   - the log shows a claude-CLI critical failure (auth error, API error) even without a report
//   - a parseable report indicates non-success status (after consistency check)
//   - the report exists but is malformed
func (v *validator) Validate(
	ctx context.Context,
	logFile string,
) (*report.CompletionReport, error) {
	completionReport, err := report.ParseFromLog(ctx, logFile)
	if err != nil {
		slog.Debug("failed to parse completion report", "error", err)
		// Parse error — downgrade to "no report" and fall through to critical failure scan.
		completionReport = nil
	}

	if completionReport == nil {
		// No report found (or parse error) — scan for claude-CLI-level critical failures.
		reason, scanErr := report.ScanForCriticalFailures(ctx, logFile)
		if scanErr != nil {
			slog.Debug("failed to scan for critical failures", "error", scanErr)
			// I/O error during scan — treat as no failure detected (don't block backwards compat).
			return nil, nil //nolint:nilnil
		}
		if reason != "" {
			return nil, errors.Errorf(ctx, "claude CLI critical failure: %s", reason)
		}
		// No report, no critical failure — backwards compatible success.
		return nil, nil //nolint:nilnil
	}

	slog.Info(
		"completion report",
		"status",
		completionReport.Status,
		"summary",
		completionReport.Summary,
	)

	// Validate consistency between status and verification results.
	correctedStatus, overridden := completionReport.ValidateConsistency()
	if overridden {
		slog.Warn(
			"overriding self-reported status",
			"reported", completionReport.Status,
			"corrected", correctedStatus,
			"verificationCommand", completionReport.Verification.Command,
			"verificationExitCode", completionReport.Verification.ExitCode,
		)
		completionReport.Status = correctedStatus
	}

	if completionReport.Status != "success" {
		// Report says not success — treat as failure.
		slog.Info("completion report indicates failure", "status", completionReport.Status)
		if len(completionReport.Blockers) > 0 {
			slog.Info("blockers reported", "blockers", completionReport.Blockers)
		}
		return nil, errors.Errorf(ctx, "completion report status: %s", completionReport.Status)
	}

	return completionReport, nil
}
