// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/bborbe/dark-factory/pkg/status"
)

// NewQueueHandler creates a handler for the /api/v1/queue endpoint.
func NewQueueHandler(checker status.Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx := r.Context()
		queued, err := checker.GetQueuedPrompts(ctx)
		if err != nil {
			log.Printf("dark-factory: failed to get queued prompts: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(queued); err != nil {
			log.Printf("dark-factory: failed to encode queued prompts: %v", err)
		}
	}
}
