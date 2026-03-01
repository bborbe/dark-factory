// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"

	"github.com/bborbe/dark-factory/pkg/status"
)

// NewStatusHandler creates a handler for the /api/v1/status endpoint.
func NewStatusHandler(checker status.Checker) libhttp.WithError {
	return libhttp.WithErrorFunc(
		func(ctx context.Context, resp http.ResponseWriter, req *http.Request) error {
			if req.Method != http.MethodGet {
				return libhttp.WrapWithStatusCode(
					errors.New(ctx, "method not allowed"),
					http.StatusMethodNotAllowed,
				)
			}

			st, err := checker.GetStatus(ctx)
			if err != nil {
				return err
			}

			resp.Header().Set("Content-Type", "application/json")
			return json.NewEncoder(resp).Encode(st)
		},
	)
}
