// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
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
func NewInboxHandler(inboxDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read inbox directory
		entries, err := os.ReadDir(inboxDir)
		if err != nil {
			log.Printf("dark-factory: failed to read inbox directory: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
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
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("dark-factory: failed to encode inbox response: %v", err)
		}
	}
}
