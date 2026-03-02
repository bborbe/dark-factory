// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package report

// CompletionReport is the structured output the AI agent must produce at the end of every prompt execution.
type CompletionReport struct {
	Status   string   `json:"status"`   // "success", "partial", "failed"
	Summary  string   `json:"summary"`  // one-line description of what was done
	Blockers []string `json:"blockers"` // why it's not success (empty on success)
}
