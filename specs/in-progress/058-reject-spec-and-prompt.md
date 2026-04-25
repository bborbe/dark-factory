---
status: prompted
approved: "2026-04-25T10:23:23Z"
generating: "2026-04-25T10:23:23Z"
prompted: "2026-04-25T10:31:08Z"
branch: dark-factory/reject-spec-and-prompt
---

## Summary

- New CLI commands `dark-factory spec reject <name>` and `dark-factory prompt reject <name>`
- Adds a terminal `rejected` status, distinct from `unapprove` (which goes back to `draft`) and `cancelled` (which is post-execution stop)
- Allowed only from pre-execution states; gated via lifecycle predicates from the state-machine refactor
- Rejected files move to new `specs/rejected/` and `prompts/rejected/` directories with numeric prefix preserved (audit trail)
- Spec reject cascades to all linked prompts (which must themselves still be rejectable)
- Audit metadata recorded: `rejected: <timestamp>`, `rejected_reason: <text>` in frontmatter

## Problem

The current model has three ways to remove a spec or prompt from the active queue, but none of them mean "we decided not to do this":

| Command | Effect | Fits "abandoned"? |
|---------|--------|-------------------|
| `unapprove` | Sends spec/prompt back to `draft` | No — implies we'll come back to it |
| `prompt cancel` | Stops a running execution; result status is `cancelled` | No — only valid mid-flight (executing/committing) |
| Manual `mv` to a homemade `rejected/` directory | Hand-edited frontmatter, hand-moved file | Brittle, no audit trail, no consistency |

We have repeatedly hit this gap. Most recently: spec 013 in the trading repo had three prompts that we abandoned. The workaround was `mkdir prompts/rejected && mv ... && hand-edit frontmatter`. This pattern recurs and deserves a first-class command.

## Goal

A single command that captures "we won't pursue this work item" with proper audit metadata, file placement, and lifecycle integrity.

After this spec lands:

- Killing a stale prompt: one command, file moves to `prompts/rejected/`, reason recorded
- Killing a stale spec with non-started prompts: one command, cascade rejects everything
- The daemon never picks up rejected items (terminal state)
- The audit log shows when it was rejected, why, and by what command

## Non-goals

- Not a replacement for `unapprove` — the two commands have different intents and both stay
- Not a replacement for `cancel` (post-execution) — `reject` is pre-execution only
- No `unreject` command — rejection is terminal; revisit by creating a new spec/prompt
- No automatic rejection (e.g. timeout-based) — operator-driven only
- No retroactive cleanup of past manually-moved rejected/ directories

## Depends on

- `lifecycle-state-machine.md` — this spec consumes `IsPreExecution()` from the state machine. Land that spec first.

### Definitions

- **Linked prompts** (of a spec): all prompts whose YAML frontmatter `spec:` array contains this spec's identifier. Discovered by scanning `prompts/` (inbox), `prompts/in-progress/`, and `prompts/completed/` for matching frontmatter — not by directory location.

### Extensions to the state machine

This spec **extends** spec 057's state machine in three ways:

1. **Adds `rejected` status** to `AvailablePromptStatuses` and `AvailableSpecStatuses` (terminal — no outgoing edges).
2. **Adds edges to the transition tables**:
   - Prompt: `idea → rejected`, `draft → rejected`, `approved → rejected`
   - Spec: `idea → rejected`, `draft → rejected`, `approved → rejected`, `generating → rejected`, `prompted → rejected`
3. **Adds `IsRejectable()` predicate** to both lifecycles. Predicate semantics is **state-only** — it answers "is this state ever a candidate for rejection?":
   - Prompt: `IsRejectable() bool { return s.IsPreExecution() }` — covers idea, draft, approved
   - Spec: `IsRejectable() bool { return s.IsPreExecution() || s == StatusPrompted }` — covers idea, draft, approved, generating, prompted

   The predicate is **necessary but not sufficient** for spec rejection in the `prompted` state. The reject command performs an **additional runtime check** (every linked prompt must itself satisfy `IsRejectable()`). This split keeps the predicate pure (no I/O) while letting the command do the cross-entity validation.

## Desired Behavior

1. `dark-factory spec reject <name> --reason "<text>"` rejects the spec.
2. `dark-factory prompt reject <name> --reason "<text>"` rejects the prompt.
3. `--reason` is required. Stored verbatim in `rejected_reason` frontmatter field.
4. Rejection is allowed when both checks pass:
   - **State check** (the `IsRejectable()` predicate, state-only): prompt status must be in `{idea, draft, approved}`; spec status must be in `{idea, draft, approved, generating, prompted}`.
   - **Cross-entity check** (only for specs in `prompted`): every linked prompt must additionally satisfy `IsRejectable()`. Performed by the command, not the predicate.
5. On reject:
   - Status set to `rejected`, `rejected: <RFC3339 UTC timestamp>` set, `rejected_reason: <text>` set
   - File moved to `specs/rejected/` or `prompts/rejected/` (numeric prefix preserved — no renumber)
   - For specs: every linked prompt is also rejected (cascade), with the same reason
6. The daemon (specwatcher + processor) ignores files in `rejected/` directories. Status `rejected` filters out of any "active" lookups.
7. `dark-factory spec list` and `dark-factory prompt list` hide rejected entries by default. A `--all` flag (or `--include-rejected`) shows them.
8. Attempting to reject an already-rejected item errors clearly. Attempting to reject a started/completed item errors with the current status named.

## Constraints

- The `rejected/` directories are auto-created on first reject — no setup step needed.
- Rejected files keep their numeric prefix; rejection does not renumber surviving items in `in-progress/` or `inbox/`. (Distinct from `unapprove`, which renumbers.)
- **Cascade atomicity** — pre-flight + serial commit, no rollback:
  1. **Pre-flight**: load every linked prompt; verify each is `IsRejectable()`. If any prompt fails the check, the command errors out before mutating any file. Zero side effects on validation failure.
  2. **Commit**: after pre-flight passes, perform mutations serially: each linked prompt's frontmatter is updated and the file moved to `prompts/rejected/`; finally the spec's frontmatter is updated and moved to `specs/rejected/`.
  3. **Mid-cascade FS error**: if a move or write fails mid-cascade (e.g., disk full, permission error), the command stops, leaves partial state on disk, and surfaces a clear error listing which items succeeded and which did not. The operator is responsible for cleanup. (Justification: implementing real two-phase commit on the file system is disproportionate to the failure rate — pre-flight catches the common "some prompt is already executing" case, and the residual FS-error path is rare and operator-recoverable.)
- Frontmatter wire format is additive: new fields `rejected`, `rejected_reason` are optional and ignored by older tooling.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Reject a prompt currently in `executing`/`committing`/`completed`/`failed`/`in_review` | Error: "cannot reject prompt with status X — pre-execution states only" | Use `cancel` or wait for completion |
| Reject a spec with one or more linked prompts past `approved` | Error listing the offending prompts and their statuses; no changes made | Cancel or complete the linked prompts first |
| Reject already-rejected item | Error: "X is already rejected" | No-op |
| Reject without `--reason` | Error: "--reason is required" | Add the flag |
| Daemon races: spec is being approved while reject runs | The `.dark-factory.lock` flock-based daemon lock + per-command file load+write sequence makes the race extremely narrow; if it occurs, reject errors when re-loading the spec finds it past a rejectable state | Retry |
| `rejected/` directory missing | Created on demand | n/a |

## Do-Nothing Option

Without this command:
- Continued ad-hoc `mkdir + mv + hand-edit frontmatter` for every abandoned spec
- No audit trail of why work was killed
- Risk of editing the file inconsistently (status not updated, file moved without metadata, etc.)
- Future commands and tooling cannot reliably distinguish "abandoned" from "draft"

The cost of doing nothing is small per occurrence but recurring. The state-machine spec already paves most of the road.

## Acceptance Criteria

- [ ] `dark-factory spec reject <name> --reason "<text>"` command exists and works
- [ ] `dark-factory prompt reject <name> --reason "<text>"` command exists and works
- [ ] `--reason` is required; missing flag errors out
- [ ] Rejected entities have `status: rejected`, `rejected: <timestamp>`, `rejected_reason: <text>` in frontmatter
- [ ] Rejected files land in `specs/rejected/` or `prompts/rejected/` with numeric prefix preserved
- [ ] Spec reject cascades to all linked prompts; pre-flight catches non-rejectable prompts and aborts before any mutation
- [ ] Daemon (specwatcher + processor) skips rejected files
- [ ] `spec list` and `prompt list` hide rejected by default; `--all` shows them
- [ ] Rejecting an already-rejected, in-flight, or completed item errors with a clear message naming the current status
- [ ] All existing Ginkgo tests pass unchanged
- [ ] New Ginkgo tests cover: prompt reject from each rejectable state, spec reject without prompts, spec reject with cascade, spec reject pre-flight failure (one bad prompt), daemon skips files in `rejected/` directories
- [ ] `make precommit` exits 0

## Verification

```bash
cd ~/Documents/workspaces/dark-factory
make precommit

# End-to-end smoke (in a temp sandbox):
# 1. Create a throwaway spec, approve it, generate prompts (don't run them).
# 2. dark-factory spec reject test-spec --reason "manual smoke"
# 3. Confirm spec lands in specs/rejected/ with status rejected and reason.
# 4. Confirm all linked prompts also moved to prompts/rejected/ with same reason.
# 5. Restart daemon; confirm it does not pick up the rejected items.
# 6. dark-factory spec list — rejected hidden by default; --all shows them.
```
