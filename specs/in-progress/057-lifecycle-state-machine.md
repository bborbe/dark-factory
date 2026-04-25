---
status: prompted
approved: "2026-04-25T09:23:14Z"
generating: "2026-04-25T09:23:26Z"
prompted: "2026-04-25T09:29:59Z"
branch: dark-factory/lifecycle-state-machine
---

## Summary

- Centralize spec and prompt lifecycle rules in a single state-machine model
- Bring `spec.Status` up to parity with `prompt.PromptStatus` (Validate, String, AvailableStatuses)
- Add lifecycle predicates (`IsTerminal`, `IsActive`, `IsPreExecution`) usable from any command
- Define valid-edges table with `CanTransitionTo()` checks
- Replace ad-hoc status checks in existing commands (`approve`, `unapprove`, `complete`) with the new model
- Pure refactor — no observable behavior change, no new CLI commands, all existing tests pass

## Problem

Spec and prompt lifecycle rules are scattered across command files. Each command hand-rolls its own gate, e.g. `if status != "approved" { return error }`. There is no single place that says "what are the valid transitions" or "what counts as a terminal state". Adding a new command (e.g. `reject`) requires adding more ad-hoc checks, drifting further from a single source of truth.

The spec lifecycle (`pkg/spec/spec.go`) is also weaker than the prompt lifecycle (`pkg/prompt/prompt.go`):

- Prompt has `AvailablePromptStatuses`, `Validate()`, `String()`, type-safe enum.
- Spec has only the raw constants — no Available list, no Validate, no String, and the frontmatter `Status` field is plain `string` rather than the enum type.

This asymmetry makes spec-related code more error-prone than prompt-related code.

## Goal

A single, shared mental model for spec and prompt lifecycles:

- Each entity (spec, prompt) exposes the same surface: enum type, Available collection, Validate, String, predicates, transition table.
- Commands ask the model "can I transition from X to Y?" instead of hard-coding the source state.
- Adding a new state or new command is a one-line change to the transition table.

## Non-goals

- No new CLI commands (no `reject`, no `cancel`, etc. — those are separate specs)
- No change to status string values written to frontmatter (wire-compatible)
- No change to file directory layout (`specs/`, `specs/in-progress/`, etc.)
- No change to daemon behavior
- No change to any user-visible CLI output

## Assumptions

- The prompt lifecycle (`pkg/prompt/prompt.go`) is the authoritative reference shape; the spec lifecycle is brought up to parity.
- The typed methods (`Validate`, `CanTransitionTo`, predicates) live on the `Status` / `PromptStatus` value type, not on the `Frontmatter.Status` struct field. The struct field stays plain `string` — this preserves the existing YAML wire format with zero marshalling work.
- The valid-edges tables below are frozen as part of this spec — they describe the contract, not a starting point.

## Desired Behavior

1. `Validate(ctx)` exists as a method callers may invoke to check whether a status string is in the known enum. **`Load()` is permissive** — it accepts any string into the Frontmatter without calling `Validate()`. Callers that need strict checking opt in.
2. The system validates every transition: a command attempting to move an entity along a non-existent edge errors out with a message naming both the current state and the requested state.
3. Lifecycle predicates (terminal, active, pre-execution) are queryable consistently from any command — both spec and prompt expose the same predicate surface.
4. Adding a new lifecycle state or transition requires editing exactly one declaration per entity. No command file needs to change for the new edge to take effect.
5. Existing commands (`spec approve`, `spec unapprove`, `spec complete`, `prompt approve`, `prompt unapprove`, `prompt complete`) reject the same invalid invocations they reject today. User-visible error messages stay byte-identical for the cases that errored before the refactor. Cases that were silently no-op'd before (e.g., `spec approve` from `prompted`) may now error via `CanTransitionTo()` — that tightening is an intentional improvement, not a regression.
6. After the refactor, no command file in `pkg/cmd/` contains a hard-coded status string comparison.

### Why `Load()` is permissive

The codebase has legacy status values that exist on disk and in test fixtures but are not in the canonical enum (most notably `queued`, which `pkg/processor/processor.go::autoSetQueuedStatus` normalizes to `approved` on the next read). Forcing `Load()` to reject unknown values would break >15 existing tests and the processor's normalization path. The strict guard belongs at the **transition boundary** (`CanTransitionTo()`), where commands actually act on status — not at the parse boundary.

### Valid edges — prompt (frozen)

```
idea         → draft
draft        → approved
approved     → executing | cancelled | draft        ← `approved → draft` is the unapprove edge
executing    → committing | failed | cancelled
committing   → completed | failed
failed       → approved (retry) | cancelled
in_review    → pending_verification | failed
pending_verification → completed | failed
```

Terminal: `completed`, `cancelled`. Pre-execution: `idea`, `draft`, `approved`. Active: any state that is neither pre-execution nor terminal — `executing`, `committing`, `failed`, `in_review`, `pending_verification`. (`failed` is intentionally Active, not terminal: a failed prompt can be re-approved for retry; only `cancelled` and `completed` end the lifecycle.)

### Valid edges — spec (frozen)

```
idea       → draft
draft      → approved
approved   → generating | draft        ← `approved → draft` is the unapprove edge
generating → prompted
prompted   → verifying
verifying  → completed
```

Terminal: `completed`. Pre-execution: `idea`, `draft`, `approved`, `generating`. Active: `prompted`, `verifying`.

The `approved → draft` backward edges are **deliberate**, not accidental. dark-factory's `unapprove` command transitions an approved entity back to draft so it can be revised before re-approval. The state machine is forward-mostly with explicit, documented backward edges; it is not strict-forward-only. Any new backward edge (e.g., `failed → draft`, `cancelled → approved` for a future requeue extension) requires an explicit row in the table.

## Constraints

- Frontmatter wire format must remain unchanged. A spec/prompt file written before the refactor must load cleanly after the refactor, and a file written after the refactor must load cleanly with pre-refactor tooling. Round-trip load/save is byte-identical for unchanged files.
- All existing Ginkgo tests in `pkg/cmd/`, `pkg/spec/`, `pkg/prompt/`, `pkg/specwatcher/`, `pkg/processor/` pass unchanged. No test assertion text or behavior is modified.
- No change to the daemon's filtering logic — daemon still picks up `approved` specs and `approved` prompts.
- Transitions and predicates are defined in exactly one place per entity (one transition table, one predicate set). Grepping `pkg/cmd/` for `Status == "..."` or `status != "..."` returns zero hits after the refactor (verified by the command in the Verification section).
- The frozen valid-edges tables in Desired Behavior are the contract — the implementation does not extend or alter them as part of this spec.
- Method-shape implementation note (frozen): both entities expose an `AvailableStatuses` collection, `String()`, `Validate(ctx)`, `CanTransitionTo(target) error`, plus `IsTerminal()`, `IsActive()`, `IsPreExecution()` predicates. This shape is dictated by the symmetry-with-`prompt.PromptStatus` requirement.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Command attempts an invalid transition | `CanTransitionTo()` returns typed error naming source and target | Fix the command or update the table |
| Legacy status value on disk (e.g., `queued` from older tooling) | `Load()` accepts it; processor's `autoSetQueuedStatus` normalizes it on the next read | No action needed |
| New status added to enum but transition table not updated | `make precommit` fails before the change can ship | Update the table |

## Do-Nothing Option

Cost of keeping the status quo:

- Every new command (reject, retry, requeue) duplicates ad-hoc checks
- Spec and prompt models drift further apart over time
- Subtle bugs from mismatched checks (e.g., one command considers `failed` terminal, another doesn't)
- Refactor cost grows monotonically

Doing the refactor now (before adding `reject`) avoids carrying ad-hoc checks into a fourth and fifth caller.

## Acceptance Criteria

- [ ] An invalid transition (e.g., spec `draft` → `completed`) errors out with a typed error that names both source and target states
- [ ] `Load()` accepts an unknown/legacy status value without erroring (verified by a test that writes `status: queued` and asserts successful load) — strict checking happens at the transition boundary, not at parse
- [ ] `Validate(ctx)` returns a typed error when called on an unknown status value (verified by a test calling `Status("bogus").Validate(ctx)`)
- [ ] Existing commands reject the same invalid invocations they rejected before the refactor — error message text is byte-identical (regression test diff is empty)
- [ ] Adding a transition row enables the previously-rejected transition with no other source changes (no command file, no helper, no test mock — only the table)
- [ ] `! grep -rn 'Status == "' pkg/cmd/` and `! grep -rn 'status != "' pkg/cmd/` both succeed (i.e., zero matches; verification fails loudly if a regression slips in)
- [ ] All existing Ginkgo tests pass unchanged
- [ ] `make precommit` exits 0
- [ ] A round-trip `Load → Save` of any unchanged spec or prompt file produces a byte-identical file

## Verification

```bash
cd ~/Documents/workspaces/dark-factory
make precommit

# These must fail (i.e., grep finds nothing) — script exits non-zero on regression:
! grep -rn 'Status == "' pkg/cmd/
! grep -rn 'status != "' pkg/cmd/
```
