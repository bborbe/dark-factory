// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"time"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/subproc"
)

//counterfeiter:generate -o ../../mocks/pr_merger.go --fake-name PRMerger . PRMerger

// PRMerger watches a PR until mergeable and merges it.
type PRMerger interface {
	WaitAndMerge(ctx context.Context, prURL string) error
}

// NewPRMerger creates a new PRMerger.
func NewPRMerger(ghToken string, currentDateTimeGetter libtime.CurrentDateTimeGetter) PRMerger {
	return &prMerger{
		ghToken:               ghToken,
		pollInterval:          30 * time.Second,
		mergeTimeout:          30 * time.Minute,
		currentDateTimeGetter: currentDateTimeGetter,
		runner:                subproc.NewRunner(),
	}
}

// newPRMergerWithRunner creates a PRMerger with an injected runner (for tests).
func newPRMergerWithRunner(
	ghToken string,
	cdt libtime.CurrentDateTimeGetter,
	r subproc.Runner,
) PRMerger {
	return &prMerger{
		ghToken:               ghToken,
		pollInterval:          30 * time.Second,
		mergeTimeout:          30 * time.Minute,
		currentDateTimeGetter: cdt,
		runner:                r,
	}
}

// prMerger implements PRMerger.
type prMerger struct {
	ghToken               string
	pollInterval          time.Duration
	mergeTimeout          time.Duration
	currentDateTimeGetter libtime.CurrentDateTimeGetter
	runner                subproc.Runner
}

// prStatus represents the JSON response from gh pr view.
type prStatus struct {
	MergeStateStatus string `json:"mergeStateStatus"`
}

// decideMergeAction maps a mergeStateStatus value to an action.
func decideMergeAction(mergeStateStatus string) (shouldMerge bool, err error) {
	switch mergeStateStatus {
	case "CLEAN":
		return true, nil
	case "DIRTY":
		return false, stderrors.New("PR has conflicts and cannot be merged")
	default:
		return false, nil
	}
}

// WaitAndMerge polls the PR until it's mergeable, then merges it.
func (p *prMerger) WaitAndMerge(ctx context.Context, prURL string) error {
	deadline := time.Time(p.currentDateTimeGetter.Now()).Add(p.mergeTimeout)
	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx, ctx.Err(), "context cancelled while waiting for PR")
		case <-ticker.C:
			if time.Time(p.currentDateTimeGetter.Now()).After(deadline) {
				return errors.Errorf(ctx, "timeout waiting for PR to become mergeable")
			}

			status, err := p.checkPRStatus(ctx, prURL)
			if err != nil {
				return errors.Wrap(ctx, err, "check PR status")
			}

			shouldMerge, mergeErr := decideMergeAction(status.MergeStateStatus)
			if mergeErr != nil {
				return errors.Wrap(ctx, mergeErr, "PR not mergeable")
			}
			if shouldMerge {
				return p.mergePR(ctx, prURL)
			}
		}
	}
}

// checkPRStatus queries the PR merge state.
func (p *prMerger) checkPRStatus(ctx context.Context, prURL string) (*prStatus, error) {
	if err := ValidatePRURL(ctx, prURL); err != nil {
		return nil, errors.Wrap(ctx, err, "validate PR URL")
	}
	var extraEnv []string
	if p.ghToken != "" {
		extraEnv = []string{"GH_TOKEN=" + p.ghToken}
	}
	output, err := p.runner.RunWithWarnAndTimeoutEnv(
		ctx,
		"gh pr view",
		"",
		extraEnv,
		"gh", "pr", "view", prURL, "--json", "mergeStateStatus",
	)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "execute gh pr view")
	}

	var status prStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, errors.Wrap(ctx, err, "parse pr status json")
	}

	return &status, nil
}

// mergePR merges the pull request and deletes the branch.
func (p *prMerger) mergePR(ctx context.Context, prURL string) error {
	if err := ValidatePRURL(ctx, prURL); err != nil {
		return errors.Wrap(ctx, err, "validate PR URL")
	}
	var extraEnv []string
	if p.ghToken != "" {
		extraEnv = []string{"GH_TOKEN=" + p.ghToken}
	}
	_, err := p.runner.RunWithWarnAndTimeoutEnv(
		ctx,
		"gh pr merge",
		"",
		extraEnv,
		"gh", "pr", "merge", prURL, "--merge", "--delete-branch",
	)
	if err != nil {
		return errors.Wrap(ctx, err, "merge pull request")
	}
	return nil
}
