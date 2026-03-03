// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package report

// Verification records the result of a verification command.
type Verification struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exitCode"`
}

// CompletionReport is the structured output the AI agent must produce at the end of every prompt execution.
type CompletionReport struct {
	Status       string        `json:"status"`                 // "success", "partial", "failed"
	Summary      string        `json:"summary"`                // one-line description of what was done
	Blockers     []string      `json:"blockers"`               // why it's not success (empty on success)
	Verification *Verification `json:"verification,omitempty"` // optional verification command result
}

// ValidateConsistency checks if the reported status is consistent with verification results.
// Returns the corrected status and true if it was overridden.
func (r *CompletionReport) ValidateConsistency() (correctedStatus string, overridden bool) {
	if r.Status == "success" && r.Verification != nil && r.Verification.ExitCode != 0 {
		return "partial", true
	}
	return r.Status, false
}
