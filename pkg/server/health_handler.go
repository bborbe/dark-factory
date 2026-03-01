// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"net/http"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
)

// NewHealthHandler creates a handler for the /health endpoint.
func NewHealthHandler() libhttp.WithError {
	return libhttp.WithErrorFunc(
		func(ctx context.Context, resp http.ResponseWriter, req *http.Request) error {
			if req.Method != http.MethodGet {
				return libhttp.WrapWithStatusCode(
					errors.New(ctx, "method not allowed"),
					http.StatusMethodNotAllowed,
				)
			}

			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(http.StatusOK)
			_, _ = resp.Write([]byte(`{"status":"ok"}`))
			return nil
		},
	)
}
