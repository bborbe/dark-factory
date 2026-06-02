// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queuescanner

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/failurehandler"
	"github.com/bborbe/dark-factory/pkg/preflightconditions"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/queue-scanner.go --fake-name QueueScanner . Scanner
//counterfeiter:generate -o ../../mocks/prompt-processor.go --fake-name PromptProcessor . PromptProcessor
//counterfeiter:generate -o ../../mocks/queue-scanner-prompt-manager.go --fake-name QueueScannerPromptManager . PromptManager

// PromptProcessor executes a single prompt end-to-end.
// Implemented by *processor (avoids a cycle: scanner depends on processor's per-prompt entrypoint).
type PromptProcessor interface {
	ProcessPrompt(ctx context.Context, pr prompt.Prompt) error
}

// PromptManager is the minimal subset this package needs.
// Defined locally to avoid an import cycle (pkg/processor imports queuescanner).
type PromptManager interface {
	ListQueued(ctx context.Context) ([]prompt.Prompt, error)
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
	AllPreviousCompleted(ctx context.Context, n int) bool
	FindMissingCompleted(ctx context.Context, n int) []int
	FindPromptStatusInProgress(ctx context.Context, number int) string
	SetStatus(ctx context.Context, path string, status string) error
	// Per-spec predecessor lookup (spec 092)
	AllPreviousInSpecCompleted(ctx context.Context, n int, specID string) bool
	FindMissingInSpecCompleted(ctx context.Context, n int, specID string) (int, error)
}

// Scanner drives the queue-scan loop: list queued, validate, dispatch to PromptProcessor, handle blockers.
type Scanner interface {
	// ScanAndProcess returns the count of prompts that completed during this scan.
	// The count feeds into the post-#337 NothingToDoCallback (no-progress detection in one-shot mode).
	ScanAndProcess(ctx context.Context) (completed int, err error)
	// HasPendingVerification returns true if any prompt in the queue dir has pending_verification status.
	HasPendingVerification(ctx context.Context) bool
	// ClearSkippedCache clears the skip cache so all files are re-evaluated on the next scan.
	// Called by the processor on fsnotify wakeup events.
	ClearSkippedCache()
}

// scanner implements Scanner.
type scanner struct {
	promptManager   PromptManager
	promptProcessor PromptProcessor
	failureHandler  failurehandler.Handler
	queueDir        string
	lastBlockedMsg  string
	skippedPrompts  map[string]libtime.DateTime // filename → mod time when skipped
}

// NewScanner creates a new Scanner.
func NewScanner(
	promptManager PromptManager,
	promptProcessor PromptProcessor,
	failureHandler failurehandler.Handler,
	queueDir string,
) Scanner {
	return &scanner{
		promptManager:   promptManager,
		promptProcessor: promptProcessor,
		failureHandler:  failureHandler,
		queueDir:        queueDir,
		skippedPrompts:  make(map[string]libtime.DateTime),
	}
}

// ClearSkippedCache clears the skip cache so all files are re-evaluated on the next scan.
func (s *scanner) ClearSkippedCache() {
	s.skippedPrompts = make(map[string]libtime.DateTime)
}

// ScanAndProcess scans for and processes any existing queued prompts.
// Returns the count of prompts successfully processed and any fatal error.
func (s *scanner) ScanAndProcess(ctx context.Context) (int, error) {
	if s.HasPendingVerification(ctx) {
		slog.Info("queue blocked: prompt pending verification")
		return 0, nil
	}

	completed := 0
	for {
		select {
		case <-ctx.Done():
			return completed, nil
		default:
		}

		done, err := s.processSingleQueued(ctx)
		if err != nil {
			return completed, err
		}
		if done {
			return completed, nil
		}
		completed++
	}
}

// processSingleQueued picks the next queued prompt and processes it.
// Returns (true, nil) when the scan loop should stop (queue empty, blocked, or preflight broken).
// Returns (false, nil) to continue scanning for the next prompt.
// Returns (true, err) when a fatal error requires the daemon to stop.
//
//nolint:gocognit // per-spec filter loop + multi-stage guard checks + log gating; refactor candidate tracked separately
func (s *scanner) processSingleQueued(ctx context.Context) (bool, error) {
	queued, err := s.promptManager.ListQueued(ctx)
	if err != nil {
		return true, errors.Wrap(ctx, err, "list queued prompts")
	}

	if len(queued) == 0 {
		slog.Debug("queue scan complete", "queuedCount", 0)
		return true, nil
	}

	slog.Debug("queue scan complete", "queuedCount", len(queued))

	// Determinism: ListQueued already returns entries sorted alphabetically by
	// filename (pkg/prompt/prompt.go:1021). For prompts with a fixed-width
	// numeric prefix this corresponds to numeric order, so the alphabetic
	// order resolves cross-spec ties by lowest global prompt number.

	var pr prompt.Prompt
	skipped := false
	for _, candidate := range queued {
		if err := s.autoSetQueuedStatus(ctx, &candidate); err != nil {
			return true, errors.Wrap(ctx, err, "auto-set queued status")
		}
		if s.shouldSkipPrompt(ctx, candidate) {
			skipped = true
			continue
		}
		specID, err := s.readSpecID(ctx, candidate)
		if err != nil {
			// Malformed prompt frontmatter — treat as blocked, surface via logBlockedOnce
			s.logBlockedOnce(ctx, candidate, "", "malformed frontmatter", "")
			return true, nil
		}
		if specID == "" {
			// No spec field — fall back to global guard. Prompts without a spec
			// field use the legacy global predecessor guard.
			if s.promptManager.AllPreviousCompleted(ctx, candidate.Number()) {
				pr = candidate
				break
			}
			continue
		}
		if s.promptManager.AllPreviousInSpecCompleted(ctx, candidate.Number(), specID) {
			pr = candidate
			break
		}
		// Blocked: log once with spec id, then return DONE
		missing, _ := s.promptManager.FindMissingInSpecCompleted(ctx, candidate.Number(), specID)
		s.logBlockedOnce(
			ctx,
			candidate,
			specID,
			"previous prompt not completed",
			missingStr(missing),
		)
		return true, nil
	}
	if pr.Path == "" {
		// If at least one candidate was skipped, re-poll to allow other prompts
		// to be picked up on the next cycle. Otherwise no candidate is ready.
		return !skipped, nil
	}

	s.lastBlockedMsg = ""

	slog.Info("found queued prompt", "file", filepath.Base(pr.Path))

	if err := s.promptProcessor.ProcessPrompt(ctx, pr); err != nil {
		if stderrors.Is(err, preflightconditions.ErrPreflightFailed) {
			// Baseline is broken — propagate so the runner terminates dark-factory.
			return false, err
		}
		if stopErr := s.failureHandler.Handle(ctx, pr.Path, err); stopErr != nil {
			return true, stopErr
		}
		return false, nil // re-queued or permanently failed — process next prompt
	}

	slog.Info("watching for queued prompts", "dir", s.queueDir)
	return false, nil
}

// readSpecID loads the prompt and returns its spec id. If the frontmatter has
// no spec field, returns ("", nil) so the scanner can fall back to the global
// guard. If the frontmatter has more than one spec id, returns an error — the
// spec does not define a tie-break, so we fail closed. A Load error is also
// treated as "no spec field" so the scanner falls back to the legacy global
// guard instead of refusing to advance the queue on a transient read failure.
func (s *scanner) readSpecID(ctx context.Context, pr prompt.Prompt) (string, error) {
	pf, err := s.promptManager.Load(ctx, pr.Path)
	if err != nil || pf == nil {
		return "", nil
	}
	specs := pf.Frontmatter.Specs
	if len(specs) == 0 {
		return "", nil
	}
	if len(specs) > 1 {
		return "", errors.Errorf(
			ctx,
			"multi-spec prompt: tie-breaking unspecified (specs=%v)",
			[]string(specs),
		)
	}
	return string(specs[0]), nil
}

// missingStr formats a missing prompt number for log output. Empty input
// returns "".
func missingStr(missing int) string {
	if missing < 0 {
		return ""
	}
	return fmt.Sprintf("%03d", missing)
}

// shouldSkipPrompt checks if a prompt should be skipped due to validation failure.
// Returns true if the prompt should be skipped, false if it's ready to process.
func (s *scanner) shouldSkipPrompt(ctx context.Context, pr prompt.Prompt) bool {
	fileInfo, err := os.Stat(pr.Path)
	if err == nil {
		if lastSkipped, wasSkipped := s.skippedPrompts[pr.Path]; wasSkipped {
			if fileInfo.ModTime().Equal(time.Time(lastSkipped)) {
				slog.Debug(
					"skipping previously-failed prompt (unchanged)",
					"file",
					filepath.Base(pr.Path),
				)
				return true
			}
			delete(s.skippedPrompts, pr.Path)
		}
	}

	if err := pr.ValidateForExecution(ctx); err != nil {
		slog.Warn("skipping prompt", "file", filepath.Base(pr.Path), "reason", err.Error())
		if fileInfo != nil {
			s.skippedPrompts[pr.Path] = libtime.DateTime(fileInfo.ModTime())
		}
		return true
	}

	return false
}

// logBlockedOnce logs the "prompt blocked" message only when the missing-prompt details change,
// suppressing repeated identical messages on every poll cycle.
func (s *scanner) logBlockedOnce(
	ctx context.Context,
	pr prompt.Prompt,
	specID string,
	reason string,
	missing string,
) {
	key := filepath.Base(pr.Path) + "|" + specID + "|" + reason + "|" + missing
	if key == s.lastBlockedMsg {
		return
	}
	slog.InfoContext(
		ctx,
		"prompt blocked",
		"file", filepath.Base(pr.Path),
		"reason", reason,
		"spec", specID,
		"missing", missing,
	)
	s.lastBlockedMsg = key
}

// autoSetQueuedStatus sets status to "queued" for any non-terminal status.
// This makes the folder location the source of truth - if a file is in queue/, it should be queued.
func (s *scanner) autoSetQueuedStatus(ctx context.Context, pr *prompt.Prompt) error {
	switch pr.Status {
	case prompt.ApprovedPromptStatus,
		prompt.ExecutingPromptStatus,
		prompt.CompletedPromptStatus,
		prompt.FailedPromptStatus,
		prompt.PendingVerificationPromptStatus,
		prompt.CancelledPromptStatus:
		return nil
	}
	baseName := filepath.Base(pr.Path)
	previousStatus := pr.Status
	slog.Info(
		"auto-setting status to approved",
		"file",
		baseName,
		"previousStatus",
		previousStatus,
	)
	if err := s.promptManager.SetStatus(ctx, pr.Path, string(prompt.ApprovedPromptStatus)); err != nil {
		return errors.Wrap(ctx, err, "set status to approved")
	}
	pr.Status = prompt.ApprovedPromptStatus
	return nil
}

// HasPendingVerification returns true if any prompt in queueDir has pending_verification status.
func (s *scanner) HasPendingVerification(ctx context.Context) bool {
	entries, err := os.ReadDir(s.queueDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		pf, err := s.promptManager.Load(ctx, filepath.Join(s.queueDir, entry.Name()))
		if err != nil || pf == nil {
			continue
		}
		if pf.Frontmatter.Status == string(prompt.PendingVerificationPromptStatus) {
			return true
		}
	}
	return false
}
