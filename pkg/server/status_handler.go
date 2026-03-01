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

// NewStatusHandler creates a handler for the /api/v1/status endpoint.
func NewStatusHandler(checker status.Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx := r.Context()
		st, err := checker.GetStatus(ctx)
		if err != nil {
			log.Printf("dark-factory: failed to get status: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(st); err != nil {
			log.Printf("dark-factory: failed to encode status: %v", err)
		}
	}
}
