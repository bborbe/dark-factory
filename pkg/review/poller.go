// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package review

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/review_poller.go --fake-name ReviewPoller . ReviewPoller

// ReviewPoller watches all in_review prompts, fetches GitHub review state,
// generates fix prompts on request-changes, and triggers merge on approval.
//
//nolint:revive // ReviewPoller is the intended name per spec requirements
type ReviewPoller interface {
	Run(ctx context.Context) error
}

// NewReviewPoller creates a new ReviewPoller.
func NewReviewPoller(
	queueDir string,
	inboxDir string,
	allowedReviewers []string,
	maxRetries int,
	pollInterval time.Duration,
	fetcher git.ReviewFetcher,
	prMerger git.PRMerger,
	promptManager prompt.Manager,
	generator FixPromptGenerator,
) ReviewPoller {
	return &reviewPoller{
		queueDir:         queueDir,
		inboxDir:         inboxDir,
		allowedReviewers: allowedReviewers,
		maxRetries:       maxRetries,
		pollInterval:     pollInterval,
		fetcher:          fetcher,
		prMerger:         prMerger,
		promptManager:    promptManager,
		generator:        generator,
	}
}

// reviewPoller implements ReviewPoller.
type reviewPoller struct {
	queueDir         string
	inboxDir         string
	allowedReviewers []string
	maxRetries       int
	pollInterval     time.Duration
	fetcher          git.ReviewFetcher
	prMerger         git.PRMerger
	promptManager    prompt.Manager
	generator        FixPromptGenerator
}

// Run loops until ctx is cancelled, polling in_review prompts on each iteration.
func (p *reviewPoller) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		p.pollOnce(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(p.pollInterval):
		}
	}
}

// pollOnce scans queueDir for in_review prompts and processes each one.
func (p *reviewPoller) pollOnce(ctx context.Context) {
	paths, err := p.listInReview(ctx)
	if err != nil {
		slog.Warn("failed to list in-review prompts", "error", err)
		return
	}
	for _, path := range paths {
		p.processPrompt(ctx, path)
	}
}

// listInReview returns paths of all in_review prompt files in queueDir.
func (p *reviewPoller) listInReview(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(p.queueDir)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "read queue dir")
	}
	var result []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(p.queueDir, entry.Name())
		fm, err := p.promptManager.ReadFrontmatter(ctx, path)
		if err != nil {
			slog.Warn("failed to read frontmatter", "file", entry.Name(), "error", err)
			continue
		}
		if prompt.PromptStatus(fm.Status) == prompt.InReviewPromptStatus {
			result = append(result, path)
		}
	}
	return result, nil
}

// processPrompt handles a single in_review prompt: checks PR state, fetches review verdict,
// and takes appropriate action (merge, generate fix, or mark failed).
func (p *reviewPoller) processPrompt(ctx context.Context, path string) {
	pf, err := p.promptManager.Load(ctx, path)
	if err != nil {
		slog.Warn("failed to load prompt", "file", filepath.Base(path), "error", err)
		return
	}

	prURL := pf.PRURL()
	if prURL == "" {
		slog.Warn("prompt has no PR URL, skipping", "file", filepath.Base(path))
		return
	}

	state, err := p.fetcher.FetchPRState(ctx, prURL)
	if err != nil {
		slog.Warn("failed to fetch PR state", "file", filepath.Base(path), "error", err)
		return
	}

	switch state {
	case "MERGED":
		if err := p.promptManager.MoveToCompleted(ctx, path); err != nil {
			slog.Warn(
				"failed to move merged prompt to completed",
				"file",
				filepath.Base(path),
				"error",
				err,
			)
		}
		return
	case "CLOSED":
		if err := p.promptManager.SetStatus(ctx, path, string(prompt.FailedPromptStatus)); err != nil {
			slog.Warn(
				"failed to set closed prompt to failed",
				"file",
				filepath.Base(path),
				"error",
				err,
			)
		}
		return
	}

	reviewResult, err := p.fetcher.FetchLatestReview(ctx, prURL, p.allowedReviewers)
	if err != nil {
		slog.Warn("failed to fetch latest review", "file", filepath.Base(path), "error", err)
		return
	}

	switch reviewResult.Verdict {
	case git.ReviewVerdictNone:
		// No trusted review yet — nothing to do.
	case git.ReviewVerdictApproved:
		p.handleApproved(ctx, path, prURL)
	case git.ReviewVerdictChangesRequested:
		p.handleChangesRequested(ctx, path, prURL, pf, reviewResult.Body)
	}
}

// handleApproved merges the PR and marks the prompt as completed.
func (p *reviewPoller) handleApproved(ctx context.Context, path string, prURL string) {
	if err := p.prMerger.WaitAndMerge(ctx, prURL); err != nil {
		slog.Warn("failed to merge PR", "file", filepath.Base(path), "error", err)
		return
	}
	if err := p.promptManager.MoveToCompleted(ctx, path); err != nil {
		slog.Warn(
			"failed to move approved prompt to completed",
			"file",
			filepath.Base(path),
			"error",
			err,
		)
	}
}

// handleChangesRequested either marks the prompt as failed (retry limit reached)
// or generates a fix prompt and increments the retry count.
func (p *reviewPoller) handleChangesRequested(
	ctx context.Context,
	path string,
	prURL string,
	pf *prompt.PromptFile,
	reviewBody string,
) {
	retryCount := pf.RetryCount()
	if retryCount >= p.maxRetries {
		slog.Warn("retry limit reached", "file", filepath.Base(path), "retryCount", retryCount)
		if err := p.promptManager.SetStatus(ctx, path, string(prompt.FailedPromptStatus)); err != nil {
			slog.Warn("failed to set failed status", "file", filepath.Base(path), "error", err)
		}
		return
	}

	opts := GenerateOpts{
		InboxDir:           p.inboxDir,
		OriginalPromptName: filepath.Base(path),
		Branch:             pf.Branch(),
		PRURL:              prURL,
		RetryCount:         retryCount + 1,
		ReviewBody:         reviewBody,
	}
	if err := p.generator.Generate(ctx, opts); err != nil {
		slog.Warn("failed to generate fix prompt", "file", filepath.Base(path), "error", err)
		return
	}
	if err := p.promptManager.IncrementRetryCount(ctx, path); err != nil {
		slog.Warn("failed to increment retry count", "file", filepath.Base(path), "error", err)
	}
}
