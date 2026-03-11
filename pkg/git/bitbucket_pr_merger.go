// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/bborbe/errors"
)

// bitbucketPRMerger implements PRMerger for Bitbucket Server.
type bitbucketPRMerger struct {
	client       *bitbucketClient
	project      string
	repo         string
	pollInterval time.Duration
	mergeTimeout time.Duration
}

// NewBitbucketPRMerger creates a PRMerger backed by the Bitbucket Server REST API.
func NewBitbucketPRMerger(
	baseURL string,
	token string,
	project string,
	repo string,
) PRMerger {
	return &bitbucketPRMerger{
		client:       newBitbucketClient(baseURL, token),
		project:      project,
		repo:         repo,
		pollInterval: 30 * time.Second,
		mergeTimeout: 30 * time.Minute,
	}
}

type bbPRDetail struct {
	ID      int    `json:"id"`
	State   string `json:"state"`
	Version int    `json:"version"`
}

// WaitAndMerge polls until the PR is open then merges it.
func (b *bitbucketPRMerger) WaitAndMerge(ctx context.Context, prURL string) error {
	prID, err := parseBitbucketPRID(prURL)
	if err != nil {
		return errors.Wrap(ctx, err, "extract PR ID from URL")
	}

	deadline := time.Now().Add(b.mergeTimeout)
	ticker := time.NewTicker(b.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx, ctx.Err(), "context cancelled while waiting for PR")
		case <-ticker.C:
			if time.Now().After(deadline) {
				return errors.Errorf(ctx, "timeout waiting for PR to become mergeable")
			}

			pr, err := b.fetchPR(ctx, prID)
			if err != nil {
				return errors.Wrap(ctx, err, "fetch PR status")
			}

			if pr.State == "MERGED" {
				return nil
			}
			if pr.State == "DECLINED" {
				return errors.Errorf(ctx, "PR was declined")
			}

			if err := b.mergePR(ctx, prID, pr.Version); err != nil {
				return errors.Wrap(ctx, err, "merge PR")
			}
			return nil
		}
	}
}

func (b *bitbucketPRMerger) fetchPR(ctx context.Context, prID int) (*bbPRDetail, error) {
	var pr bbPRDetail
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d",
		b.project, b.repo, prID)
	if err := b.client.do(ctx, "GET", path, nil, &pr); err != nil {
		return nil, errors.Wrap(ctx, err, "fetch PR detail")
	}
	return &pr, nil
}

func (b *bitbucketPRMerger) mergePR(ctx context.Context, prID int, version int) error {
	path := fmt.Sprintf(
		"/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/merge?version=%d",
		b.project, b.repo, prID, version,
	)
	if err := b.client.do(ctx, "POST", path, nil, nil); err != nil {
		return errors.Wrap(ctx, err, "merge pull request")
	}
	slog.Info("merged Bitbucket PR", "id", prID)
	return nil
}

// parseBitbucketPRID extracts the numeric PR ID from a Bitbucket PR URL.
// Expected format: .../pull-requests/{id} or .../pull-requests/{id}/overview
func parseBitbucketPRID(prURL string) (int, error) {
	parts := strings.Split(strings.TrimRight(prURL, "/"), "/")
	for i, part := range parts {
		if part == "pull-requests" && i+1 < len(parts) {
			idStr := parts[i+1]
			id, err := strconv.Atoi(idStr)
			if err != nil {
				return 0, fmt.Errorf("invalid PR ID %q in URL %q", idStr, prURL)
			}
			return id, nil
		}
	}
	return 0, fmt.Errorf("could not extract PR ID from URL %q", prURL)
}
