---
status: prompted
approved: "2026-06-10T14:27:36Z"
generating: "2026-06-10T14:27:36Z"
prompted: "2026-06-10T14:37:53Z"
branch: dark-factory/bug-spec-092-contract-violations
---

## Summary

- Spec 092 (`daemon-blocked-queue-ux`) is at `verifying`, but a fresh verification walk at v0.178.2 found five of its acceptance criteria still failing.
- Three are implementation defects: the rejected-reason frontmatter field is written under the wrong YAML key (`rejected_reason` instead of the spec-mandated `rejectedReason`); pre-execution rejects never stamp `originalStatus`; and the daemon's blocked-log line emits a human reason string while status output emits the hyphenated enum, so the two surfaces drift.
- Two are test-coverage gaps: no test proves an unrelated spec advances while a sibling spec is blocked, and no test exercises a concurrent reject + queue-advance against the same prompt file under the project lock.
- This remediation closes those five gaps so spec 092 can pass a fresh verify-spec walk. It does NOT itself complete spec 092 — that walk does, after merge.
- The already-correct Blocked-line format (`Blocked: NNN (...)`) shipped at v0.178.2 must not regress, and all tests must remain hermetic against real `prompts/` state.

## Problem

Spec 092 shipped across three prompts and reached `verifying`, but the as-built implementation violates its own contract in three places and leaves two contractual behaviors unproven. The rejected-reason field is persisted under the YAML key `rejected_reason`, so spec 092's evidence command `yq '.rejectedReason'` returns null against every rejected file — the operator audit trail the spec promised is silently absent. A draft prompt rejected before execution never records `originalStatus`, so the pre-execution audit trail the spec mandates is missing. The daemon logs the blocker reason as the human string `previous prompt not completed` (with spaces) while `dark-factory status` renders the hyphenated enum `previous-prompt-not-completed`; spec 092 declares log/status drift a regression, and that drift exists today. Finally, two load-bearing behaviors — cross-spec queue independence and reject/advance lock serialization — have no test that fails when the behavior breaks; the existing tests assert weaker properties (any-candidate-processed, generic FileLock primitive) that a regression would slip past. While these defects stand, spec 092 cannot pass verification and the operator-facing guarantees it was written to deliver are partially fictional.

## Goal

After this work, against hermetic fixtures:

1. A rejected prompt's frontmatter carries the reason under the YAML key `rejectedReason`, and `yq '.rejectedReason'` reads it back. Existing files written under the old `rejected_reason` key still read correctly.
2. A draft prompt rejected pre-execution carries `originalStatus: draft` alongside `status: rejected` and `rejectedReason: <text>`.
3. The daemon's blocked-log line and `dark-factory status` carry the identical hyphenated reason token, sourced so the two cannot drift, and a parity test derives its expectation from that shared source rather than a hand-written literal.
4. A test proves that when one spec's queue is blocked, an unrelated advanceable spec's candidate is still processed and the blocked spec's candidate is not.
5. A test proves that a concurrent `prompt reject` and queue-advance on the same prompt file serialize under the project lock: exactly one final on-disk state, no corruption, the loser observes post-lock state.

Spec 092 itself remains at `verifying`; this spec's completion is its own verify-spec walk, not spec 092's.

## Non-goals

- Do NOT change the Blocked-line output format (`Blocked: NNN (reason=..., missing=MMM)`) shipped at v0.178.2 — it is correct; this spec must not regress it.
- Do NOT introduce a new prompt status, auto-recovery, or any behavior beyond spec 092's existing contract — this is a remediation of spec 092, not an extension of it.
- Do NOT mark spec 092 `completed` as part of this work — a fresh verify-spec walk after merge does that.
- Do NOT touch real `prompts/` state in tests — fixtures are temp dirs only; `prompts/in-progress/441-*.md` is live operator state.
- Do NOT add a runtime migration that rewrites existing `rejected_reason` files on disk — read-compat is sufficient; if a future consumer demands on-disk migration, that's a separate spec.

## Acceptance Criteria

- [ ] **Reason field uses the camelCase key.** Given a hermetic fixture prompt at `prompts/in-progress/<NNN>-failed.md` with `status: failed`, after `dark-factory prompt reject <NNN> --reason "orphan"`, the rejected file's frontmatter exposes the reason under `rejectedReason` — evidence: `yq '.rejectedReason' prompts/rejected/<NNN>-*.md` returns `orphan` (not `null`).
- [ ] **No file is persisted under the old key.** After the same reject, the rejected file has no `rejected_reason` line — evidence: `grep -c '^rejected_reason:' prompts/rejected/<NNN>-*.md` returns 0.
- [ ] **Backward read-compat for existing files.** A fixture file written with the legacy `rejected_reason: legacy text` key loads such that the typed reason field equals `legacy text` — evidence: a unit/spec test loads the legacy-key fixture and asserts the parsed reason equals `legacy text`; `grep -n` for that test by name returns ≥1.
- [ ] **Pre-exec reject stamps originalStatus.** Given a hermetic fixture prompt with `status: draft` in `prompts/in-progress/`, after `dark-factory prompt reject <NNN> --reason "noop"`, the rejected file carries `originalStatus: draft` — evidence: `yq '.status, .originalStatus, .rejectedReason' prompts/rejected/<NNN>-*.md` returns `rejected`, `draft`, `noop` in order.
- [ ] **Inverted draft-reject test asserts the contract.** The reject test that previously asserted the rejected draft's original-status field was empty now asserts it equals `draft` — evidence: `grep -n 'originalStatus' pkg/cmd/reject_test.go` (or the relevant reject test file) shows an assertion for `draft` and no surviving assertion that the field equals empty string for the draft-pre case.
- [ ] **Scanner log emits the hyphenated enum.** When the daemon blocks on a not-completed predecessor, the blocked-log line contains `reason=previous-prompt-not-completed` (hyphenated, no spaces) — evidence: a scanner test captures the emitted log line and `grep` for `reason=previous-prompt-not-completed` returns ≥1; `grep -c 'reason=previous prompt not completed'` (spaces) returns 0.
- [ ] **Parity test derives from a shared source.** The status–log parity test obtains its expected reason token from the same constant/function the scanner and status code use, not a duplicated string literal — evidence: `grep -n` of the parity test shows it references the shared symbol; the shared symbol is defined in non-test code and `grep -rn` finds the scanner log path and status path both consuming it.
- [ ] **Cross-spec advance test (gap 4).** A new test sets up spec A blocked (predecessor failed or missing) AND spec B advanceable, and asserts spec B's candidate IS processed AND spec A's candidate is NOT — evidence: the test exists (`grep -n` by name in `pkg/queuescanner/scanner_test.go` returns ≥1) and asserts both the positive (B processed) and negative (A not processed) outcomes, not merely "at least one processed".
- [ ] **Concurrent reject + advance test (gap 5).** A new test runs a reject and a scanner advance concurrently on one prompt fixture and asserts exactly one wins — the file ends in exactly one of `rejected/` or `in-progress/`, never duplicated or corrupted — and the loser observes post-lock state via re-read after acquiring the lock — evidence: the test exists (`grep -n` by name in `pkg/queuescanner/scanner_test.go` returns ≥1); within it, a count assertion confirms exactly one final file and an assertion confirms the loser re-reads post-acquire state.
- [ ] **Blocked-line format unchanged (no regression).** The v0.178.2 Blocked-line format is byte-stable — evidence: `grep -n 'Blocked: %d (reason=%s, missing=%d)' pkg/status/formatter.go` returns ≥1 and `git diff pkg/status/formatter.go` shows no change to the two `Blocked:` format strings.
- [ ] **Tests are hermetic.** No test in the changed packages reads or writes the real `prompts/` tree — evidence: the new and modified tests operate on `os.MkdirTemp` paths only; `grep -rn 'prompts/in-progress\|prompts/rejected\|prompts/completed' pkg/queuescanner/*_test.go pkg/cmd/reject_test.go` returns zero references to real (non-temp) prompt paths.
- [ ] **`make precommit` exits 0** in the dark-factory module — evidence: exit code 0.

## Verification

```
# 1. Build / lint / full test pass
cd /Users/bborbe/Documents/workspaces/dark-factory-spec092-fix
make precommit

# 2. Replay spec 092's failing evidence commands against a fresh hermetic reject.
#    (Done inside the package tests; the precommit run above exercises them.)
#    Spot-check the camelCase key and originalStatus survive a round-trip:
#      yq '.rejectedReason'   -> reason string, not null
#      yq '.originalStatus'   -> draft (for a draft reject) / failed (for a failed reject)

# 3. Confirm the scanner log token matches status:
grep -rn 'reason=previous-prompt-not-completed' pkg/queuescanner/
grep -rcn 'previous prompt not completed' pkg/queuescanner/scanner.go   # expect 0 (no spaced literal)

# 4. Confirm the Blocked-line format was not touched:
git diff pkg/status/formatter.go    # expect: empty, or no change to the two Blocked: format strings
```

Expected: `make precommit` exits 0; the spaced reason literal is gone from the scanner; the formatter's Blocked lines are unchanged. After merge, a fresh `/dark-factory:verify-spec 092` walk is the gate that moves spec 092 to `completed` — not this spec.

## Desired Behavior

1. **The rejected-reason field is persisted under `rejectedReason`.** The frontmatter field that carries the reject reason is written to disk under the YAML key `rejectedReason` (camelCase), matching spec 092 Goal §2 and Desired Behavior §2. Existing files on disk that carry the legacy `rejected_reason` key continue to parse into the same typed field (read-compat: accept both keys on read, write only the new key). No on-disk migration of existing files is performed.

2. **Pre-execution rejects record `originalStatus`.** When a prompt is rejected from a pre-execution state (`draft`, `idea`, `approved`), the rewritten frontmatter records the prior status in `originalStatus` (e.g. a draft yields `originalStatus: draft`), the same way a failed-path reject already records `originalStatus: failed`.

3. **The blocked-log line and status output share one reason token.** The daemon's `prompt blocked` log line emits the reason as the hyphenated enum token (`prompt blocked file=NNN reason=previous-prompt-not-completed missing=MMM`), identical to what `dark-factory status` renders. The token is sourced from a single shared definition so the two surfaces cannot drift, and the parity test derives its expected value from that shared source rather than duplicating the literal.

4. **Cross-spec queue independence is proven.** A test demonstrates that with one spec blocked (failed/missing predecessor) and an unrelated spec advanceable, the advanceable spec's candidate is processed and the blocked spec's candidate is not.

5. **Reject/advance lock serialization is proven.** A test demonstrates that a concurrent `prompt reject` and queue-advance on the same prompt file serialize under the project lock: exactly one final, uncorrupted on-disk state, and the loser observes the post-lock state by re-reading after acquiring the lock.

## Constraints

- The Blocked-line format in `pkg/status/formatter.go` (the `Blocked: %d (reason=%s, missing=%d)` and `Blocked: %d (reason=%s)` format strings) is correct as shipped at v0.178.2 and MUST NOT change. AC "Blocked-line format unchanged" locks this.
- All five enumerated reason categories from spec 092 Desired Behavior §1 (`previous-prompt-not-completed`, `previous-prompt-missing`, `prompt-frontmatter-parse-error`, `prompt-file-read-error`, `project-lock-timeout`) remain the canonical tokens. Only the scanner's log emission is being corrected to match them; the status side is already correct.
- Tests MUST be hermetic. `prompts/in-progress/441-fix-prompt-complete-autorelease.md` and the other live files under `prompts/` are real operator state. Fixtures use `os.MkdirTemp` only — same discipline as the v0.178.1 committingrecoverer hermeticity guard (spec 093).
- Spec 092 stays at `status: verifying`; this remediation does not edit spec 092 or mark it complete. Its own verify-spec walk after merge is the gate.
- All previously-passing tests keep passing; `make precommit` is green. The only intentional test-assertion change is inverting the draft-reject `originalStatus` assertion (which encoded the defect) to the spec contract.
- The widened reject CLI surface, per-spec guard semantics, and status-render path that spec 092 already shipped are NOT being re-architected — this spec corrects the field key, the missing stamp, and the log token, and adds two tests. No interface signatures change beyond what read-compat for the legacy key requires.

## Failure Modes

| Trigger | Detection | Expected behavior | Concurrency | Reversibility | Recovery |
|---------|-----------|-------------------|-------------|---------------|----------|
| A prompt file on disk carries the legacy `rejected_reason` key (5 such files exist in `prompts/completed/`). | Load returns empty reason if read-compat is missing. | Loader accepts both `rejected_reason` and `rejectedReason`, populating the same typed field; writes emit only `rejectedReason`. | N/A — read-only on load. | Reversible — no on-disk rewrite of legacy files. | Operator confirms with `yq '.rejectedReason'` after the file is next written; legacy files keep parsing until then. |
| Concurrent `prompt reject NNN` and daemon advance on the same prompt 226. | Both processes log to `.dark-factory.log`. | Project lock serializes; exactly one wins, file ends in exactly one of `rejected/` / `in-progress/`, no double-write, loser re-reads post-lock state. | Lock is exclusive; loser waits then re-reads, no retry storm. | Reversible — single final state. | `ls prompts/rejected/226-* | wc -l` returns 1; `git status` clean. |
| The scanner log token and status token diverge again (e.g. a future edit hardcodes a literal). | Parity test fails because both derive from the shared source; a divergent literal no longer satisfies it. | Suite fails loudly at test time. | N/A. | Reversible — caught before merge. | Author routes both surfaces through the shared symbol; re-run. |
| A new test is added that touches the real `prompts/` tree. | `grep` for non-temp prompt paths in test files surfaces it; or the test mutates live state and fails review. | Test uses `os.MkdirTemp` fixtures only. | N/A. | Reversible. | Author moves the fixture into a temp dir; re-run. |
| The cross-spec test is weakened back to "at least one processed". | The negative assertion (blocked spec's candidate NOT processed) is removed, so the test no longer fails on a regression. | Test asserts BOTH B-processed and A-not-processed; removing the negative assertion is a review-flagged weakening. | N/A. | Reversible. | Reviewer restores the negative assertion. |

## Reproduction

dark-factory version: `v0.178.2` (master at `21f10f4`, this worktree's base).

Spec 092 contract: `specs/in-progress/092-daemon-blocked-queue-ux.md` (status `verifying`).

Defect 1 — frontmatter key drift (fails spec 092 ACs "Reject accepts failed status" and "Frontmatter safety on hostile reason"):

1. The struct field is declared at `pkg/prompt/prompt.go:263` as `RejectedReason string \`yaml:"rejected_reason,omitempty"\`` — snake_case.
2. Spec 092 Goal §2 / Desired Behavior §2 and its AC evidence command use `rejectedReason` (camelCase): `yq '.status, .originalStatus, .rejectedReason'`.
3. Running that yq command against any rejected file returns `null` for `.rejectedReason`.
4. Five live files already carry the old key — `grep -rln 'rejected_reason' prompts/` returns `prompts/completed/{441-spec-092-widen-reject-accept-failed,382-spec-075-foundation,331-spec-058-commands,330-spec-058-model,332-spec-058-list-filtering}.md` — so a clean rename needs read-compat for those. (Root cause: prompt 441 line 17 deliberately kept the snake_case tag "already present" instead of renaming to the spec-mandated camelCase.)

Defect 2 — `originalStatus` never stamped pre-exec (fails spec 092 AC "Reject preserves pre-execution behavior"):

1. `StampRejected` at `pkg/prompt/prompt.go:445` sets `Rejected`, `RejectedReason`, and `Status` — but never `OriginalStatus`.
2. Only `StampRejectedWithOriginal` (`pkg/prompt/prompt.go:456`) sets `OriginalStatus`, and only the failed path calls it.
3. Spec 092 AC requires a rejected draft to carry `originalStatus: draft`.
4. The existing test asserts the defect: `pkg/cmd/reject_test.go:245` asserts `pf.Frontmatter.OriginalStatus` equals `""` for the draft-pre case.

Defect 3 — log/status reason-token drift (fails spec 092 AC "Status–log content parity"):

1. `pkg/queuescanner/scanner.go:182` passes the human string `"previous prompt not completed"` (spaces) into `logBlockedOnce`.
2. `pkg/prompt/prompt.go:1010` and `:1020` return the hyphenated enum `"previous-prompt-not-completed"` used by status output (`pkg/status/status.go:74`).
3. Spec 092 Constraint: "drift between log and status output is a regression."
4. The parity test at `pkg/status/status_test.go:615` asserts a hand-written log literal (`logLine := "prompt blocked file=227 reason=previous-prompt-not-completed spec=058 missing=226"`) instead of the scanner's real output — so the test passes while the real scanner emits the spaced string.

Test gap 4 (fails spec 092 AC "Per-spec ordering allows unrelated spec to advance"):

1. `pkg/queuescanner/scanner_test.go:398` ("selects a candidate from one spec without being blocked by a different spec") uses all-advanceable mocks (`AllPreviousCompletedReturns(true)`) and asserts only that the alphabetic-first candidate is processed.
2. No test sets up a genuinely blocked spec A (failed/missing predecessor) alongside an advanceable spec B and asserts B advances while A does not.

Test gap 5 (fails spec 092 AC "Concurrent reject + advance lock semantics"):

1. `pkg/queuescanner/scanner_test.go:675` ("serializes reject and advance via project lock") tests the generic FileLock primitive and explicitly does NOT invoke a reject against a prompt fixture ("It does not invoke the reject command directly").
2. No test runs a reject and a scanner advance concurrently on one prompt fixture and asserts exactly one wins with the loser re-reading post-lock state.

## Expected vs Actual

**Expected (per spec 092):** `yq '.rejectedReason'` returns the reason on a rejected file; a rejected draft carries `originalStatus: draft`; the scanner's blocked-log line carries `reason=previous-prompt-not-completed` identical to status; a test proves unrelated specs advance while a sibling is blocked; a test proves concurrent reject + advance serialize under the project lock.

**Actual (at v0.178.2):** `yq '.rejectedReason'` returns `null` (field persisted as `rejected_reason`); a rejected draft has empty `originalStatus`; the scanner logs `reason=previous prompt not completed` (spaced) while status renders the hyphenated enum; the cross-spec test asserts only "at least one processed" with all-advanceable mocks; the concurrency test exercises only the FileLock primitive, never a reject against a prompt fixture.

## Why this is a bug

Spec 092 is the authoritative contract (`specs/in-progress/092-daemon-blocked-queue-ux.md`, status `verifying`). Its acceptance criteria cite exact evidence commands — `yq '.rejectedReason'`, `originalStatus: draft`, log/status parity — that the as-built implementation fails. The field-key drift and missing stamp mean the operator audit trail the spec promised is silently absent; the log/status drift is named a regression in spec 092's own Constraints. The two test gaps mean two load-bearing behaviors (cross-spec independence, lock serialization — the very behaviors that fix the 2026-06-02 silent-idle incident) have no regression lock: a future change could break them and every test would still pass. Confirmed by code reading at the cited line numbers and by `grep` of the five live files carrying the legacy key.

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Rename the rejected-reason YAML tag to `rejectedReason` with read-compat for legacy `rejected_reason`; make `StampRejected` stamp `originalStatus`; invert the draft-reject test assertion | 1, 2 | reason-key, no-old-key, read-compat, originalStatus, inverted-test | — |
| 2 | Emit the hyphenated enum in the scanner's blocked-log line via a shared reason source; rewrite the parity test to derive its expectation from that source | 3 | scanner-log-enum, parity-shared-source, blocked-format-unchanged | — |
| 3 | Add the cross-spec advance test (B advances, A blocked) and the concurrent reject+advance lock test, both hermetic | 4, 5 | cross-spec-advance, concurrent-reject-advance, hermetic | prompt 1 (lock test exercises the widened reject path) |

Rationale: prompts 1 and 2 are independent single-package corrections (prompt frontmatter vs scanner log). Prompt 3 adds the two regression-lock tests; its concurrency test drives the reject path that prompt 1 corrects, so it depends on prompt 1. `make precommit` and the no-regression Blocked-format check ride along in every prompt.

## Do-Nothing Option

If we do not remediate, spec 092 cannot pass its verify-spec walk and stays stuck at `verifying` indefinitely. The operator-facing guarantees it shipped to deliver remain partially fictional: `yq '.rejectedReason'` keeps returning null so the reject audit trail is unreadable by the documented command; pre-execution rejects keep losing their `originalStatus` provenance; and the log/status drift the spec explicitly forbade persists, so an operator correlating the daemon log against `status` sees two different reason strings. The two untested behaviors remain unprotected, inviting a silent regression of exactly the cross-spec-independence fix that closed the 2026-06-02 incident. Not acceptable — the spec is in flight and these are its own contract violations.
