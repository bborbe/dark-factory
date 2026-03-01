// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"

	"github.com/bborbe/dark-factory/pkg/status"
)

const maxLimit = 1000

// NewCompletedHandler creates a handler for the /api/v1/completed endpoint.
func NewCompletedHandler(checker status.Checker) libhttp.WithError {
	return libhttp.WithErrorFunc(
		func(ctx context.Context, resp http.ResponseWriter, req *http.Request) error {
			if req.Method != http.MethodGet {
				return libhttp.WrapWithStatusCode(
					errors.New(ctx, "method not allowed"),
					http.StatusMethodNotAllowed,
				)
			}

			// Parse limit parameter
			limit := 10 // default
			if limitStr := req.URL.Query().Get("limit"); limitStr != "" {
				parsedLimit, err := strconv.Atoi(limitStr)
				if err == nil && parsedLimit > 0 {
					limit = parsedLimit
					// Cap limit to maxLimit
					if limit > maxLimit {
						limit = maxLimit
					}
				}
			}

			completed, err := checker.GetCompletedPrompts(ctx, limit)
			if err != nil {
				return err
			}

			resp.Header().Set("Content-Type", "application/json")
			return json.NewEncoder(resp).Encode(completed)
		},
	)
}
