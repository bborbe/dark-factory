// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/bborbe/errors"
)

// bitbucketClient is a minimal HTTP client for Bitbucket Server REST API.
type bitbucketClient struct {
	baseURL string
	token   string // never logged
}

// newBitbucketClient creates a bitbucketClient.
func newBitbucketClient(baseURL, token string) *bitbucketClient {
	return &bitbucketClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
	}
}

// do executes an authenticated HTTP request and decodes the JSON response into out (may be nil).
// Returns an error for non-2xx status codes, with the token redacted from any error messages.
func (c *bitbucketClient) do(
	ctx context.Context,
	method, path string,
	body interface{},
	out interface{},
) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return errors.Wrap(ctx, err, "marshal request body")
		}
		bodyReader = bytes.NewReader(b)
	}

	url := c.baseURL + path
	// #nosec G107 -- URL is constructed from config-provided baseURL, not user input
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return errors.Wrap(ctx, err, "create http request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(ctx, err, "execute http request")
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		// redact token from logged error — do NOT include c.token in the message
		return errors.Errorf(
			ctx,
			"bitbucket API %s %s returned %d: %s",
			method,
			path,
			resp.StatusCode,
			redactToken(string(respBody), c.token),
		)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return errors.Wrap(ctx, err, "decode response body")
		}
	}
	return nil
}

// redactToken replaces any occurrence of the token in s with "[REDACTED]".
// This ensures tokens never appear in error messages or logs.
func redactToken(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "[REDACTED]")
}
