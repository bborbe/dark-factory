// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package report

const (
	MarkerStart = "<!-- DARK-FACTORY-REPORT"
	MarkerEnd   = "DARK-FACTORY-REPORT -->"
)

// Suffix returns the markdown text that dark-factory appends to every prompt.
// It instructs the AI agent to output a structured completion report.
func Suffix() string {
	return `

---

## Completion Report (MANDATORY)

As your VERY LAST output, you MUST produce a completion report in this EXACT format.
The JSON must be on a SINGLE LINE between the markers.

` + MarkerStart + `
{"status":"success","summary":"Replaced splitFrontmatter with adrg/frontmatter library","blockers":[]}
` + MarkerEnd + `

Field values:
- status: "success" = all requirements met, verification passed. "partial" = some work done but blockers remain. "failed" = could not complete the task.
- summary: One sentence describing what was accomplished.
- blockers: Array of strings explaining what prevented success. Empty array [] when status is "success".

This report is MANDATORY. Do not skip it.
`
}
