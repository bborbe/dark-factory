// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bborbe/errors"
)

// bitbucketPRCreator implements PRCreator for Bitbucket Server.
type bitbucketPRCreator struct {
	baseURL       string
	token         string
	project       string
	repo          string
	defaultBranch string
	reviewers     []string
}

// NewBitbucketPRCreator creates a PRCreator backed by the Bitbucket Server REST API.
func NewBitbucketPRCreator(
	baseURL string,
	token string,
	project string,
	repo string,
	defaultBranch string,
	reviewers []string,
) PRCreator {
	return &bitbucketPRCreator{
		baseURL:       strings.TrimRight(baseURL, "/"),
		token:         token,
		project:       project,
		repo:          repo,
		defaultBranch: defaultBranch,
		reviewers:     reviewers,
	}
}

type bbPRRequest struct {
	Title       string       `json:"title"`
	Description string       `json:"description"`
	FromRef     bbRef        `json:"fromRef"`
	ToRef       bbRef        `json:"toRef"`
	Reviewers   []bbReviewer `json:"reviewers"`
}

type bbRef struct {
	ID         string       `json:"id"`
	Repository bbRepository `json:"repository"`
}

type bbRepository struct {
	Slug    string    `json:"slug"`
	Project bbProject `json:"project"`
}

type bbProject struct {
	Key string `json:"key"`
}

type bbReviewer struct {
	User bbUser `json:"user"`
}

type bbUser struct {
	Name string `json:"name"`
}

type bbPRResponse struct {
	Links struct {
		Self []struct {
			Href string `json:"href"`
		} `json:"self"`
	} `json:"links"`
	ID int `json:"id"`
}

// Create creates a Bitbucket Server pull request and returns its URL.
func (b *bitbucketPRCreator) Create(
	ctx context.Context,
	title string,
	body string,
) (string, error) {
	currentBranch, err := currentGitBranch(ctx)
	if err != nil {
		return "", errors.Wrap(ctx, err, "get current branch")
	}

	targetBranch := b.defaultBranch
	if targetBranch == "" {
		targetBranch = "master"
	}

	var reviewers []bbReviewer
	for _, r := range b.reviewers {
		reviewers = append(reviewers, bbReviewer{User: bbUser{Name: r}})
	}

	repo := bbRepository{
		Slug:    b.repo,
		Project: bbProject{Key: b.project},
	}

	reqBody := bbPRRequest{
		Title:       title,
		Description: body,
		FromRef: bbRef{
			ID:         "refs/heads/" + currentBranch,
			Repository: repo,
		},
		ToRef: bbRef{
			ID:         "refs/heads/" + targetBranch,
			Repository: repo,
		},
		Reviewers: reviewers,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", errors.Wrap(ctx, err, "marshal PR request")
	}

	url := fmt.Sprintf(
		"%s/rest/api/1.0/projects/%s/repos/%s/pull-requests",
		b.baseURL, b.project, b.repo,
	)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return "", errors.Wrap(ctx, err, "create PR request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrap(ctx, err, "execute create PR request")
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return "", errors.Errorf(ctx, "create PR returned status %d: %s", resp.StatusCode, respBody)
	}

	var prResp bbPRResponse
	if err := json.Unmarshal(respBody, &prResp); err != nil {
		return "", errors.Wrap(ctx, err, "parse create PR response")
	}

	if len(prResp.Links.Self) > 0 {
		return prResp.Links.Self[0].Href, nil
	}
	return fmt.Sprintf(
		"%s/projects/%s/repos/%s/pull-requests/%d",
		b.baseURL, b.project, b.repo, prResp.ID,
	), nil
}

// FindOpenPR returns the URL of an open PR for the given branch, or "" if none exists.
func (b *bitbucketPRCreator) FindOpenPR(ctx context.Context, branch string) (string, error) {
	url := fmt.Sprintf(
		"%s/rest/api/1.0/projects/%s/repos/%s/pull-requests?state=OPEN&at=refs/heads/%s",
		b.baseURL, b.project, b.repo, branch,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", errors.Wrap(ctx, err, "create find PR request")
	}
	req.Header.Set("Authorization", "Bearer "+b.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrap(ctx, err, "execute find PR request")
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf(ctx, "find PR returned status %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		Values []bbPRResponse `json:"values"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", errors.Wrap(ctx, err, "parse find PR response")
	}

	if len(result.Values) == 0 {
		return "", nil
	}

	pr := result.Values[0]
	if len(pr.Links.Self) > 0 {
		return pr.Links.Self[0].Href, nil
	}
	return fmt.Sprintf(
		"%s/projects/%s/repos/%s/pull-requests/%d",
		b.baseURL, b.project, b.repo, pr.ID,
	), nil
}

// currentGitBranch returns the current git branch name.
func currentGitBranch(ctx context.Context) (string, error) {
	b := NewBrancher()
	return b.CurrentBranch(ctx)
}
