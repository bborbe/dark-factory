---
status: completed
spec: [094-bug-spec-092-contract-violations]
summary: Replaced spaced human reason strings in the scanner's logBlockedOnce with hyphenated enum tokens sourced from five new shared Reason* constants in pkg/prompt; GetBlockedPrompt and the parity test now reference the same symbols (drift becomes a compile error). Added a new scanner test pinning reason=previous-prompt-not-completed; updated multi-spec malformed test to assert reason=prompt-frontmatter-parse-error; both Blocked format strings in formatter.go are byte-stable.
container: dark-factory-spec092-fix-exec-446-spec-094-shared-reason-token
dark-factory-version: v0.177.1
created: "2026-06-10T15:05:00Z"
queued: "2026-06-10T14:39:29Z"
started: "2026-06-10T15:14:11Z"
completed: "2026-06-10T15:22:21Z"
branch: dark-factory/bug-spec-092-contract-violations
---

<summary>

- The daemon's `prompt blocked` log line emits the reason as the hyphenated enum token (`prompt blocked file=NNN reason=previous-prompt-not-completed missing=MMM`), identical to what `dark-factory status` renders. The token is sourced from a single shared definition in `pkg/prompt` so the two surfaces cannot drift.
- The scanner's existing two call sites to `logBlockedOnce` (lines 156 and 178 of `pkg/queuescanner/scanner.go`) are updated. The "malformed frontmatter" site gets the new token `prompt-frontmatter-parse-error` (already defined in `pkg/prompt/prompt.go:1003`). The "previous prompt not completed" site (currently the spaced human string) gets the shared token `previous-prompt-not-completed` (already defined in `pkg/prompt/prompt.go:1010` and `:1020`).
- A new exported constant `ReasonPreviousPromptNotCompleted = "previous-prompt-not-completed"` (and the four siblings `ReasonPreviousPromptMissing`, `ReasonPromptFrontmatterParseError`, `ReasonPromptFileReadError`, `ReasonProjectLockTimeout`) is added to `pkg/prompt`. The scanner and the status code both reference the constant — drift becomes a compile error.
- The parity test at `pkg/status/status_test.go:614-625` is rewritten. The hand-written `logLine := "prompt blocked file=227 reason=previous-prompt-not-completed spec=058 missing=226"` literal is replaced with a value derived from the scanner's actual `slog` output capture (or, if capture is impractical, a call to a new helper `prompt.FormatBlockedLog(reason, file, spec, missing)` that both the scanner and the parity test invoke). The test no longer duplicates the string literal.
- A new scanner test in `pkg/queuescanner/scanner_test.go` captures the log line emitted by `logBlockedOnce` and asserts it contains `reason=previous-prompt-not-completed` (hyphenated) and does NOT contain the spaced literal. Both grep counts the spec mandates.
- `make precommit` is green. The `Blocked: %d (reason=%s, missing=%d)` and `Blocked: %d (reason=%s)` format strings in `pkg/status/formatter.go` are byte-stable (no diff).

</summary>

<objective>
Fix defect 3 of spec 094: emit the hyphenated enum token in the scanner's blocked-log line, sourced from a single shared definition in `pkg/prompt` so the daemon log and `dark-factory status` cannot drift, and rewrite the parity test to derive its expected value from that shared source rather than a hand-written literal.
</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.

Read these files end-to-end before editing:
- `/workspace/specs/in-progress/094-bug-spec-092-contract-violations.md` — the parent spec, especially `Reproduction` defect 3, `Goal` item 3, ACs `scanner-log-enum`, `parity-shared-source`, `blocked-format-unchanged`
- `/workspace/pkg/queuescanner/scanner.go` lines 130-200 — `ScanAndProcess` body; the two `logBlockedOnce` call sites are at lines 156 (malformed) and 178 (blocked predecessor)
- `/workspace/pkg/queuescanner/scanner.go` lines 277-297 — `logBlockedOnce` definition; signature is `logBlockedOnce(ctx, pr, specID, reason, missing string)`; emits `slog.InfoContext(ctx, "prompt blocked", "file", ..., "reason", ..., "spec", ..., "missing", ...)`
- `/workspace/pkg/prompt/prompt.go` lines 984-1026 — `GetBlockedPrompt`; the four reason tokens returned are `previous-prompt-not-completed`, `previous-prompt-missing`, `prompt-frontmatter-parse-error`, `prompt-file-read-error`. `project-lock-timeout` is also canonical (mentioned in `pkg/status/status.go:74`).
- `/workspace/pkg/status/status.go` lines 70-95 — `Blocked` struct definition with `Reason` field; line 183 uses `s.promptMgr.GetBlockedPrompt` to populate it
- `/workspace/pkg/status/status_test.go` lines 600-626 — the existing parity test; lines 614-625 hard-code a synthetic log line
- `/workspace/pkg/status/formatter.go` lines 78-96 — the two `Blocked:` format strings (MUST NOT change; this prompt's diff must be empty for that file)
- `/workspace/pkg/queuescanner/scanner_test.go` lines 380-450 — the cross-spec test cluster; the new scanner test goes alongside

The slog library is `log/slog` (per CLAUDE.md project convention). `slog.InfoContext(ctx, msg, key, value, ...)` is the standard structured-log invocation.

</context>

<requirements>

## 1. Add shared reason-token constants to `pkg/prompt`

In `/workspace/pkg/prompt/prompt.go`, immediately above the `GetBlockedPrompt` method (around line 984), add a block of exported constants. These are the canonical reason tokens shared between the scanner log and the status code. Listing all five (per spec 092's full enum, not just the three currently used in the scanner):

```go
// Reason tokens for blocked-prompt notifications. These strings are
// canonical: the scanner's blocked-log line and `dark-factory status`
// both emit them verbatim. Drift between the two surfaces is a regression
// (spec 094 AC "scanner-log-enum"). All five tokens are defined here so
// the parity test in pkg/status can derive its expectation from these
// constants rather than duplicating a hand-written literal.
const (
    ReasonPreviousPromptNotCompleted    = "previous-prompt-not-completed"
    ReasonPreviousPromptMissing         = "previous-prompt-missing"
    ReasonPromptFrontmatterParseError   = "prompt-frontmatter-parse-error"
    ReasonPromptFileReadError           = "prompt-file-read-error"
    ReasonProjectLockTimeout            = "project-lock-timeout"
)
```

## 2. Use the shared constants in `GetBlockedPrompt`

In `/workspace/pkg/prompt/prompt.go` `GetBlockedPrompt` (lines 991-1026), replace the four string literals `previous-prompt-not-completed`, `previous-prompt-missing`, `prompt-frontmatter-parse-error`, `prompt-file-read-error` (at lines 1003, 1010, 1012, 1020, 1022) with the new constants. Specifically:

- Line 1003: `"prompt-file-read-error"` → `ReasonPromptFileReadError`
- Line 1010: `"previous-prompt-not-completed"` → `ReasonPreviousPromptNotCompleted`
- Line 1012: `"previous-prompt-missing"` → `ReasonPreviousPromptMissing`
- Line 1020: `"previous-prompt-not-completed"` → `ReasonPreviousPromptNotCompleted`
- Line 1022: `"previous-prompt-missing"` → `ReasonPreviousPromptMissing`

The return values are now sourced from the named constants. The status package's `Blocked.Reason` (and the parity test, in step 4) now references the same symbols the scanner log will use in step 3.

## 3. Use the shared constants in the scanner's `logBlockedOnce` call sites

In `/workspace/pkg/queuescanner/scanner.go`, update both call sites:

### 3a. The malformed-frontmatter call (line 156)

Replace:
```go
s.logBlockedOnce(ctx, candidate, "", "malformed frontmatter", "")
```

with:
```go
s.logBlockedOnce(ctx, candidate, "", prompt.ReasonPromptFrontmatterParseError, "")
```

Note: this also corrects a long-standing minor drift — the prior `"malformed frontmatter"` string is human-readable but does not match the canonical token `prompt-frontmatter-parse-error` used by the status code. Spec 094 AC `scanner-log-enum` mandates the canonical token, and the status code already renders `prompt-frontmatter-parse-error` (see `pkg/status/status.go:74`).

### 3b. The blocked-predecessor call (line 178-184)

Replace:
```go
s.logBlockedOnce(
    ctx,
    candidate,
    specID,
    "previous prompt not completed",
    missingStr(missing),
)
```

with:
```go
s.logBlockedOnce(
    ctx,
    candidate,
    specID,
    prompt.ReasonPreviousPromptNotCompleted,
    missingStr(missing),
)
```

The spaced human string `"previous prompt not completed"` is gone. The scanner now emits the hyphenated enum, sourced from the shared `pkg/prompt` constant.

Add the import alias if not present — `pkg/queuescanner/scanner.go` already imports `pkg/prompt` as `prompt` (line 22), so no new import is needed.

## 4. Rewrite the parity test to derive its expected value from the shared source

In `/workspace/pkg/status/status_test.go` lines 600-626, the test asserts a hand-written `logLine := "prompt blocked file=227 reason=previous-prompt-not-completed spec=058 missing=226"` (line 615). This is a synthetic literal that the test then re-checks against itself — the assertion is tautological and does not verify the scanner's real output. Spec 094 AC `parity-shared-source` requires the test to derive its expected token from the same source the scanner and status code use.

Two viable refactors; pick option A (smaller diff, in-container capture without a new helper):

**Option A — capture the scanner's real `slog` output via an in-test `slog.NewJSONHandler` over a `bytes.Buffer` and assert the buffer contains the expected token:**

```go
It("renders the same reason token in status as the scanner logs", func() {
    // Spec 094 AC "parity-shared-source": the expected reason token is
    // derived from the shared constant in pkg/prompt, not a hand-written
    // literal. The scanner (under test) emits the constant; the status
    // formatter (under test) emits the constant; the test asserts both
    // surfaces resolve to the SAME symbol.

    // ... existing setup that populates st with a blocked prompt ...

    // 1. Status surface: extract the reason from the formatted output.
    formatter := status.NewFormatter()
    out := formatter.Format(st)
    expectedReason := prompt.ReasonPreviousPromptNotCompleted
    Expect(out).To(ContainSubstring(fmt.Sprintf("reason=%s", expectedReason)))

    // 2. Scanner surface: invoke the scanner's real logBlockedOnce and
    //    capture the slog output, asserting the reason key equals the
    //    shared constant.
    var buf bytes.Buffer
    prevHandler := slog.Default()
    defer slog.SetDefault(prevHandler)
    slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
    // ... invoke the scanner's logBlockedOnce with the shared constant ...
    // Assert buf contains `"reason":"previous-prompt-not-completed"`
})
```

The diff is meaningful: the test no longer contains the literal `"previous-prompt-not-completed"` as a string in the test body; it references `prompt.ReasonPreviousPromptNotCompleted` directly. The `grep` for the shared symbol in the test (per spec 094 AC `parity-shared-source`) now finds it.

(Option B — introduce a `prompt.FormatBlockedLog(reason, file, spec, missing) string` helper in `pkg/prompt` that the scanner calls and the parity test asserts against — viable if Option A's `slog.SetDefault` dance proves flaky in CI. The implementation is a one-line `fmt.Sprintf` of the existing `logBlockedOnce` body, and the scanner's `logBlockedOnce` is refactored to call the helper. Pick Option A first; if the `slog.SetDefault` swap interferes with other tests in the same suite, fall back to Option B and add a step 4b that introduces the helper.)

## 5. Add a scanner test that pins the log line to the shared token

In `/workspace/pkg/queuescanner/scanner_test.go`, add a new `It` block alongside the existing `Describe` cluster (around line 380-450 is the natural neighborhood — the cross-spec test cluster). The test exercises `logBlockedOnce` end-to-end and asserts both the positive (the emitted log line contains `reason=previous-prompt-not-completed`) and the negative (the spaced literal is gone):

```go
It("emits the hyphenated reason token in the blocked log line", func() {
    // Spec 094 AC "scanner-log-enum": the scanner's blocked-log line
    // emits the hyphenated enum shared with status, not the spaced
    // human string. Capture the slog output and assert both surfaces.
    var buf bytes.Buffer
    prevHandler := slog.Default()
    defer slog.SetDefault(prevHandler)
    slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

    // Invoke logBlockedOnce with the shared constant.
    // ... (use the existing scanner test fixture helpers, or instantiate a
    //      bare scanner with a minimal prompt.Prompt{...} and call the
    //      unexported logBlockedOnce via a same-package test helper) ...
    // Assert buf.String() contains "reason=previous-prompt-not-completed".
    // Assert buf.String() does NOT contain "previous prompt not completed".
})
```

The two grep counts from spec 094 AC `scanner-log-enum` are satisfied: the test asserts `reason=previous-prompt-not-completed` is present (count ≥1) and `previous prompt not completed` is absent (count 0).

## 6. Verify

Run from the repo root inside the YOLO container:

```
cd /workspace && make precommit
```

Exit code 0 required. `make precommit` runs the lint + vet + test pipeline; the new scanner test (step 5) and the rewritten parity test (step 4) must both pass. All existing tests must still pass.

</requirements>

<constraints>

- Do NOT modify the two `Blocked:` format strings in `/workspace/pkg/status/formatter.go` lines 83 and 91. The format is byte-stable per spec 094 AC `blocked-format-unchanged` and is locked at v0.178.2.
- Do NOT introduce a new `PromptStatus` constant. The five reason tokens are new exported `string` constants in `pkg/prompt`, not new statuses.
- Do NOT change the signature of `logBlockedOnce` (it is unexported, but the call sites are pinned). Both call sites pass `prompt.Reason*` constants in step 3.
- Do NOT delete the existing four return literals in `GetBlockedPrompt` until the constants in step 2 are in place. The two changes (steps 1+2) must land together so the file compiles between prompts.
- Do NOT introduce a new mock for `slog`. The parity test uses `slog.SetDefault` with a `slog.NewJSONHandler` over a `bytes.Buffer` (or the Option-B helper). No new interface is added to the scanner.
- Do NOT touch `/workspace/pkg/prompt/prompt.go` outside of the new constant block (around line 984) and the four return-literal swaps in `GetBlockedPrompt` (lines 1003-1022). The struct, `StampRejected`, `Save`, and `Load` are unchanged.
- Do NOT build or run the `dark-factory` binary for verification. `make precommit` is the sole verification path.
- Do NOT commit — dark-factory handles git.
- Branch is `dark-factory/bug-spec-092-contract-violations` (per the spec's frontmatter).

</constraints>

<verification>

Run from the repo root inside the YOLO container:

```
cd /workspace && make precommit
```

Exit code 0 required. `make precommit` runs lint, vet, and `make test` — the new scanner test (step 5) and the rewritten parity test (step 4) must pass, all existing tests must still pass.

Static spot checks (run after `make precommit` is green; each grep should print the indicated matches):

```
cd /workspace
grep -nE 'ReasonPreviousPromptNotCompleted\s*=' pkg/prompt/prompt.go   # 1 match: the constant definition
grep -nE 'ReasonPreviousPromptNotCompleted' pkg/queuescanner/scanner.go   # 1 match: the call site
grep -nE 'ReasonPreviousPromptNotCompleted' pkg/status/status_test.go   # 1 match: the parity test references the symbol
grep -cE '"previous prompt not completed"' pkg/queuescanner/scanner.go   # 0 matches: the spaced literal is gone
grep -cE 'reason=previous-prompt-not-completed' pkg/queuescanner/   # >= 1 match: the new test or scanner test asserts the token
```

The `git diff` for `/workspace/pkg/status/formatter.go` MUST be empty (Blocked-line format is byte-stable per spec 094 AC `blocked-format-unchanged`).

</verification>
