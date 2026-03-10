// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"time"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
)

//counterfeiter:generate -o ../../mocks/pr_merger.go --fake-name PRMerger . PRMerger

// PRMerger watches a PR until mergeable and merges it.
type PRMerger interface {
	WaitAndMerge(ctx context.Context, prURL string) error
}

// prMerger implements PRMerger.
type prMerger struct {
	ghToken               string
	pollInterval          time.Duration
	mergeTimeout          time.Duration
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewPRMerger creates a new PRMerger.
func NewPRMerger(ghToken string, currentDateTimeGetter libtime.CurrentDateTimeGetter) PRMerger {
	return &prMerger{
		ghToken:               ghToken,
		pollInterval:          30 * time.Second,
		mergeTimeout:          30 * time.Minute,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// prStatus represents the JSON response from gh pr view.
type prStatus struct {
	MergeStateStatus string `json:"mergeStateStatus"`
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

			switch status.MergeStateStatus {
			case "MERGEABLE":
				return p.mergePR(ctx, prURL)
			case "CONFLICTING":
				return errors.Errorf(ctx, "PR has conflicts and cannot be merged")
			default:
				// Continue polling for other states (BLOCKED, UNKNOWN, etc.)
				continue
			}
		}
	}
}

// checkPRStatus queries the PR merge state.
func (p *prMerger) checkPRStatus(ctx context.Context, prURL string) (*prStatus, error) {
	// #nosec G204 -- prURL is from our own PR creation, not user input
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", prURL, "--json", "mergeStateStatus")
	if p.ghToken != "" {
		cmd.Env = append(os.Environ(), "GH_TOKEN="+p.ghToken)
	}

	output, err := cmd.Output()
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
	// #nosec G204 -- prURL is from our own PR creation, not user input
	cmd := exec.CommandContext(ctx, "gh", "pr", "merge", prURL, "--merge", "--delete-branch")
	if p.ghToken != "" {
		cmd.Env = append(os.Environ(), "GH_TOKEN="+p.ghToken)
	}

	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "merge pull request")
	}

	return nil
}
