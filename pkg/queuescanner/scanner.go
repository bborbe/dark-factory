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

// scanner implements Scanner.
type scanner struct {
	promptManager   PromptManager
	promptProcessor PromptProcessor
	failureHandler  failurehandler.Handler
	queueDir        string
	lastBlockedMsg  string
	skippedPrompts  map[string]libtime.DateTime // filename → mod time when skipped
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

	pr := queued[0]

	if err := s.autoSetQueuedStatus(ctx, &pr); err != nil {
		return true, errors.Wrap(ctx, err, "auto-set queued status")
	}

	if s.shouldSkipPrompt(ctx, pr) {
		return false, nil
	}

	if !s.promptManager.AllPreviousCompleted(ctx, pr.Number()) {
		s.logBlockedOnce(ctx, pr)
		return true, nil // blocked — wait for watcher signal or periodic scan
	}
	s.lastBlockedMsg = ""

	slog.Info("found queued prompt", "file", filepath.Base(pr.Path))

	if err := s.promptProcessor.ProcessPrompt(ctx, pr); err != nil {
		if stderrors.Is(err, preflightconditions.ErrPreflightSkip) {
			// Baseline is broken — exit scan loop and wait for next 5s tick.
			return true, nil
		}
		if stopErr := s.failureHandler.Handle(ctx, pr.Path, err); stopErr != nil {
			return true, stopErr
		}
		return false, nil // re-queued or permanently failed — process next prompt
	}

	slog.Info("watching for queued prompts", "dir", s.queueDir)
	return false, nil
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
func (s *scanner) logBlockedOnce(ctx context.Context, pr prompt.Prompt) {
	missing := s.promptManager.FindMissingCompleted(ctx, pr.Number())
	details := make([]string, 0, len(missing))
	for _, num := range missing {
		status := s.promptManager.FindPromptStatusInProgress(ctx, num)
		if status != "" {
			details = append(details, fmt.Sprintf("%03d(%s)", num, status))
		} else {
			details = append(details, fmt.Sprintf("%03d(not found)", num))
		}
	}
	msg := strings.Join(details, ", ")
	if msg == s.lastBlockedMsg {
		return
	}
	slog.Info(
		"prompt blocked",
		"file", filepath.Base(pr.Path),
		"reason", "previous prompt not completed",
		"missing", msg,
	)
	s.lastBlockedMsg = msg
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
