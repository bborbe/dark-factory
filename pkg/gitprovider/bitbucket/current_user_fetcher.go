// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bitbucket

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

//counterfeiter:generate -o ../../mocks/bitbucket-current-user-fetcher.go --fake-name CurrentUserFetcher . CurrentUserFetcher

// CurrentUserFetcher fetches the current authenticated Bitbucket user.
type CurrentUserFetcher interface {
	FetchCurrentUser(ctx context.Context) string
}

// NewCurrentUserFetcher creates a CurrentUserFetcher for the given base URL and token.
func NewCurrentUserFetcher(baseURL, token string) CurrentUserFetcher {
	return &currentUserFetcher{baseURL: baseURL, token: token}
}

type currentUserFetcher struct {
	baseURL string
	token   string
}

// FetchCurrentUser fetches the current Bitbucket Server username via the whoami endpoint.
// Returns empty string on error (graceful degradation — reviewer exclusion will not apply).
func (f *currentUserFetcher) FetchCurrentUser(ctx context.Context) string {
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
