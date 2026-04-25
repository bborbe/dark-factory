---
status: completed
spec: [058-reject-spec-and-prompt]
summary: Added StatusRejected/RejectedPromptStatus constants, IsRejectable() predicates, Rejected/RejectedReason frontmatter fields, StampRejected() helpers, lister.Summary.Rejected counter, and Ginkgo tests for both packages
container: dark-factory-330-spec-058-model
dark-factory-version: v0.132.0
created: "2026-04-25T10:30:00Z"
queued: "2026-04-25T10:49:09Z"
started: "2026-04-25T10:50:19Z"
completed: "2026-04-25T11:01:21Z"
---

<summary>
- `pkg/spec/spec.go` gains a `StatusRejected` constant and it is added to `AvailableSpecStatuses` and the transition table
- `pkg/prompt/prompt.go` gains a `RejectedPromptStatus` constant and it is added to `AvailablePromptStatuses` and the transition table
- Both status types gain an `IsRejectable()` predicate (spec: pre-execution or prompted; prompt: pre-execution only)
- Both `Frontmatter` structs gain `Rejected` and `RejectedReason` fields for audit metadata storage
- `pkg/spec/lister.go` `Summary` struct gains a `Rejected int` field counted from loaded specs
- New Ginkgo tests cover all new methods and transitions in both packages
- No CLI or daemon behavior changes in this prompt — model layer only
</summary>

<objective>
Extend the lifecycle model in `pkg/spec/spec.go` and `pkg/prompt/prompt.go` with a terminal `rejected` status, the valid transition edges from the spec, and the `IsRejectable()` predicate. Add the `Rejected`/`RejectedReason` frontmatter fields that the reject commands will write. This is the model-only prompt — commands come in prompt 2.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-enum-type-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

**Prerequisite**: Spec 057 (lifecycle-state-machine) is already landed — `CanTransitionTo()`, `IsPreExecution()`, `IsTerminal()`, and `IsActive()` already exist on both `spec.Status` and `prompt.PromptStatus`. This prompt builds on top of that existing model.

Key files to read before editing:
- `pkg/spec/spec.go` — current `Status` constants, `AvailableSpecStatuses`, `specTransitions`, predicates, `Frontmatter` struct
- `pkg/prompt/prompt.go` — current `PromptStatus` constants, `AvailablePromptStatuses`, `promptTransitions`, predicates, `Frontmatter` struct
- `pkg/spec/lister.go` — `Summary` struct and `Summary()` method
- `pkg/spec/spec_test.go` — existing test patterns (Ginkgo v2, external `package spec_test`)
- `pkg/prompt/prompt_test.go` — existing test patterns
</context>

<requirements>

## 1. Add `StatusRejected` to `pkg/spec/spec.go`

### 1a. Add the constant

After the existing status constant block (after `StatusCompleted`), add:

```go
// StatusRejected indicates the spec was deliberately abandoned before execution.
// This is a terminal state — rejected specs are moved to specs/rejected/ and never processed again.
StatusRejected Status = "rejected"
```

### 1b. Update `AvailableSpecStatuses`

Add `StatusRejected` to the `AvailableSpecStatuses` slice (append at end):

```go
var AvailableSpecStatuses = SpecStatuses{
    StatusIdea,
    StatusDraft,
    StatusApproved,
    StatusGenerating,
    StatusPrompted,
    StatusVerifying,
    StatusCompleted,
    StatusRejected,
}
```

### 1c. Update `specTransitions`

Add the five rejection edges to the existing `specTransitions` map:

```go
var specTransitions = map[Status][]Status{
    StatusIdea:       {StatusDraft, StatusRejected},
    StatusDraft:      {StatusApproved, StatusRejected},
    StatusApproved:   {StatusGenerating, StatusDraft, StatusRejected}, // unapprove edge: approved → draft
    StatusGenerating: {StatusPrompted, StatusRejected},
    StatusPrompted:   {StatusVerifying, StatusRejected},
    StatusVerifying:  {StatusCompleted},
}
```

`StatusRejected` has no outgoing edges (terminal state) — it is intentionally absent as a key in the map.

### 1d. Add `IsRejectable()` to `spec.Status`

Add after the existing `IsActive()` method:

```go
// IsRejectable returns true if the spec may be rejected from its current state.
// Rejection is allowed from all pre-execution states and from prompted (when all linked
// prompts are themselves rejectable — that additional check is performed by the command).
func (s Status) IsRejectable() bool {
    return s.IsPreExecution() || s == StatusPrompted
}
```

Note: `IsTerminal()` currently returns `s == StatusCompleted`. Do NOT add `StatusRejected` to `IsTerminal()` in this prompt — the existing definition of terminal captures completed. Rejected is a separate kind of terminal. The `IsActive()` implementation will naturally return false for `StatusRejected` since it is neither pre-execution nor the specific `IsTerminal()` check. If you need to make `IsRejected()` explicit, do NOT modify `IsTerminal()` — leave it as-is.

### 1e. Add `Rejected` and `RejectedReason` fields to `spec.Frontmatter`

In the `Frontmatter` struct in `pkg/spec/spec.go`, add two new fields after the existing timestamp fields:

```go
Rejected       string `yaml:"rejected,omitempty"`
RejectedReason string `yaml:"rejected_reason,omitempty"`
```

Add them after the `Completed` field, before `Branch`.

### 1f. Add `StampRejected()` helper to `SpecFile`

After other stamp helpers:

```go
// StampRejected sets the rejected timestamp and reason, then marks status as rejected.
func (s *SpecFile) StampRejected(reason string) {
    s.stampOnce(&s.Frontmatter.Rejected)
    s.Frontmatter.RejectedReason = reason
    s.Frontmatter.Status = string(StatusRejected)
}
```

## 2. Add `RejectedPromptStatus` to `pkg/prompt/prompt.go`

### 2a. Add the constant

After the `CancelledPromptStatus` and `CommittingPromptStatus` constants (at the end of the const block), add:

```go
// RejectedPromptStatus indicates the prompt was deliberately abandoned before execution.
// This is a terminal state — rejected prompts are moved to prompts/rejected/ and never executed.
RejectedPromptStatus PromptStatus = "rejected"
```

### 2b. Update `AvailablePromptStatuses`

Add `RejectedPromptStatus` to the slice (append at end — the exact slice is in `prompt.go`; find it and append).

### 2c. Update `promptTransitions`

Add three rejection edges:

```go
var promptTransitions = map[PromptStatus][]PromptStatus{
    IdeaPromptStatus:                {DraftPromptStatus, RejectedPromptStatus},
    DraftPromptStatus:               {ApprovedPromptStatus, RejectedPromptStatus},
    ApprovedPromptStatus:            {ExecutingPromptStatus, CancelledPromptStatus, DraftPromptStatus, RejectedPromptStatus},
    ExecutingPromptStatus:           {CommittingPromptStatus, FailedPromptStatus, CancelledPromptStatus},
    CommittingPromptStatus:          {CompletedPromptStatus, FailedPromptStatus},
    FailedPromptStatus:              {ApprovedPromptStatus, CancelledPromptStatus},
    InReviewPromptStatus:            {PendingVerificationPromptStatus, FailedPromptStatus},
    PendingVerificationPromptStatus: {CompletedPromptStatus, FailedPromptStatus},
}
```

`RejectedPromptStatus` has no outgoing edges (terminal).

### 2d. Add `IsRejectable()` to `prompt.PromptStatus`

Add after `IsActive()`:

```go
// IsRejectable returns true if the prompt may be rejected from its current state.
// Rejection is only allowed from pre-execution states (idea, draft, approved).
func (s PromptStatus) IsRejectable() bool {
    return s.IsPreExecution()
}
```

### 2e. Add `Rejected` and `RejectedReason` fields to `prompt.Frontmatter`

In `pkg/prompt/prompt.go`, add to the `Frontmatter` struct after `LastFailReason`:

```go
Rejected       string `yaml:"rejected,omitempty"`
RejectedReason string `yaml:"rejected_reason,omitempty"`
```

### 2f. Add `StampRejected()` helper to `PromptFile`

Add a method on `*PromptFile` (following the same pattern as `SetLastFailReason` and other stamp methods in `prompt.go`):

```go
// StampRejected sets the rejected timestamp and reason, then marks status as rejected.
func (pf *PromptFile) StampRejected(reason string) {
    if pf.Frontmatter.Rejected == "" {
        pf.Frontmatter.Rejected = pf.now().UTC().Format(time.RFC3339)
    }
    pf.Frontmatter.RejectedReason = reason
    pf.Frontmatter.Status = string(RejectedPromptStatus)
}
```

Check how `now()` is available on `PromptFile` — look for existing uses of `pf.now()` or `pf.currentDateTimeGetter` to replicate the pattern correctly.

## 3. Update `pkg/spec/lister.go` — `Summary` struct and method

### 3a. Add `Rejected int` to `Summary`

In `pkg/spec/lister.go`, the `Summary` struct currently counts Idea, Draft, Approved, Prompted, Verifying, Completed. Add:

```go
Rejected int
```

### 3b. Update `Summary()` method

In the `switch` statement in `Summary()`, add a case:

```go
case StatusRejected:
    s.Rejected++
```

## 4. Write tests for `pkg/spec/`

Add a new `Describe("rejected status", ...)` block in `pkg/spec/spec_test.go` (external `package spec_test`). Do NOT modify existing tests.

### 4a. `StatusRejected` in `AvailableSpecStatuses`
```go
Expect(spec.AvailableSpecStatuses.Contains(spec.StatusRejected)).To(BeTrue())
```

### 4b. `IsRejectable()` returns true from all rejectable states
```go
Expect(spec.StatusIdea.IsRejectable()).To(BeTrue())
Expect(spec.StatusDraft.IsRejectable()).To(BeTrue())
Expect(spec.StatusApproved.IsRejectable()).To(BeTrue())
Expect(spec.StatusGenerating.IsRejectable()).To(BeTrue())
Expect(spec.StatusPrompted.IsRejectable()).To(BeTrue())
```

### 4c. `IsRejectable()` returns false from non-rejectable states
```go
Expect(spec.StatusVerifying.IsRejectable()).To(BeFalse())
Expect(spec.StatusCompleted.IsRejectable()).To(BeFalse())
Expect(spec.StatusRejected.IsRejectable()).To(BeFalse())
```

### 4d. Valid reject transitions succeed
```go
Expect(spec.StatusIdea.CanTransitionTo(spec.StatusRejected)).To(Succeed())
Expect(spec.StatusDraft.CanTransitionTo(spec.StatusRejected)).To(Succeed())
Expect(spec.StatusApproved.CanTransitionTo(spec.StatusRejected)).To(Succeed())
Expect(spec.StatusGenerating.CanTransitionTo(spec.StatusRejected)).To(Succeed())
Expect(spec.StatusPrompted.CanTransitionTo(spec.StatusRejected)).To(Succeed())
```

### 4e. No outgoing edges from rejected
```go
Expect(spec.StatusRejected.CanTransitionTo(spec.StatusDraft)).To(HaveOccurred())
Expect(spec.StatusVerifying.CanTransitionTo(spec.StatusRejected)).To(HaveOccurred())
Expect(spec.StatusCompleted.CanTransitionTo(spec.StatusRejected)).To(HaveOccurred())
```

### 4f. `StampRejected()` sets all three fields
Create a temp spec file, load it, call `StampRejected("manual smoke")`. Verify:
- `sf.Frontmatter.Status == string(spec.StatusRejected)`
- `sf.Frontmatter.RejectedReason == "manual smoke"`
- `sf.Frontmatter.Rejected != ""`

## 5. Write tests for `pkg/prompt/`

Add a new `Describe("rejected status", ...)` block in `pkg/prompt/prompt_test.go` (external `package prompt_test`). Do NOT modify existing tests.

### 5a. `RejectedPromptStatus` in `AvailablePromptStatuses`
```go
Expect(prompt.AvailablePromptStatuses.Contains(prompt.RejectedPromptStatus)).To(BeTrue())
```

### 5b. `IsRejectable()` returns true from pre-execution states only
```go
Expect(prompt.IdeaPromptStatus.IsRejectable()).To(BeTrue())
Expect(prompt.DraftPromptStatus.IsRejectable()).To(BeTrue())
Expect(prompt.ApprovedPromptStatus.IsRejectable()).To(BeTrue())
Expect(prompt.ExecutingPromptStatus.IsRejectable()).To(BeFalse())
Expect(prompt.FailedPromptStatus.IsRejectable()).To(BeFalse())
Expect(prompt.CompletedPromptStatus.IsRejectable()).To(BeFalse())
Expect(prompt.CancelledPromptStatus.IsRejectable()).To(BeFalse())
Expect(prompt.RejectedPromptStatus.IsRejectable()).To(BeFalse())
```

### 5c. Valid reject transitions succeed
```go
Expect(prompt.IdeaPromptStatus.CanTransitionTo(prompt.RejectedPromptStatus)).To(Succeed())
Expect(prompt.DraftPromptStatus.CanTransitionTo(prompt.RejectedPromptStatus)).To(Succeed())
Expect(prompt.ApprovedPromptStatus.CanTransitionTo(prompt.RejectedPromptStatus)).To(Succeed())
```

### 5d. No outgoing edges from rejected; non-pre-execution cannot be rejected
```go
Expect(prompt.RejectedPromptStatus.CanTransitionTo(prompt.DraftPromptStatus)).To(HaveOccurred())
Expect(prompt.ExecutingPromptStatus.CanTransitionTo(prompt.RejectedPromptStatus)).To(HaveOccurred())
Expect(prompt.CompletedPromptStatus.CanTransitionTo(prompt.RejectedPromptStatus)).To(HaveOccurred())
```

### 5e. `StampRejected()` sets all three fields
Create a temp prompt file, load it, call `StampRejected("abandoned work")`. Verify:
- `pf.Frontmatter.Status == string(prompt.RejectedPromptStatus)`
- `pf.Frontmatter.RejectedReason == "abandoned work"`
- `pf.Frontmatter.Rejected != ""`

## 6. Write CHANGELOG entry

Add `## Unreleased` at the top of `CHANGELOG.md` if it does not already exist, then append:

```
- feat: add rejected status, IsRejectable() predicate, and Rejected/RejectedReason frontmatter fields to spec and prompt lifecycle model
```

## 7. Run `make test`

```bash
cd /workspace && make test
```

Must pass before proceeding to `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Frontmatter wire format is additive: new `rejected` and `rejected_reason` fields are `omitempty` — ignored by older tooling
- `Load()` stays permissive in both packages — do NOT add `Validate()` inside `Load()`. The new `rejected` status must round-trip through `Load()` without error even though `Validate()` would not accept unknown statuses
- Do NOT modify `IsTerminal()` on `spec.Status` — leave it returning `s == StatusCompleted` only. The `StatusRejected` is its own separate terminal concept. `IsActive()` already returns false for anything that isn't pre-execution and isn't the terminal states, so `StatusRejected` will naturally be "inactive" via `IsActive()`
- All existing tests must pass unchanged — do NOT modify any existing test assertion
- External test packages (`package spec_test`, `package prompt_test`)
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for functions with a `ctx` parameter — not `fmt.Errorf` in those contexts
- Coverage ≥80% for changed packages (`pkg/spec/`, `pkg/prompt/`)
- This prompt is model-layer only — no CLI, no factory, no daemon changes
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
```bash
cd /workspace

# Status constants present
grep -n "StatusRejected\|RejectedPromptStatus" pkg/spec/spec.go pkg/prompt/prompt.go

# Transitions include rejected edges
grep -A2 "StatusIdea\|IdeaPromptStatus" pkg/spec/spec.go pkg/prompt/prompt.go | grep -i rejected

# IsRejectable present on both types
grep -n "IsRejectable" pkg/spec/spec.go pkg/prompt/prompt.go

# Frontmatter fields added
grep -n "rejected_reason\|Rejected" pkg/spec/spec.go pkg/prompt/prompt.go

# Coverage
go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/spec/... ./pkg/prompt/... && go tool cover -func=/tmp/cover.out | grep -E "pkg/spec|pkg/prompt" | tail -5

# New tests run
go test ./pkg/spec/... -v -run "rejected status"
go test ./pkg/prompt/... -v -run "rejected status"
```
</verification>
