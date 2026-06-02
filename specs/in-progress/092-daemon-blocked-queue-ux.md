---
status: verifying
approved: "2026-06-02T19:07:50Z"
generating: "2026-06-02T19:18:51Z"
prompted: "2026-06-02T19:46:12Z"
verifying: "2026-06-02T21:56:34Z"
branch: dark-factory/daemon-blocked-queue-ux
---

## Summary

- When the daemon refuses to advance the queue, the operator MUST see the reason in `dark-factory status` — no log-grepping required.
- `dark-factory prompt reject --reason <text>` MUST accept prompts in the `failed` terminal state, not just pre-execution states.
- The "previous prompt completed" guard MUST be evaluated per-spec, so a failed prompt on one spec cannot stall unrelated specs.
- Collectively these three changes eliminate the "silent idle daemon" failure mode lived through on 2026-06-02 (spec 058 stalled 40 minutes by orphan prompt 226 of spec 056).
- Sibling vault task `[[dark-factory daemon blocked-queue UX]]` carries the symptom narrative; this spec is the contract.

## Problem

When the daemon's queue-advance guard refuses to fire (prior prompt not in `completed`, prior prompt file missing, etc.), `dark-factory status` reports `Current: idle` with a non-zero queue counter and gives the operator no reason. The only diagnostic path today is reading `.dark-factory.log` for `prompt blocked` entries. On top of that, the operator's only unblock action — `dark-factory prompt reject` — refuses to operate on the `failed` status that triggers the most common block, leaving manual `git mv` of `in-progress/<NNN>-failed.md` to `completed/` as the field workaround. That move lies about state to satisfy a directory-presence check. Finally, the guard's notion of "previous prompt" is global by prompt number; a failed prompt on spec 056 silently blocks unrelated prompts on spec 058 because 058's numbers are higher. The 2026-06-02 incident exhibited all three failures together.

## Goal

After this work, when the daemon refuses to advance the queue:

1. `dark-factory status` shows a `Blocked: NNN (reason=..., missing=MMM)` line under `Queue:` naming exactly which prompt is gating and why.
2. The operator can move a `failed` prompt to `rejected/` with `dark-factory prompt reject NNN --reason "<text>"` — no manual `git mv`.
3. A failed prompt on one spec does not block queue advance for any other spec.

The fix is forward-only on master; the three behaviors are independently observable.

## Non-goals

- Do NOT add auto-recovery / auto-retry for failed prompts — the operator decides intent.
- Do NOT renumber prompts per-spec — keep global prompt numbers; change only the guard's lookup scope.
- Do NOT backport to v0.173.x — fix forward.
- Do NOT add a new notification channel (sound, email, Slack) for the blocker — same surface (`dark-factory status`) operators already use.
- Do NOT generalise into a state-machine refactor — touch only the three transition guards (`reject`, queue-advance, status visibility).
- Do NOT introduce a new prompt status (e.g. `blocked`) — the blocker is a derived view computed at status-render time, not stored state. If a future consumer demands a stored `blocked` state, that's a separate spec.
- Do NOT add an opt-out flag for any of the three behaviors — each is invariant. If a future consumer demands per-project disable, that's a separate spec.

## Desired Behavior

1. **Status surfaces the blocker.** When the daemon's queue-advance guard refuses on the current candidate prompt, `dark-factory status` MUST print a single `Blocked: NNN (reason=<reason>, missing=MMM)` line under the existing `Queue:` block. `NNN` is the candidate prompt number being gated; `<reason>` is one of the enumerated refuse-to-advance categories: `previous-prompt-not-completed`, `previous-prompt-missing`, `prompt-frontmatter-parse-error`, `prompt-file-read-error`, `project-lock-timeout`. `MMM` is the prompt number the guard expected to find when the reason involves a predecessor (`previous-prompt-*`); for the other categories `MMM` is omitted and the format is `Blocked: NNN (reason=<reason>)`. When the queue is empty OR the daemon is actively executing a prompt OR the daemon is idle with no candidate at all (queue counter 0), the `Blocked:` line MUST NOT appear.

2. **`prompt reject` accepts the `failed` status.** `dark-factory prompt reject NNN --reason "<text>"` MUST succeed when the target prompt has frontmatter `status: failed` and lives in `prompts/in-progress/`. The command MUST move the file to `prompts/rejected/<NNN>-rejected.md` and rewrite frontmatter so that `originalStatus: failed` is preserved alongside the new `status: rejected` and `rejectedReason: <text>`. Existing pre-execution states (`idea`, `draft`, `approved`) continue to be accepted with current semantics.

3. **Queue-advance guard is per-spec.** The daemon's "previous prompt is completed" check MUST be scoped to the candidate prompt's spec id. The guard reads the highest-numbered prompt strictly less than the candidate within the same spec and requires it to be `completed`. Prompts of other specs MUST NOT participate in the check. When multiple specs each have a ready candidate, the daemon picks the lowest global prompt number among the per-spec winners — deterministic across runs.

## Constraints

- `AvailablePromptStatuses` typed-string-constant set stays as-is per [[go-enum-type-pattern]] — no new status value introduced.
- Existing `dark-factory status` output format (Queue, Current, Daemon lines) MUST remain byte-stable for the non-blocked path. Existing parsers / scrapers do not regress.
- Existing `prompt reject` behavior on `idea` / `draft` / `approved` prompts is unchanged (same exit code, same file move target).
- The per-spec ordering change reads only `(spec_id, prompt_number)` from the prompt frontmatter / filename; no new field is added to prompt frontmatter (other than the `originalStatus` and `rejectedReason` written by the widened reject path).
- CLI surface via Cobra per `docs/rules/spec-writing.md` and existing dark-factory CLI conventions — no stdlib `flag`, no new top-level subcommands.
- Project lock semantics already in place (file lock per project repo): `prompt reject` acquires the lock; queue-advance cannot fire while reject is mid-transition.
- The daemon log line `prompt blocked file=NNN reason=... missing=MMM` is the source of truth for the `Blocked:` line content — drift between log and status output is a regression.

## Failure Modes

| Trigger | Detection | Expected behavior | Concurrency | Recovery |
|---------|-----------|-------------------|-------------|----------|
| Operator runs `prompt reject 226` while daemon's advance loop is evaluating prompt 226. | Both processes log to `.dark-factory.log`. | Reject acquires project lock first; advance loop blocks until reject completes, then re-reads directory state and sees 226 in `rejected/`. No double-write to 226's file. | Lock is exclusive; advance loop waits with no retry storm. | Operator confirms with `ls prompts/rejected/226-*` (expect exactly one match) and `git status` (expect clean tree, no merge-conflict markers). |
| Operator runs `prompt reject` on a prompt currently being mid-retried by a hypothetical auto-requeue path. | Reject command stdout. | Reject takes precedence: moves file to `rejected/`, frontmatter records both `originalStatus: failed` and `rejectedReason: <text>`. Any in-flight retry observer reads the post-reject state on its next tick. | Project lock serialises. | Audit trail in frontmatter — both fields are durable. |
| `dark-factory status` invoked when `prompts/in-progress/` is empty AND queue counter is 0. | Stdout parse. | No `Blocked:` line in output. Existing `Current: idle` line unchanged. | N/A. | N/A — non-error path. |
| Per-spec guard sees a candidate whose immediate predecessor file is missing entirely (gap in numbering within the spec). | Daemon log. | `Blocked: NNN (reason=previous-prompt-missing, missing=MMM)` surfaced in `status`. No advance. | N/A. | Operator runs `dark-factory status`, reads the `Blocked:` line for `NNN` and `MMM`, then either restores the missing prompt at `prompts/<state>/MMM-*.md` OR rejects the candidate via `dark-factory prompt reject NNN --reason "<text>"`. |
| Two specs each ready: spec A has 220/221 (220 completed, 221 ready), spec B has 222/223 (222 completed, 223 ready). | Daemon log + next-run prompt id. | Daemon picks the lowest global prompt number among per-spec winners — runs 221 (spec A) before 223 (spec B). | Single-process daemon — no cross-process race on selection. | Order is deterministic; rerun produces same selection given same on-disk state. |
| Mid-action crash during `prompt reject NNN`: file already moved to `rejected/` but frontmatter rewrite not yet flushed. | Next `prompt reject` retry, or next status invocation. | File is in `rejected/` with original frontmatter (status still `failed`). Re-running `prompt reject NNN --reason <text>` is idempotent: detects the target file in `rejected/`, completes the frontmatter rewrite, exits 0. | Project lock prevents concurrent retry. | Re-run reject; verify frontmatter shows `originalStatus: failed` and `rejectedReason: <text>`. |
| `dark-factory status` invoked while daemon log is being rotated. | Stdout. | Status uses live in-memory daemon state, not log scrape — `Blocked:` line content is unaffected by log rotation. | N/A. | N/A. |

## Security / Abuse Cases

Out of scope — `prompt reject` and `status` are local CLI commands operating on files the operator already controls. No HTTP surface, no untrusted input. `--reason <text>` is written verbatim into frontmatter; the rewrite path MUST preserve YAML safety (quote / escape) so a reason containing `:` or newlines does not corrupt frontmatter. Evidence: round-trip parse of the rejected prompt's frontmatter after a reject with reason `"a: b\nc"` yields the original string.

## Acceptance Criteria

- [ ] `make precommit` exits 0 in each of the changed packages — evidence: per-package exit code 0 for `pkg/status` (Blocked-line rendering), `pkg/prompt` (widened reject CLI + frontmatter rewrite), `pkg/processor` (per-spec queue-advance guard + lowest-global-number tiebreak), and `pkg/queuescanner` (per-spec predecessor lookup). If any of these packages is untouched by the final diff, that package's precommit run is omitted; every package whose `git diff --stat` is non-empty MUST be covered.
- [ ] **Blocker visible in status (blocked path).** Given a fixture with prompts/in-progress/ containing prompt 227 (spec 058, status `queued`) and prompt 226 (spec 058, status `failed`), invoking `dark-factory status` writes to stdout a line matching `^Blocked: 227 \(reason=previous-prompt-not-completed, missing=226\)$` — evidence: stdout grep of the exact regex returns ≥1 match.
- [ ] **Blocker absent on healthy queue.** Given a fixture with `prompts/in-progress/` empty and `prompts/queued/` empty, `dark-factory status` stdout contains NO line starting with `Blocked:` — evidence: `grep -c '^Blocked:'` on stdout returns 0.
- [ ] **Blocker absent on advanceable queue.** Given a fixture where prompt 226 of spec 058 is `completed` and prompt 227 of spec 058 is `queued`, `dark-factory status` stdout contains no `Blocked:` line — evidence: `grep -c '^Blocked:'` on stdout returns 0.
- [ ] **Reject accepts failed status.** Given a fixture prompt at `prompts/in-progress/226-spec-056-foo-failed.md` with frontmatter `status: failed`, invoking `dark-factory prompt reject 226 --reason "orphan from sibling worktree"` exits 0, the file is now at `prompts/rejected/226-spec-056-foo-rejected.md`, and the new file's frontmatter contains `status: rejected`, `originalStatus: failed`, `rejectedReason: orphan from sibling worktree` — evidence: exit code 0; `ls prompts/rejected/226-*` succeeds; `yq '.status, .originalStatus, .rejectedReason' prompts/rejected/226-*.md` returns the three values in order.
- [ ] **Reject preserves pre-execution behavior.** Given a fixture prompt with `status: draft` in `prompts/in-progress/`, invoking `dark-factory prompt reject NNN --reason "<text>"` exits 0 and moves the file to `prompts/rejected/` with frontmatter `status: rejected`, `rejectedReason: <text>`, and `originalStatus: draft` — evidence: exit code 0; frontmatter fields readback via yq.
- [ ] **Per-spec ordering allows unrelated spec to advance.** Given a fixture with prompt 226 (spec 056) `status: failed` in `prompts/in-progress/` AND prompt 227 (spec 058) `status: queued` in `prompts/queued/` AND prompt 226-equivalent of spec 058 (whatever its number is, e.g. 225) `status: completed`, the daemon advance routine selects prompt 227 for execution — evidence: daemon log line `prompt advanced file=227` appears; no `prompt blocked file=227` line appears within the same advance cycle.
- [ ] **Per-spec deterministic cross-spec ordering.** Given two specs each with one ready candidate (spec A's prompt 221, spec B's prompt 223), the daemon advance routine selects 221 (the lower global number) — evidence: daemon log line `prompt advanced file=221` appears before any advance log line for 223.
- [ ] **Concurrent reject + advance lock semantics.** A test running `prompt reject NNN` and the daemon's advance loop concurrently on the same prompt id results in: exactly one final on-disk state (file in `rejected/`), no merge-conflict markers, advance loop log shows wait-for-lock then post-lock re-read — evidence: `ls prompts/rejected/ | wc -l` returns 1; `git status` clean; advance-loop log grep for `lock acquired` after reject completes.
- [ ] **Frontmatter safety on hostile reason.** `dark-factory prompt reject NNN --reason "a: b\nc"` writes a YAML-safe frontmatter where `yq '.rejectedReason'` reads back the literal string `a: b\nc` — evidence: yq round-trip equals input.
- [ ] **Status–log content parity.** When the daemon emits `prompt blocked file=227 reason=previous-prompt-not-completed missing=226` to `.dark-factory.log`, the same-cycle `dark-factory status` `Blocked:` line carries the same three values (227, previous-prompt-not-completed, 226) — evidence: grep both surfaces in the same test cycle, assert equality of extracted fields.

**Scenario coverage:** No new scenario. All three behaviors are reachable from unit + integration tests against on-disk prompt fixtures and the daemon's advance/status code paths. Concurrency is exercised with a Ginkgo + goroutine integration test on the project lock — no real cluster or external system required.

## Verification

```
make precommit
```

Sibling follow-up (out of this spec's scope, tracked separately): update `~/Documents/Obsidian/Personal/50 Knowledge Base/Dark Factory - Troubleshooting.md` — replace the manual `git mv failed→completed` workaround section with the new `dark-factory prompt reject NNN --reason "<text>"` recipe.

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Widen `prompt reject` to accept `failed`; write `originalStatus` + `rejectedReason` to frontmatter; YAML safety | 2 | reject-accepts-failed, reject-preserves-pre-exec, frontmatter-safety | — |
| 2 | Per-spec queue-advance guard; deterministic lowest-global-number tiebreak across specs | 3 | per-spec-allows-unrelated, per-spec-deterministic-cross-spec, concurrent-reject-advance | prompt 1 (uses widened reject in lock test) |
| 3 | `dark-factory status` derives `Blocked:` line from in-memory guard state; parity with log | 1 | blocker-visible, blocker-absent-empty, blocker-absent-advanceable, status-log-parity | prompt 2 (reads per-spec guard result) |

Rationale: prompt 1 is a self-contained CLI widening usable by prompt 2's lock test. Prompt 2 changes the guard semantics that prompt 3 surfaces. Prompt 3 is purely additive output and depends on prompt 2's guard result shape.

## Do-Nothing Option

Keep the current behavior: operators read `.dark-factory.log`, `git mv` failed prompts to `completed/` to unblock the queue, and accept that any failed prompt on any spec can silently stall every later-numbered spec for an unbounded duration. The 2026-06-02 incident cost 40 minutes on a single operator on a single block; fleet rollout multiplies that. The state-integrity cost (audit trail lies about `failed` prompts being `completed`) compounds across every workaround. Not acceptable.
