---
status: completed
summary: Added `idea` status to both prompt and spec lifecycles with doc comments on all status constants, updated Summary struct and lister, updated format strings in spec_status.go and combined_status.go, added test for IdeaPromptStatus, and updated prompt-writing.md lifecycle table.
container: dark-factory-207-add-idea-status
dark-factory-version: v0.59.5-dirty
created: "2026-03-21T10:58:38Z"
queued: "2026-03-21T10:58:38Z"
started: "2026-03-21T10:58:46Z"
completed: "2026-03-21T11:05:35Z"
---

<summary>
- Both specs and prompts gain an "idea" status representing incomplete concepts needing refinement
- Every status constant in both types gets a doc comment explaining its meaning
- The idea status sits before draft in the lifecycle — idea needs refinement, draft is ready for review
- Status summary output includes idea counts alongside draft, approved, etc.
- The daemon and approve commands continue working unchanged — idea files are valid but ignored by the daemon
- Documentation is updated to reflect the complete aligned lifecycle
</summary>

<objective>
Add `idea` as a valid status for both prompts and specs, and ensure every status constant has a clear doc comment explaining what it means. The `idea` status represents incomplete work needing refinement, while `draft` means complete and ready for approval review.
</objective>

<context>
Read CLAUDE.md for project conventions.

Key files:
- `pkg/prompt/prompt.go` — find `PromptStatus` type, its constants, and `AvailablePromptStatuses` slice
- `pkg/spec/spec.go` — find spec `Status` type and its constants
- `pkg/spec/lister.go` — find `Summary` struct and `lister.Summary()` method (status counting logic)
- `pkg/cmd/spec_status.go` — find the `Specs: %d total (...)` format string (includes `%d verifying`)
- `pkg/cmd/combined_status.go` — find the `Specs: %d total (...)` format string (omits verifying)
- `docs/spec-writing.md` — spec status lifecycle table (already has `idea` row)
- `docs/prompt-writing.md` — prompt status lifecycle table (no `idea` row yet)
</context>

<requirements>
### 1. Add `StatusIdea` to spec status constants in `pkg/spec/spec.go`

Add `StatusIdea` before `StatusDraft` and update all doc comments:
```go
const (
    // StatusIdea indicates a rough concept that needs refinement before it can be reviewed.
    StatusIdea Status = "idea"
    // StatusDraft indicates the spec is complete and ready for human review and approval.
    StatusDraft Status = "draft"
    // StatusApproved indicates the spec has been reviewed and approved for prompt generation.
    StatusApproved Status = "approved"
    // StatusPrompted indicates prompts have been generated from the spec.
    StatusPrompted Status = "prompted"
    // StatusVerifying indicates all linked prompts completed, awaiting human verification of acceptance criteria.
    StatusVerifying Status = "verifying"
    // StatusCompleted indicates human verified all acceptance criteria are met.
    StatusCompleted Status = "completed"
)
```

### 2. Add `IdeaPromptStatus` to prompt status constants in `pkg/prompt/prompt.go`

Add `IdeaPromptStatus` before `DraftPromptStatus` and add doc comments to all constants:
```go
const (
    // IdeaPromptStatus indicates a rough concept that needs refinement before it can be reviewed.
    IdeaPromptStatus PromptStatus = "idea"
    // DraftPromptStatus indicates the prompt is complete and ready for human review and approval.
    DraftPromptStatus PromptStatus = "draft"
    // ApprovedPromptStatus indicates the prompt has been approved and queued for execution.
    ApprovedPromptStatus PromptStatus = "approved"
    // ExecutingPromptStatus indicates the prompt is currently being executed in a YOLO container.
    ExecutingPromptStatus PromptStatus = "executing"
    // CompletedPromptStatus indicates the prompt has been executed successfully.
    CompletedPromptStatus PromptStatus = "completed"
    // FailedPromptStatus indicates the prompt execution failed and needs fix or retry.
    FailedPromptStatus PromptStatus = "failed"
    // InReviewPromptStatus indicates the prompt's PR is under review.
    InReviewPromptStatus PromptStatus = "in_review"
    // PendingVerificationPromptStatus indicates the prompt is awaiting verification after review.
    PendingVerificationPromptStatus PromptStatus = "pending_verification"
)
```

### 3. Add `IdeaPromptStatus` to `AvailablePromptStatuses` slice

Insert at position 0 (before `DraftPromptStatus`) in the `AvailablePromptStatuses` variable in `pkg/prompt/prompt.go`.

### 4. Add `Idea` field to `Summary` struct in `pkg/spec/lister.go`

Find the `Summary` struct and add `Idea int` as the first field (before `Draft`):
```go
type Summary struct {
    Idea                   int
    Draft                  int
    // ... rest unchanged
}
```

### 5. Add `StatusIdea` case to `lister.Summary()` method in `pkg/spec/lister.go`

Find the `switch Status(sf.Frontmatter.Status)` block in the `Summary()` method and add:
```go
case StatusIdea:
    s.Idea++
```
before the `case StatusDraft:` case.

### 6. Update format string in `pkg/cmd/spec_status.go`

Find the `Specs: %d total (%d draft, %d approved, %d prompted, %d verifying, %d completed)` format string.

Change to:
```
Specs: %d total (%d idea, %d draft, %d approved, %d prompted, %d verifying, %d completed)
```

Add `summary.Idea` as the second argument (after `summary.Total`, before `summary.Draft`).

### 7. Update format string in `pkg/cmd/combined_status.go`

Find the `Specs: %d total (%d draft, %d approved, %d prompted, %d completed)` format string (note: this one omits `verifying`).

Change to:
```
Specs: %d total (%d idea, %d draft, %d approved, %d prompted, %d completed)
```

Add `summary.Idea` as the second argument (after `summary.Total`, before `summary.Draft`).

### 8. Update prompt status lifecycle table in `docs/prompt-writing.md`

Find the "Prompt Status Lifecycle" table. Add an `idea` row before `created`:

| `idea` | Rough concept, needs refinement | Human creates file |

Keep the existing `created` row description unchanged ("In inbox, not yet queued").

### 9. Verify `docs/spec-writing.md` already has `idea` row

The spec status lifecycle table already includes `idea`. Verify it is present and the `draft` description says "All sections filled". No changes needed if already correct.

### 10. Add test for idea prompt status validation

In `pkg/prompt/prompt_test.go`, find the `Describe("Status.Validate", ...)` block. Add a test case that verifies `IdeaPromptStatus.Validate(ctx)` returns nil (valid status). Follow the existing test pattern in that block.

### 11. Run `make precommit` — must pass.
</requirements>

<constraints>
- Do NOT change approve command behavior — it accepts any inbox status (idea, draft, or created)
- Do NOT change daemon behavior — it only processes approved/queued files
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
```
make precommit
```

```
grep -r 'idea' pkg/prompt/prompt.go pkg/spec/spec.go pkg/spec/lister.go
# expected: IdeaPromptStatus, StatusIdea, and Idea field
```
</verification>
