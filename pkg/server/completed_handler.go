// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/bborbe/dark-factory/pkg/status"
)

// NewCompletedHandler creates a handler for the /api/v1/completed endpoint.
func NewCompletedHandler(checker status.Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse limit parameter
		limit := 10 // default
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			parsedLimit, err := strconv.Atoi(limitStr)
			if err == nil && parsedLimit > 0 {
				limit = parsedLimit
			}
		}

		ctx := r.Context()
		completed, err := checker.GetCompletedPrompts(ctx, limit)
		if err != nil {
			log.Printf("dark-factory: failed to get completed prompts: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(completed); err != nil {
			log.Printf("dark-factory: failed to encode completed prompts: %v", err)
		}
	}
}
