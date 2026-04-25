---
status: draft
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

This spec **extends** the state machine in two ways:

1. **Adds `rejected` status** to `AvailablePromptStatuses` and `AvailableSpecStatuses` (terminal — no outgoing edges).
2. **Adds edges to the transition tables**:
   - Prompt: `idea → rejected`, `draft → rejected`, `approved → rejected`
   - Spec: `idea → rejected`, `draft → rejected`, `approved → rejected`, `generating → rejected`, `prompted → rejected`
3. **Adds `IsRejectable()` predicate** to both lifecycles:
   - Prompt: `IsRejectable() bool { return s.IsPreExecution() }`
   - Spec: `IsRejectable() bool { return s.IsPreExecution() || s == StatusPrompted }`
   The `prompted` case for specs is allowed by the predicate, but the command additionally requires every linked prompt to be `IsRejectable()` (a runtime check, not part of the predicate).

## Desired Behavior

1. `dark-factory spec reject <name> --reason "<text>"` rejects the spec.
2. `dark-factory prompt reject <name> --reason "<text>"` rejects the prompt.
3. `--reason` is required. Stored verbatim in `rejected_reason` frontmatter field.
4. Rejection is allowed iff the entity's status is in the rejectable set (predicate from state-machine spec):
   - Prompt: `idea`, `draft`, `approved`
   - Spec: `idea`, `draft`, `approved`, `generating`, plus `prompted` iff every linked prompt is rejectable
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
- The cascade is best-effort transactional within a single command run: if any linked prompt fails the rejectable check, the spec reject errors out and no changes are made (to anything).
- Frontmatter wire format is additive: new fields `rejected`, `rejected_reason` are optional and ignored by older tooling.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Reject a prompt currently in `executing`/`committing`/`completed`/`failed`/`in_review` | Error: "cannot reject prompt with status X — pre-execution states only" | Use `cancel` or wait for completion |
| Reject a spec with one or more linked prompts past `approved` | Error listing the offending prompts and their statuses; no changes made | Cancel or complete the linked prompts first |
| Reject already-rejected item | Error: "X is already rejected" | No-op |
| Reject without `--reason` | Error: "--reason is required" | Add the flag |
| Daemon races: spec is being approved while reject runs | File-system locking via existing dark-factory mechanisms; reject errors if the spec was just transitioned out of a rejectable state | Retry |
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
- [ ] Spec reject cascades to all linked prompts; cascade is atomic (all-or-nothing)
- [ ] Daemon (specwatcher + processor) skips rejected files
- [ ] `spec list` and `prompt list` hide rejected by default; `--all` shows them
- [ ] Rejecting an already-rejected, in-flight, or completed item errors with a clear message naming the current status
- [ ] Existing tests still pass; new Ginkgo tests cover reject flow + cascade + daemon-skip
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
