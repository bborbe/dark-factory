---
status: completed
summary: Refactored prompt status type from Status to PromptStatus following go-enum-type-pattern, added DraftPromptStatus and ApprovedPromptStatus (replacing queued), renamed MarkQueued to MarkApproved, updated all callers and test files across pkg/
container: dark-factory-130-refactor-prompt-status-enum
dark-factory-version: v0.24.0
created: "2026-03-07T19:34:00Z"
queued: "2026-03-07T19:50:15Z"
started: "2026-03-07T19:55:06Z"
completed: "2026-03-07T20:11:14Z"
---
<summary>
- Renames prompt status type from `Status` to `PromptStatus` following go-enum-type-pattern
- Adds `draft` as initial prompt status, renames `queued` to `approved`
- Constants follow pattern: `DraftPromptStatus`, `ApprovedPromptStatus`, etc.
- Adds `AvailablePromptStatuses`, `PromptStatuses` collection, `Contains`, `String` methods
- Prompts and specs now share the same naming: draft → approved → ...
</summary>

<objective>
Refactor prompt statuses to follow the go-enum-type-pattern. Rename `Status` to `PromptStatus`, add `draft` as initial status, rename `queued` to `approved`. After this, prompts and specs share the same lifecycle naming convention.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `~/Documents/workspaces/coding-guidelines/go-enum-type-pattern.md` — the canonical enum pattern to follow.
Read `pkg/prompt/prompt.go` — current `Status` type and constants (`StatusQueued`, `StatusExecuting`, etc.).
Read `pkg/cmd/approve.go` — uses `MarkQueued()` which will be renamed.
Read `pkg/server/queue_helpers.go` — calls `MarkQueued()`, needs update.
</context>

<requirements>
1. In `pkg/prompt/prompt.go`, refactor the status type to follow `go-enum-type-pattern.md`:
   - Rename `type Status string` to `type PromptStatus string`.
   - Rename constants following `<Value><Type>` pattern:
     - Add `DraftPromptStatus PromptStatus = "draft"` (new).
     - `StatusQueued` → `ApprovedPromptStatus PromptStatus = "approved"`.
     - `StatusExecuting` → `ExecutingPromptStatus PromptStatus = "executing"`.
     - `StatusCompleted` → `CompletedPromptStatus PromptStatus = "completed"`.
     - `StatusFailed` → `FailedPromptStatus PromptStatus = "failed"`.
     - `StatusInReview` → `InReviewPromptStatus PromptStatus = "in_review"`.
   - Add `var AvailablePromptStatuses = PromptStatuses{DraftPromptStatus, ApprovedPromptStatus, ExecutingPromptStatus, CompletedPromptStatus, FailedPromptStatus, InReviewPromptStatus}`.
   - Add `type PromptStatuses []PromptStatus`.
   - Add `func (p PromptStatuses) Contains(status PromptStatus) bool` using `github.com/bborbe/collection`.
   - Add `func (p PromptStatus) String() string { return string(p) }`.
   - Update `Validate` to check `AvailablePromptStatuses.Contains()`.
   - Rename `MarkQueued()` → `MarkApproved()`.
   - Update `ResetExecuting` and `ResetFailed` to reset to `ApprovedPromptStatus`.
   - Update all comments referencing "queued" status to say "approved".

2. Update all callers and references across the codebase. Grep `pkg/` for `StatusQueued`, `StatusExecuting`, `StatusCompleted`, `StatusFailed`, `StatusInReview`, `MarkQueued`, and the old `Status` type to find every occurrence.

3. Update all test files: replace old constant names and `"queued"` strings with `"approved"`.

4. In `pkg/server/queue_helpers.go`: update `MarkQueued()` → `MarkApproved()` and comments.

5. Remove any imports that become unused after these changes.

6. **Data migration**: Update any existing prompt files in `prompts/in-progress/` that have `status: queued` in their frontmatter to `status: approved`. Use grep to find them, then update each file.
</requirements>

<constraints>
- **Precondition**: The `remove-prompt-queue-command` prompt must be completed first — it removes `queue.go` which references the old `Status` type.
- Do NOT rename the `Queued` timestamp field in `Frontmatter` — it records when the prompt was queued, which is historical
- Do NOT touch spec status types — that will be a separate prompt
- Do NOT modify command routing in `main.go` — only update status references within commands
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
