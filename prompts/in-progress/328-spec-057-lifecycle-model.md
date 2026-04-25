---
status: committing
spec: [057-lifecycle-state-machine]
summary: Added SpecStatuses/AvailableSpecStatuses/String/Validate/CanTransitionTo/IsTerminal/IsPreExecution/IsActive to spec.Status and CanTransitionTo/IsTerminal/IsPreExecution/IsActive to prompt.PromptStatus, with Ginkgo tests in both packages; Load() stays permissive; make precommit exited 0.
container: dark-factory-328-spec-057-lifecycle-model
dark-factory-version: v0.132.0
created: "2026-04-25T09:27:35Z"
queued: "2026-04-25T09:35:25Z"
started: "2026-04-25T09:58:56Z"
---

<summary>
- `spec.Status` gains a full lifecycle method surface: `AvailableSpecStatuses` collection, `SpecStatuses.Contains()`, `String()`, `Validate(ctx)`, `CanTransitionTo(target) error`, and three predicates `IsTerminal()`, `IsActive()`, `IsPreExecution()`
- `prompt.PromptStatus` gains the methods it was missing to match: `CanTransitionTo(target) error`, `IsTerminal()`, `IsActive()`, `IsPreExecution()`
- Both transition tables (spec and prompt) are frozen as single declarations; adding one edge to the table is the only change needed to allow a new transition
- `Load()` is permissive — it accepts any status string (including legacy values like `queued`) without validating; strict checking happens at the transition boundary via `CanTransitionTo()`
- All predicates and transition checks are covered by new Ginkgo tests in `pkg/spec/` and `pkg/prompt/`
- No change to CLI behavior, frontmatter wire format, or daemon filtering logic
- All existing tests pass unchanged
</summary>

<objective>
Add the shared lifecycle model to `pkg/spec/spec.go` and `pkg/prompt/prompt.go` so that both entities expose the same method surface: enum collection, Validate, String, CanTransitionTo, and the three predicates. This is the model layer only — command-layer usage of these methods is in prompt 2.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-enum-type-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-validation-framework-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Key files to read before editing:
- `pkg/prompt/prompt.go` — authoritative reference shape: `PromptStatus`, `AvailablePromptStatuses`, `PromptStatuses`, `String()`, `Validate()` are already present; need to add `CanTransitionTo()` and three predicates
- `pkg/spec/spec.go` — current state: `Status` type and constants exist but no `AvailableSpecStatuses`, no `String()`, no `Validate()`, no `CanTransitionTo()`, no predicates; `Frontmatter.Status` is plain `string`
- `pkg/prompt/prompt_test.go` — existing test patterns (Ginkgo v2, external `package prompt_test`)
- `pkg/spec/spec_test.go` — existing test patterns
- `pkg/spec/spec_suite_test.go` — suite file
- `pkg/prompt/prompt_suite_test.go` — suite file
</context>

<requirements>

## 1. Add lifecycle model to `pkg/spec/spec.go`

### 1a. `SpecStatuses` type and `AvailableSpecStatuses`

Add after the existing `Status` constants block:

```go
// SpecStatuses is a slice of Status values.
//
//nolint:revive // SpecStatuses is the intended name per go-enum-type-pattern
type SpecStatuses []Status

// Contains returns true if the given status is in the collection.
func (s SpecStatuses) Contains(status Status) bool {
    return collection.Contains(s, status)
}

// AvailableSpecStatuses is the collection of all valid spec Status values.
var AvailableSpecStatuses = SpecStatuses{
    StatusIdea,
    StatusDraft,
    StatusApproved,
    StatusGenerating,
    StatusPrompted,
    StatusVerifying,
    StatusCompleted,
}
```

Add `"github.com/bborbe/collection"` to imports.

### 1b. `String()` and `Validate()` on `spec.Status`

```go
// String returns the string representation of the Status.
func (s Status) String() string { return string(s) }

// Validate validates the Status value.
func (s Status) Validate(ctx context.Context) error {
    if !AvailableSpecStatuses.Contains(s) {
        return errors.Wrapf(ctx, validation.Error, "status(%s) is invalid", s)
    }
    return nil
}
```

Add `"github.com/bborbe/validation"` to imports.

### 1c. Spec transition table and `CanTransitionTo()`

The frozen valid-edges table for spec (from the spec):
```
idea       → draft
draft      → approved
approved   → generating | draft        ← `approved → draft` is the unapprove edge
generating → prompted
prompted   → verifying
verifying  → completed
```

```go
// specTransitions defines the valid state transitions for spec lifecycle.
// This is the single source of truth — add one row here to enable a new transition.
var specTransitions = map[Status][]Status{
    StatusIdea:       {StatusDraft},
    StatusDraft:      {StatusApproved},
    StatusApproved:   {StatusGenerating, StatusDraft}, // unapprove edge: approved → draft
    StatusGenerating: {StatusPrompted},
    StatusPrompted:   {StatusVerifying},
    StatusVerifying:  {StatusCompleted},
}

// CanTransitionTo returns nil if transitioning from s to target is valid,
// or an error naming both states if the transition is not in the table.
func (s Status) CanTransitionTo(target Status) error {
    for _, allowed := range specTransitions[s] {
        if allowed == target {
            return nil
        }
    }
    return fmt.Errorf("cannot transition spec from %q to %q", s, target)
}
```

Note: `CanTransitionTo` uses `fmt.Errorf` (not `errors.Errorf`) because it has no context parameter — it is a pure value method. This is intentional and correct.

### 1d. Lifecycle predicates on `spec.Status`

From the spec:
- Terminal: `completed`
- Pre-execution: `idea`, `draft`, `approved`, `generating`
- Active: `prompted`, `verifying` (neither pre-execution nor terminal)

```go
// IsTerminal returns true if the spec has reached a final, non-actionable state.
func (s Status) IsTerminal() bool {
    return s == StatusCompleted
}

// IsPreExecution returns true if the spec has not yet entered active processing.
func (s Status) IsPreExecution() bool {
    return s == StatusIdea || s == StatusDraft || s == StatusApproved || s == StatusGenerating
}

// IsActive returns true if the spec is in active processing (neither pre-execution nor terminal).
func (s Status) IsActive() bool {
    return !s.IsPreExecution() && !s.IsTerminal()
}
```

### 1e. Do NOT modify `spec.Load()`

`Load()` stays permissive. It must accept any status string, including legacy values like `queued` (set by `pkg/processor/processor.go::autoSetQueuedStatus` and used in test fixtures). Strict status checking happens at the transition boundary in commands via `CanTransitionTo()`, not at the parse boundary. Do not add `.Validate()` calls inside `Load()`.

## 2. Add lifecycle model to `pkg/prompt/prompt.go`

`PromptStatus` already has `String()`, `Validate()`, `AvailablePromptStatuses`, `PromptStatuses.Contains()`. Add only the missing methods.

### 2a. Prompt transition table and `CanTransitionTo()`

The frozen valid-edges table for prompt (from the spec):
```
idea         → draft
draft        → approved
approved     → executing | cancelled | draft        ← `approved → draft` is the unapprove edge
executing    → committing | failed | cancelled
committing   → completed | failed
failed       → approved | cancelled
in_review    → pending_verification | failed
pending_verification → completed | failed
```

```go
// promptTransitions defines the valid state transitions for prompt lifecycle.
// This is the single source of truth — add one row here to enable a new transition.
var promptTransitions = map[PromptStatus][]PromptStatus{
    IdeaPromptStatus:                {DraftPromptStatus},
    DraftPromptStatus:               {ApprovedPromptStatus},
    ApprovedPromptStatus:            {ExecutingPromptStatus, CancelledPromptStatus, DraftPromptStatus}, // unapprove: approved → draft
    ExecutingPromptStatus:           {CommittingPromptStatus, FailedPromptStatus, CancelledPromptStatus},
    CommittingPromptStatus:          {CompletedPromptStatus, FailedPromptStatus},
    FailedPromptStatus:              {ApprovedPromptStatus, CancelledPromptStatus},
    InReviewPromptStatus:            {PendingVerificationPromptStatus, FailedPromptStatus},
    PendingVerificationPromptStatus: {CompletedPromptStatus, FailedPromptStatus},
}

// CanTransitionTo returns nil if transitioning from s to target is valid,
// or an error naming both states if the transition is not in the table.
func (s PromptStatus) CanTransitionTo(target PromptStatus) error {
    for _, allowed := range promptTransitions[s] {
        if allowed == target {
            return nil
        }
    }
    return fmt.Errorf("cannot transition prompt from %q to %q", s, target)
}
```

Note: `CanTransitionTo` uses `fmt.Errorf` (no context parameter — pure value method).

### 2b. Lifecycle predicates on `prompt.PromptStatus`

From the spec:
- Terminal: `completed`, `cancelled`
- Pre-execution: `idea`, `draft`, `approved`
- Active: `executing`, `committing`, `failed`, `in_review`, `pending_verification` (neither pre-execution nor terminal; `failed` is intentionally Active)

```go
// IsTerminal returns true if the prompt has reached a final, non-actionable state.
func (s PromptStatus) IsTerminal() bool {
    return s == CompletedPromptStatus || s == CancelledPromptStatus
}

// IsPreExecution returns true if the prompt has not yet entered active execution.
func (s PromptStatus) IsPreExecution() bool {
    return s == IdeaPromptStatus || s == DraftPromptStatus || s == ApprovedPromptStatus
}

// IsActive returns true if the prompt is in active processing (neither pre-execution nor terminal).
// Note: FailedPromptStatus is intentionally Active — failed prompts can be re-approved for retry.
func (s PromptStatus) IsActive() bool {
    return !s.IsPreExecution() && !s.IsTerminal()
}
```

### 2c. Do NOT modify `prompt.Load()`

Same rule as spec: `Load()` stays permissive. Tests rely on writing `status: queued` and reading the file back successfully. Do not add `.Validate()` calls inside `Load()`.

## 3. Write tests for `pkg/spec/`

Add a new `Describe("Status lifecycle model", ...)` block in `pkg/spec/spec_test.go` (external `package spec_test`). Do NOT modify existing tests.

### 3a. `AvailableSpecStatuses.Contains`
```go
Expect(spec.AvailableSpecStatuses.Contains(spec.StatusApproved)).To(BeTrue())
Expect(spec.AvailableSpecStatuses.Contains(spec.Status("bogus"))).To(BeFalse())
```

### 3b. `String()`
```go
Expect(spec.StatusApproved.String()).To(Equal("approved"))
Expect(spec.StatusCompleted.String()).To(Equal("completed"))
```

### 3c. `Validate()`
```go
Expect(spec.StatusApproved.Validate(ctx)).To(Succeed())
Expect(spec.Status("bogus").Validate(ctx)).To(MatchError(ContainSubstring("invalid")))
```

### 3d. `CanTransitionTo()` — valid transitions
Test all edges from the frozen table succeed (return nil), including the unapprove edge:
```go
// Forward
Expect(spec.StatusDraft.CanTransitionTo(spec.StatusApproved)).To(Succeed())
Expect(spec.StatusApproved.CanTransitionTo(spec.StatusGenerating)).To(Succeed())
// Backward (unapprove)
Expect(spec.StatusApproved.CanTransitionTo(spec.StatusDraft)).To(Succeed())
```

### 3e. `CanTransitionTo()` — invalid transitions
```go
Expect(spec.StatusDraft.CanTransitionTo(spec.StatusCompleted)).To(HaveOccurred())
Expect(spec.StatusDraft.CanTransitionTo(spec.StatusCompleted)).To(MatchError(ContainSubstring("draft")))
Expect(spec.StatusDraft.CanTransitionTo(spec.StatusCompleted)).To(MatchError(ContainSubstring("completed")))
```

### 3f. Adding a transition row enables the previously-rejected transition
```go
// Verify that idea→completed is currently rejected
Expect(spec.StatusIdea.CanTransitionTo(spec.StatusCompleted)).To(HaveOccurred())
// (This test documents the contract; the table is the only place to change for a new edge)
```

### 3g. Lifecycle predicates
```go
// Terminal
Expect(spec.StatusCompleted.IsTerminal()).To(BeTrue())
Expect(spec.StatusVerifying.IsTerminal()).To(BeFalse())
Expect(spec.StatusApproved.IsTerminal()).To(BeFalse())

// Pre-execution
Expect(spec.StatusIdea.IsPreExecution()).To(BeTrue())
Expect(spec.StatusDraft.IsPreExecution()).To(BeTrue())
Expect(spec.StatusApproved.IsPreExecution()).To(BeTrue())
Expect(spec.StatusGenerating.IsPreExecution()).To(BeTrue())
Expect(spec.StatusPrompted.IsPreExecution()).To(BeFalse())
Expect(spec.StatusCompleted.IsPreExecution()).To(BeFalse())

// Active
Expect(spec.StatusPrompted.IsActive()).To(BeTrue())
Expect(spec.StatusVerifying.IsActive()).To(BeTrue())
Expect(spec.StatusApproved.IsActive()).To(BeFalse())
Expect(spec.StatusCompleted.IsActive()).To(BeFalse())
```

### 3h. Load is permissive — accepts legacy/unknown status strings
Create a temp file with:
```
---
status: queued
---
```
Call `spec.Load(ctx, path, currentDateTimeGetter)`. Expect **no error** and `sf.Frontmatter.Status == "queued"`. This documents the contract that `Load()` does not validate; legacy values like `queued` are normalized later by the processor, not rejected at parse time.

### 3i. Load accepts valid status string
Create a temp file with `status: approved`. Call `spec.Load`. Expect no error and `sf.Frontmatter.Status == "approved"`.

### 3j. Load accepts file with no frontmatter
Create a temp file with no `---` delimiters. Call `spec.Load`. Expect no error.

## 4. Write tests for `pkg/prompt/`

Add a new `Describe("PromptStatus lifecycle model", ...)` block in `pkg/prompt/prompt_test.go`. Do NOT modify existing tests.

### 4a. `CanTransitionTo()` — valid and invalid transitions
Test a representative sample:
```go
// Valid forward
Expect(prompt.DraftPromptStatus.CanTransitionTo(prompt.ApprovedPromptStatus)).To(Succeed())
Expect(prompt.ApprovedPromptStatus.CanTransitionTo(prompt.ExecutingPromptStatus)).To(Succeed())
Expect(prompt.FailedPromptStatus.CanTransitionTo(prompt.ApprovedPromptStatus)).To(Succeed())
Expect(prompt.PendingVerificationPromptStatus.CanTransitionTo(prompt.CompletedPromptStatus)).To(Succeed())

// Valid backward — unapprove edge
Expect(prompt.ApprovedPromptStatus.CanTransitionTo(prompt.DraftPromptStatus)).To(Succeed())

// Invalid
Expect(prompt.DraftPromptStatus.CanTransitionTo(prompt.CompletedPromptStatus)).To(HaveOccurred())
err := prompt.DraftPromptStatus.CanTransitionTo(prompt.CompletedPromptStatus)
Expect(err).To(MatchError(ContainSubstring("draft")))
Expect(err).To(MatchError(ContainSubstring("completed")))
```

### 4b. Lifecycle predicates
```go
// Terminal
Expect(prompt.CompletedPromptStatus.IsTerminal()).To(BeTrue())
Expect(prompt.CancelledPromptStatus.IsTerminal()).To(BeTrue())
Expect(prompt.FailedPromptStatus.IsTerminal()).To(BeFalse())
Expect(prompt.ApprovedPromptStatus.IsTerminal()).To(BeFalse())

// Pre-execution
Expect(prompt.IdeaPromptStatus.IsPreExecution()).To(BeTrue())
Expect(prompt.DraftPromptStatus.IsPreExecution()).To(BeTrue())
Expect(prompt.ApprovedPromptStatus.IsPreExecution()).To(BeTrue())
Expect(prompt.ExecutingPromptStatus.IsPreExecution()).To(BeFalse())
Expect(prompt.FailedPromptStatus.IsPreExecution()).To(BeFalse())

// Active — failed is intentionally active (can be re-approved)
Expect(prompt.ExecutingPromptStatus.IsActive()).To(BeTrue())
Expect(prompt.FailedPromptStatus.IsActive()).To(BeTrue())
Expect(prompt.CommittingPromptStatus.IsActive()).To(BeTrue())
Expect(prompt.InReviewPromptStatus.IsActive()).To(BeTrue())
Expect(prompt.PendingVerificationPromptStatus.IsActive()).To(BeTrue())
Expect(prompt.ApprovedPromptStatus.IsActive()).To(BeFalse())
Expect(prompt.CompletedPromptStatus.IsActive()).To(BeFalse())
Expect(prompt.CancelledPromptStatus.IsActive()).To(BeFalse())
```

### 4c. Load is permissive — accepts legacy `queued` status
Create a temp file with `status: queued` frontmatter. Call `prompt.Load(ctx, path, cdtg)`. Expect **no error** and `pf.Frontmatter.Status == "queued"`. This guards the legacy contract — many existing tests in `pkg/prompt/prompt_test.go` and `pkg/spec/spec_test.go` write `status: queued` and rely on `Load()` accepting it.

### 4d. Load accepts valid status string
Create a temp file with `status: approved`. Call `prompt.Load`. Expect no error.

### 4e. Load accepts file with no frontmatter
Create a temp file with no `---` delimiters. Call `prompt.Load`. Expect no error.

## 5. Write CHANGELOG entry

Add `## Unreleased` at the top of `CHANGELOG.md` if it does not already exist, then append:

```
- refactor: add SpecStatuses/CanTransitionTo/predicates to spec.Status and CanTransitionTo/predicates to prompt.PromptStatus (Load() stays permissive — strict checks at transition boundary)
```

## 6. Run `make test`

```bash
cd /workspace && make test
```

Must pass before proceeding to `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Frontmatter wire format must remain unchanged — `Frontmatter.Status` stays `string` in both structs; typed methods are on the `Status`/`PromptStatus` type, not the struct field
- All existing tests in `pkg/spec/`, `pkg/prompt/`, `pkg/cmd/`, `pkg/specwatcher/`, `pkg/processor/` must pass unchanged — do NOT modify any existing test assertion
- `CanTransitionTo` uses `fmt.Errorf` (no context parameter, pure value method) — this is intentional and correct for a method on a value type
- The frozen valid-edges tables in the spec are the contract — implement them exactly, do not extend or alter
- Pre-execution for spec: idea, draft, approved, generating. Active for spec: prompted, verifying
- Pre-execution for prompt: idea, draft, approved. Active for prompt: executing, committing, failed, in_review, pending_verification. Terminal for prompt: completed, cancelled
- `Load()` stays PERMISSIVE — never call `.Validate()` inside `Load()`. Legacy status values like `queued` (set by processor and used in test fixtures) must continue to load without error. Strict transition checks happen via `CanTransitionTo()` in commands
- External test packages (`package spec_test`, `package prompt_test`)
- Coverage ≥80% for changed packages
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for functions that take `ctx` — never `fmt.Errorf` there
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `grep -n "CanTransitionTo\|IsTerminal\|IsActive\|IsPreExecution" pkg/spec/spec.go` — all four methods present
2. `grep -n "CanTransitionTo\|IsTerminal\|IsActive\|IsPreExecution" pkg/prompt/prompt.go` — all four methods present
3. `grep -n "specTransitions\|promptTransitions" pkg/spec/spec.go pkg/prompt/prompt.go` — one table per file
4. `go test -cover ./pkg/spec/... ./pkg/prompt/... | grep coverage` — coverage ≥80% for both packages
5. `go test ./pkg/spec/... -run "lifecycle model"` — new spec tests pass
6. `go test ./pkg/prompt/... -run "lifecycle model"` — new prompt tests pass
</verification>
