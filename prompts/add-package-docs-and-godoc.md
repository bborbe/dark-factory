---
status: created
created: "2026-03-08T21:12:08Z"
---

<summary>
- All 19 packages gain discoverable documentation visible in `go doc` and pkg.go.dev
- Three undocumented exported methods on the Workflow type get GoDoc comments
- Two report delimiter constants get GoDoc comments
- Five spec lifecycle status constants get individual GoDoc comments
- No logic changes — purely additive documentation
</summary>

<objective>
All exported identifiers and packages in `pkg/` are fully documented, enabling `go doc` browsing and pkg.go.dev rendering without manual source inspection.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read any existing `doc.go` in the project (there are none — use other bborbe projects as reference).
Read `pkg/config/workflow.go` — `Workflow.String()` (line ~28), `Workflow.Validate()` (line ~32), `Workflow.Ptr()` (line ~39) lack doc comments.
Read `pkg/report/suffix.go` — `MarkerStart` (line ~8) and `MarkerEnd` (line ~9) lack doc comments.
Read `pkg/spec/spec.go` — `StatusDraft`, `StatusApproved`, `StatusPrompted`, `StatusVerifying`, `StatusCompleted` constants (lines ~46-52) lack individual doc comments.
Read `/home/node/.claude/docs/go-patterns.md` — GoDoc conventions.
</context>

<requirements>
1. Create `doc.go` in each of these 19 packages with a single package-level comment. Each file must have the copyright header. The comment should be one sentence describing the package purpose:

   - `pkg/cmd/` — "Package cmd implements CLI commands for the dark-factory tool."
   - `pkg/config/` — "Package config handles configuration loading and validation."
   - `pkg/executor/` — "Package executor runs prompt files inside Docker containers."
   - `pkg/factory/` — "Package factory wires all dependencies for the dark-factory application."
   - `pkg/generator/` — "Package generator creates prompt files from spec definitions."
   - `pkg/git/` — "Package git provides version control operations for commits, tags, branches, and PRs."
   - `pkg/lock/` — "Package lock provides file-based mutual exclusion for single-instance enforcement."
   - `pkg/processor/` — "Package processor manages the prompt execution lifecycle."
   - `pkg/project/` — "Package project defines project-level types and naming."
   - `pkg/prompt/` — "Package prompt handles prompt file loading, saving, and queue management."
   - `pkg/report/` — "Package report generates and parses completion reports from prompt execution."
   - `pkg/review/` — "Package review implements automated PR review polling and fix prompt generation."
   - `pkg/runner/` — "Package runner orchestrates the main dark-factory event loop."
   - `pkg/server/` — "Package server provides HTTP handlers for status, queue, and inbox endpoints."
   - `pkg/spec/` — "Package spec handles specification file loading, status management, and listing."
   - `pkg/specwatcher/` — "Package specwatcher watches spec files and triggers prompt generation on changes."
   - `pkg/status/` — "Package status aggregates and formats system status information."
   - `pkg/version/` — "Package version provides build version information."
   - `pkg/watcher/` — "Package watcher monitors the prompts directory for file changes and normalizes filenames."

   Template for each `doc.go`:
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package <name> <description>.
   package <name>
   ```

2. Add GoDoc to `pkg/config/workflow.go`:
   ```go
   // String returns the string representation of the Workflow.
   func (w Workflow) String() string {

   // Validate checks that the Workflow is a known value.
   func (w Workflow) Validate(ctx context.Context) error {

   // Ptr returns a pointer to the Workflow value.
   func (w Workflow) Ptr() *Workflow {
   ```

3. Add GoDoc to `pkg/report/suffix.go`:
   ```go
   // MarkerStart is the opening delimiter for completion report blocks.
   MarkerStart = "..."

   // MarkerEnd is the closing delimiter for completion report blocks.
   MarkerEnd = "..."
   ```

4. Add GoDoc to each spec status constant in `pkg/spec/spec.go`:
   ```go
   // StatusDraft indicates the spec has been written but not yet reviewed.
   StatusDraft Status = "draft"
   // StatusApproved indicates the spec has been reviewed and approved.
   StatusApproved Status = "approved"
   // StatusPrompted indicates prompts have been generated from the spec.
   StatusPrompted Status = "prompted"
   // StatusVerifying indicates all prompts completed, awaiting human verification.
   StatusVerifying Status = "verifying"
   // StatusCompleted indicates human verified all acceptance criteria.
   StatusCompleted Status = "completed"
   ```
</requirements>

<constraints>
- GoDoc comments must start with the item name (e.g., "String returns..." not "Returns...")
- Package comments must start with "Package <name>" per Go convention
- Do NOT change any code — only add comments and doc.go files
- Do NOT create doc.go in `pkg/test-project-001-worktree-remove-fail/` — it is an empty test fixture directory
- Each doc.go must have the copyright header
- Do NOT commit — dark-factory handles git
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Verify doc.go files exist:
```bash
find pkg/ -name "doc.go" | wc -l
# Expected: 19
```

Verify no undocumented exports in changed files:
```bash
grep -n "^func\|^type\|^var\|^const" pkg/config/workflow.go pkg/report/suffix.go | head -20
# All should have comments above them
```
</verification>

<success_criteria>
- 19 doc.go files created with proper package comments
- `Workflow.String/Validate/Ptr` have GoDoc
- `MarkerStart/MarkerEnd` have GoDoc
- Spec status constants have individual GoDoc
- `make precommit` passes
</success_criteria>
