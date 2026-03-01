// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
)

// InboxFile represents a file in the inbox directory.
type InboxFile struct {
	Name string `json:"name"`
}

// InboxListResponse represents the response for GET /api/v1/inbox.
type InboxListResponse struct {
	Files []InboxFile `json:"files"`
}

// NewInboxHandler creates a handler for the /api/v1/inbox endpoint.
func NewInboxHandler(inboxDir string) libhttp.WithError {
	return libhttp.WithErrorFunc(
		func(ctx context.Context, resp http.ResponseWriter, req *http.Request) error {
			if req.Method != http.MethodGet {
				return libhttp.WrapWithStatusCode(
					errors.New(ctx, "method not allowed"),
					http.StatusMethodNotAllowed,
				)
			}

			// Read inbox directory
			entries, err := os.ReadDir(inboxDir)
			if err != nil {
				return err
			}

			// Collect .md files
			var files []InboxFile
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
					continue
				}
				files = append(files, InboxFile{Name: entry.Name()})
			}

			response := InboxListResponse{Files: files}
			resp.Header().Set("Content-Type", "application/json")
			return json.NewEncoder(resp).Encode(response)
		},
	)
}
