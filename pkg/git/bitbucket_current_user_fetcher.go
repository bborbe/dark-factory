// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

//counterfeiter:generate -o ../../mocks/bitbucket-current-user-fetcher.go --fake-name BitbucketCurrentUserFetcher . BitbucketCurrentUserFetcher

// BitbucketCurrentUserFetcher fetches the current authenticated Bitbucket user.
type BitbucketCurrentUserFetcher interface {
	FetchCurrentUser(ctx context.Context) string
}

// NewBitbucketCurrentUserFetcher creates a BitbucketCurrentUserFetcher for the given base URL and token.
func NewBitbucketCurrentUserFetcher(baseURL, token string) BitbucketCurrentUserFetcher {
	return &bitbucketCurrentUserFetcher{baseURL: baseURL, token: token}
}

type bitbucketCurrentUserFetcher struct {
	baseURL string
	token   string
}

// FetchCurrentUser fetches the current Bitbucket Server username via the whoami endpoint.
// Returns empty string on error (graceful degradation — reviewer exclusion will not apply).
func (f *bitbucketCurrentUserFetcher) FetchCurrentUser(ctx context.Context) string {
	// #nosec G107 -- URL is constructed from config-provided baseURL, not user input
	req, err := http.NewRequestWithContext(
		ctx, "GET",
		strings.TrimRight(f.baseURL, "/")+"/plugins/servlet/applinks/whoami",
		nil,
	)
	if err != nil {
		slog.Warn("bitbucket: failed to create whoami request", "error", err)
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+f.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("bitbucket: whoami request failed", "error", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Warn("bitbucket: whoami returned non-200", "status", resp.StatusCode)
		return ""
	}
	body, _ := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(body))
}
