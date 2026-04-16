// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
	libtime "github.com/bborbe/time"
)

// QueueRequest represents the request body for POST /api/v1/queue.
type QueueRequest struct {
	File string `json:"file"`
}

// QueuedFile represents a single queued file.
type QueuedFile struct {
	Old string `json:"old"`
	New string `json:"new"`
}

// QueueResponse represents the response for POST /api/v1/queue.
type QueueResponse struct {
	Queued []QueuedFile `json:"queued"`
}

// NewQueueActionHandler creates a handler for POST /api/v1/queue endpoints.
func NewQueueActionHandler(
	inboxDir string,
	queueDir string,
	promptManager PromptManager,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) libhttp.WithError {
	return libhttp.WithErrorFunc(
		func(ctx context.Context, resp http.ResponseWriter, req *http.Request) error {
			if req.Method != http.MethodPost {
				return libhttp.WrapWithStatusCode(
					errors.New(ctx, "method not allowed"),
					http.StatusMethodNotAllowed,
				)
			}

			// Check if this is /api/v1/queue/all
			if strings.HasSuffix(req.URL.Path, "/all") {
				return handleQueueAll(
					ctx,
					resp,
					inboxDir,
					queueDir,
					promptManager,
					currentDateTimeGetter,
				)
			}

			return handleQueueSingle(
				ctx,
				resp,
				req,
				inboxDir,
				queueDir,
				promptManager,
				currentDateTimeGetter,
			)
		},
	)
}

// handleQueueAll handles the POST /api/v1/queue/action/all endpoint.
func handleQueueAll(
	ctx context.Context,
	resp http.ResponseWriter,
	inboxDir string,
	queueDir string,
	promptManager PromptManager,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	queuedFiles, err := queueAllFiles(ctx, inboxDir, queueDir, promptManager, currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "queue all files")
	}

	return writeQueueResponse(resp, queuedFiles)
}

// handleQueueSingle handles the POST /api/v1/queue/action endpoint.
func handleQueueSingle(
	ctx context.Context,
	resp http.ResponseWriter,
	req *http.Request,
	inboxDir string,
	queueDir string,
	promptManager PromptManager,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	// Limit request body size to 1MB
	req.Body = http.MaxBytesReader(resp, req.Body, 1024*1024)

	var queueReq QueueRequest
	if err := json.NewDecoder(req.Body).Decode(&queueReq); err != nil {
		return libhttp.WrapWithStatusCode(
			errors.Wrap(ctx, err, "invalid request body"),
			http.StatusBadRequest,
		)
	}

	if queueReq.File == "" {
		return libhttp.WrapWithStatusCode(
			errors.New(ctx, "missing file parameter"),
			http.StatusBadRequest,
		)
	}

	// Explicit containment check: verify the raw input joined with inboxDir stays within inbox
	checkPath := filepath.Join(inboxDir, queueReq.File)
	cleanInbox := filepath.Clean(inboxDir) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(checkPath)+string(os.PathSeparator), cleanInbox) {
		return libhttp.WrapWithStatusCode(
			errors.New(ctx, "path escapes inbox directory"),
			http.StatusBadRequest,
		)
	}

	// Fix path traversal: sanitize filename to prevent directory traversal
	filename := filepath.Base(queueReq.File)
	if filename == "." || filename == ".." {
		return libhttp.WrapWithStatusCode(
			errors.New(ctx, "invalid filename"),
			http.StatusBadRequest,
		)
	}

	queuedFile, err := queueSingleFile(
		ctx,
		inboxDir,
		queueDir,
		promptManager,
		filename,
		currentDateTimeGetter,
	)
	if err != nil {
		return handleQueueError(err)
	}

	return writeQueueResponse(resp, []QueuedFile{queuedFile})
}

// handleQueueError handles errors from queueing operations.
func handleQueueError(err error) error {
	if os.IsNotExist(err) {
		return libhttp.WrapWithStatusCode(err, http.StatusNotFound)
	}
	return err
}

// writeQueueResponse writes a QueueResponse to the response writer.
func writeQueueResponse(resp http.ResponseWriter, queuedFiles []QueuedFile) error {
	response := QueueResponse{Queued: queuedFiles}
	resp.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(resp).Encode(response)
}
