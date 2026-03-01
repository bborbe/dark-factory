// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/bborbe/dark-factory/pkg/prompt"
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
	promptManager prompt.Manager,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx := r.Context()

		// Check if this is /api/v1/queue/all
		if strings.HasSuffix(r.URL.Path, "/all") {
			handleQueueAll(ctx, w, inboxDir, queueDir, promptManager)
			return
		}

		handleQueueSingle(ctx, w, r, inboxDir, queueDir, promptManager)
	}
}

// handleQueueAll handles the POST /api/v1/queue/action/all endpoint.
func handleQueueAll(
	ctx context.Context,
	w http.ResponseWriter,
	inboxDir string,
	queueDir string,
	promptManager prompt.Manager,
) {
	queuedFiles, err := queueAllFiles(ctx, inboxDir, queueDir, promptManager)
	if err != nil {
		log.Printf("dark-factory: failed to queue all files: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	writeQueueResponse(w, queuedFiles)
}

// handleQueueSingle handles the POST /api/v1/queue/action endpoint.
func handleQueueSingle(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	inboxDir string,
	queueDir string,
	promptManager prompt.Manager,
) {
	var req QueueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.File == "" {
		http.Error(w, "Missing file parameter", http.StatusBadRequest)
		return
	}

	queuedFile, err := queueSingleFile(ctx, inboxDir, queueDir, promptManager, req.File)
	if err != nil {
		handleQueueError(w, err)
		return
	}

	writeQueueResponse(w, []QueuedFile{queuedFile})
}

// handleQueueError handles errors from queueing operations.
func handleQueueError(w http.ResponseWriter, err error) {
	if os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	log.Printf("dark-factory: failed to queue file: %v", err)
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}

// writeQueueResponse writes a QueueResponse to the response writer.
func writeQueueResponse(w http.ResponseWriter, queuedFiles []QueuedFile) {
	response := QueueResponse{Queued: queuedFiles}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("dark-factory: failed to encode queue response: %v", err)
	}
}
