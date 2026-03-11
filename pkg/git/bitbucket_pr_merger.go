// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bborbe/errors"
)

// bitbucketPRMerger implements PRMerger for Bitbucket Server.
type bitbucketPRMerger struct {
	baseURL      string
	token        string
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
		baseURL:      strings.TrimRight(baseURL, "/"),
		token:        token,
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
	prID, err := extractBitbucketPRID(prURL)
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
	url := fmt.Sprintf(
		"%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d",
		b.baseURL, b.project, b.repo, prID,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "create fetch PR request")
	}
	req.Header.Set("Authorization", "Bearer "+b.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "execute fetch PR request")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf(ctx, "fetch PR returned status %d: %s", resp.StatusCode, body)
	}

	var pr bbPRDetail
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, errors.Wrap(ctx, err, "parse PR response")
	}
	return &pr, nil
}

func (b *bitbucketPRMerger) mergePR(ctx context.Context, prID int, version int) error {
	url := fmt.Sprintf(
		"%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/merge?version=%d",
		b.baseURL, b.project, b.repo, prID, version,
	)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return errors.Wrap(ctx, err, "create merge PR request")
	}
	req.Header.Set("Authorization", "Bearer "+b.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(ctx, err, "execute merge PR request")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf(ctx, "merge PR returned status %d: %s", resp.StatusCode, body)
	}
	return nil
}

// extractBitbucketPRID parses the PR ID from a Bitbucket Server PR URL.
// Supports URLs like: https://bitbucket.example.com/projects/PRJ/repos/repo/pull-requests/42
// Also supports bare numeric IDs passed as the URL.
func extractBitbucketPRID(prURL string) (int, error) {
	// Try to parse the last path segment as an int
	parts := strings.Split(strings.TrimRight(prURL, "/"), "/")
	if len(parts) > 0 {
		var id int
		if _, err := fmt.Sscanf(parts[len(parts)-1], "%d", &id); err == nil {
			return id, nil
		}
	}
	return 0, fmt.Errorf("cannot extract PR ID from URL %q", prURL)
}
