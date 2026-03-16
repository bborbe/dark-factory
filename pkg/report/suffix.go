// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package report

const (
	// MarkerStart is the opening delimiter for completion report blocks.
	MarkerStart = "<!-- DARK-FACTORY-REPORT"
	// MarkerEnd is the closing delimiter for completion report blocks.
	MarkerEnd = "DARK-FACTORY-REPORT -->"
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
{"status":"success","summary":"Replaced splitFrontmatter with adrg/frontmatter library","blockers":[],"verification":{"command":"make precommit","exitCode":0}}
` + MarkerEnd + `

Field values:
- status: "success" = all requirements met, verification passed. "partial" = some work done but blockers remain. "failed" = could not complete the task.
- summary: One sentence describing what was accomplished.
- blockers: Array of strings explaining what prevented success. Empty array [] when status is "success".
- verification: Optional object with "command" and "exitCode" fields. If status is "success", exitCode must be 0.

This report is MANDATORY. Do not skip it.
After writing this report, STOP. Do not output anything else — no summary, no explanation, no "Type /exit".
`
}

// ValidationSuffix returns the markdown text injected when a project-level validation command is configured.
// It instructs the agent to treat the command's exit code as the authoritative success/failure signal,
// overriding any <verification> section in the prompt.
func ValidationSuffix(cmd string) string {
	return "\n\n---\n\n## Project Validation Command (REQUIRED — overrides <verification> section)\n\nRun the following command as the authoritative validation step and use its exit code in the completion report:\n\n```\n" + cmd + "\n```\n\nThis overrides any `<verification>` section in this prompt. Report `\"status\":\"success\"` if and only if this command exits 0.\n"
}

// ValidationPromptSuffix returns the markdown text injected when a project-level validation prompt is configured.
// It instructs the agent to evaluate each criterion against its changes and report unmet criteria as blockers
// with "partial" status. Evaluation runs only after validationCommand passes (if one is configured).
func ValidationPromptSuffix(criteria string) string {
	return "\n\n---\n\n## Project Quality Criteria (AI-Judged)\n\nAfter all code changes are complete and `make precommit` (or the configured validation command) has passed, evaluate each of the following criteria against your changes:\n\n" + criteria + "\n\nFor each criterion:\n- If met: note it as passing.\n- If NOT met: add it to the `blockers` array in the completion report.\n\nIf any criterion is not met, set `\"status\":\"partial\"` in the completion report and list each unmet criterion as a separate entry in `blockers`. If all criteria are met, this section has no effect on the status — `\"success\"` stays `\"success\"`.\n"
}

// ChangelogSuffix returns instructions for the YOLO agent to write a descriptive changelog entry.
// It is appended to the prompt only when the project has a CHANGELOG.md.
func ChangelogSuffix() string {
	return `

---

Update CHANGELOG.md following ` + "`/home/node/.claude/docs/changelog-guide.md`" + `. Create ` + "`## Unreleased`" + ` if missing, extend it if it already exists.
`
}
