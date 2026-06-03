---
status: completed
spec: [092-daemon-blocked-queue-ux]
container: dark-factory-blocked-queue-exec-443-spec-092-status-blocked-line
dark-factory-version: v0.174.1-dirty
created: "2026-06-02T19:24:09Z"
queued: "2026-06-02T20:16:12Z"
started: "2026-06-02T21:42:36Z"
completed: "2026-06-02T21:56:25Z"
branch: dark-factory/daemon-blocked-queue-ux
---

<summary>

- `dark-factory status` shows a new `Blocked:` line under the queue block when the daemon's queue-advance guard refuses on the current candidate.
- The line names the gated prompt number, the refusal reason, and (when applicable) which predecessor is missing — no log-grepping required.
- The line appears only when there is an active blocker on a non-empty queue; the empty/idle/advanceable paths are byte-identical to today.
- The status checker reuses the per-spec guard helpers added by prompt 2 — no duplicated predecessor-scan logic.
- JSON output gains an optional `blocked` field; consumers that don't set the field see the same JSON shape they see today.
- Ginkgo coverage exercises the four spec acceptance criteria: blocker visible, absent on empty queue, absent on advanceable queue, status-log parity with the daemon's `prompt blocked` log line.

</summary>

<objective>
Surface the queue-advance guard's refusal reason in `dark-factory status` output. When the daemon refuses to advance, the operator sees exactly which prompt is gated and why — no log-grepping required. The blocker information flows from the per-spec guard added by prompt 2 into the status renderer.
</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` for the Interface → Constructor → Struct → Method convention.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` for `bborbe/errors` (no `fmt.Errorf`, always pass `ctx`).
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` for Ginkgo v2 / Gomega patterns and Counterfeiter mock reuse.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-json-error-handler-guide.md` — NOT applicable here (the output is human-readable text, not JSON).
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` for `Create*` factory wiring.

Files to read end-to-end before editing:
- `/workspace/specs/in-progress/092-daemon-blocked-queue-ux.md` — full spec, especially `Desired Behavior #1` and AC `blocker-visible`, `blocker-absent-empty`, `blocker-absent-advanceable`, `status-log-parity`
- `/workspace/pkg/status/status.go` lines 28-59 — `Status` struct (the new `Blocked` field is added here; byte-stability constraint applies to the non-blocked path)
- `/workspace/pkg/status/status.go` lines 134-200 — `GetStatus` flow (the populate functions it calls)
- `/workspace/pkg/status/status.go` lines 160-169 — the queued-prompts population block (the `Blocked` line goes right after this)
- `/workspace/pkg/status/formatter.go` lines 30-97 — `Format` method (the queue section is at lines 66-74; the new line is rendered immediately after)
- `/workspace/pkg/status/formatter.go` lines 135-156 — `formatCurrentPrompt` (mirror its column-alignment style — two-space indent for the section, four-space for items)
- `/workspace/pkg/status/prompt_manager.go` lines 14-22 — the local `PromptManager` interface (add the new `GetBlockedPrompt` method here)
- `/workspace/pkg/status/format_test.go` lines 22-200 — existing format tests; the new line must NOT break these. The byte-stability constraint is enforced here.
- `/workspace/pkg/status/status_test.go` — existing ginkgo tests; mirror the fixture pattern
- `/workspace/pkg/prompt/prompt.go` lines 916-929 — `*Manager.AllPreviousCompleted` / `FindMissingCompleted` (mirror the pattern for the new method)
- `/workspace/pkg/prompt/prompt.go` lines 280-308 — `NewPromptFile` and `Load` (the read pattern for the new `GetBlockedPrompt` implementation)
- `/workspace/pkg/factory/factory.go` lines 753-770 — `CreateStatusChecker` factory; the constructor signature does NOT change
- `/workspace/pkg/queuescanner/scanner.go` lines 195-219 — `logBlockedOnce` (the source of the `prompt blocked` log line — the new `GetBlockedPrompt` derives its values from the same per-spec guard)
- The file added by prompt 2: `pkg/queuescanner/scanner.go` lines 197-219 in the post-prompt-2 form (verify by reading the prompt 2 output before starting)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — the Ginkgo v2 / Gomega convention used across the repo

</context>

<requirements>

## 1. Preflight: verify prompt 2's helpers are present

Run this preflight grep BEFORE editing any file:

```
grep -n "AllPreviousInSpecCompleted\|FindMissingInSpecCompleted" /workspace/pkg/processor/*.go /workspace/pkg/prompt/*.go /workspace/pkg/queuescanner/*.go
```

Both `AllPreviousInSpecCompleted` and `FindMissingInSpecCompleted` MUST appear in the grep output. If either is missing, FAIL CLOSED: prompt 2 has not landed yet — stop and report. Do not proceed; do not re-implement these helpers locally.

## 2. Add `Blocked` field to `status.Status`

Edit `/workspace/pkg/status/status.go` lines 28-59 (the `Status` struct). Add the new field immediately AFTER `QueuedPrompts` and BEFORE `CommittingPrompts` to keep related queue-derived fields grouped:

```go
// Status represents the current daemon status.
type Status struct {
    ProjectDir          string   `json:"project_dir,omitempty"`
    Daemon              string   `json:"daemon"`
    DaemonPID           int      `json:"daemon_pid,omitempty"`
    CurrentPrompt       string   `json:"current_prompt,omitempty"`
    ExecutingSince      string   `json:"executing_since,omitempty"`
    Container           string   `json:"container,omitempty"`
    ContainerRunning    bool     `json:"container_running,omitempty"`
    GeneratingSpec      string   `json:"generating_spec,omitempty"`
    GeneratingContainer string   `json:"generating_container,omitempty"`
    QueueCount          int      `json:"queue_count"`
    QueuedPrompts       []string `json:"queued_prompts"`
    // Blocked describes the queue-advance guard's refusal to advance (spec 092).
    // Omitted from JSON and text output when no blocker is active.
    Blocked             *Blocked `json:"blocked,omitempty"`
    CommittingPrompts   []string `json:"committing_prompts,omitempty"`
    CommittingCount     int      `json:"committing_count,omitempty"`
    // ... rest unchanged ...
}

// Blocked describes a queue-advance guard refusal.
type Blocked struct {
    Number  int    `json:"number"`            // The prompt number being gated (3-digit, e.g. 227)
    Reason  string `json:"reason"`            // One of: previous-prompt-not-completed, previous-prompt-missing, prompt-frontmatter-parse-error, prompt-file-read-error, project-lock-timeout
    Missing int    `json:"missing,omitempty"` // The prompt number the guard expected to find (omitted when not applicable)
}
```

The `Blocked` field uses `*Blocked` (pointer) so the `omitempty` JSON tag works correctly — when no blocker is active, the field is `nil` and the JSON omits the key entirely. The `Missing` int uses `omitempty` so the `Blocked: NNN (reason=...)` format (without `missing=`) is used when the reason doesn't involve a predecessor.

Do NOT add a `Reason` field that uses an enum-typed constant — the `Blocked.Reason` is a plain string with the reason code. The valid reason codes are listed in the spec § Desired Behavior #1 and are not yet formalized as typed constants; keep them as strings.

## 3. Add `GetBlockedPrompt` to the local `PromptManager` interface

Edit `/workspace/pkg/status/prompt_manager.go` lines 14-22:

```go
//counterfeiter:generate -o ../../mocks/status-prompt-manager.go --fake-name StatusPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the status package uses.
type PromptManager interface {
    ListQueued(ctx context.Context) ([]prompt.Prompt, error)
    Title(ctx context.Context, path string) (string, error)
    ReadFrontmatter(ctx context.Context, path string) (*prompt.Frontmatter, error)
    HasExecuting(ctx context.Context) bool
    FindCommitting(ctx context.Context) ([]string, error)
    // GetBlockedPrompt scans the queue and returns the first queued prompt whose
    // per-spec predecessor is not completed. Returns (0, "", 0, false) if no
    // blocker is active.
    GetBlockedPrompt(ctx context.Context) (number int, reason string, missing int, ok bool)
}
```

## 4. Add `GetBlockedPrompt` to `*prompt.Manager` — call prompt 2's helpers, do NOT re-implement

Add to `/workspace/pkg/prompt/prompt.go` immediately AFTER the existing `FindPromptStatusInProgress` (line 929). This method is a thin scanner: walk queued candidates and call prompt 2's exported per-spec guard helpers (`AllPreviousInSpecCompleted` / `FindMissingInSpecCompleted`) to decide if each candidate is blocked. Do NOT inline a parallel predecessor-scan loop; reuse prompt 2's helpers verbatim.

```go
// GetBlockedPrompt scans queued prompts and returns the first one whose per-spec
// predecessor is not completed. Returns (0, "", 0, false) if no blocker is active
// or if no candidates exist. The decision is delegated to prompt 2's exported
// AllPreviousInSpecCompleted / FindMissingInSpecCompleted helpers — this method
// does not re-implement the guard.
func (pm *Manager) GetBlockedPrompt(ctx context.Context) (int, string, int, bool) {
    queued, err := pm.ListQueued(ctx)
    if err != nil {
        return 0, "", 0, false
    }
    for _, candidate := range queued {
        number := candidate.Number()
        if number < 0 {
            continue
        }
        pf, err := pm.Load(ctx, candidate.Path)
        if err != nil {
            return number, "prompt-file-read-error", 0, true
        }
        specs := pf.Specs()
        if len(specs) == 0 {
            if !pm.promptScanner.AllPreviousCompleted(ctx, number) {
                missing := pm.promptScanner.FindMissingCompleted(ctx, number)
                if len(missing) > 0 {
                    return number, "previous-prompt-not-completed", missing[0], true
                }
                return number, "previous-prompt-missing", 0, true
            }
            continue
        }
        specID := specs[0]
        if !pm.promptScanner.AllPreviousInSpecCompleted(ctx, number, specID) {
            missing, err := pm.promptScanner.FindMissingInSpecCompleted(ctx, number, specID)
            if err != nil {
                return number, "previous-prompt-missing", 0, true
            }
            if missing > 0 {
                return number, "previous-prompt-not-completed", missing, true
            }
            return number, "previous-prompt-missing", 0, true
        }
    }
    return 0, "", 0, false
}
```

The `pm.promptScanner.AllPreviousInSpecCompleted` and `pm.promptScanner.FindMissingInSpecCompleted` calls are the only predecessor-scan logic in this method. They are added by prompt 2; the Step 1 preflight verifies their presence.

## 5. Populate the `Blocked` field in `GetStatus`

Edit `/workspace/pkg/status/status.go` `GetStatus` method. The new field is populated AFTER the queued-prompts population (lines 166-169) and BEFORE the completed-prompts population (line 172). The new block:

```go
// Detect a blocked prompt (queue-advance guard refusal).
if status.QueueCount > 0 {
    if number, reason, missing, ok := s.promptMgr.GetBlockedPrompt(ctx); ok {
        status.Blocked = &Blocked{
            Number:  number,
            Reason:  reason,
            Missing: missing,
        }
    }
}
```

The `if status.QueueCount > 0` guard ensures the `Blocked` line is only populated when there is a queue — matches the spec's "When the queue is empty OR the daemon is actively executing a prompt OR the daemon is idle with no candidate at all (queue counter 0), the Blocked: line MUST NOT appear."

## 6. Render the `Blocked` line in the formatter

Edit `/workspace/pkg/status/formatter.go`. The `Blocked` line goes immediately AFTER the queue section (lines 66-74) and BEFORE the `Completed:` line (line 77). The exact format from spec § Desired Behavior #1:

```
Blocked:  NNN (reason=<reason>, missing=MMM)
```

or (when `Missing == 0`):

```
Blocked:  NNN (reason=<reason>)
```

The format has NO leading whitespace (the spec AC regex anchors on `^Blocked:`, requiring column-1 placement) and uses three-digit zero-padded numbers (matching the `000-format` used elsewhere — `fmt.Sprintf("%03d", n)`). The two-space gap after the colon matches the project's `Name:  value` style used by sibling lines `Current:`, `Queue:`, `Completed:`.

Add this block AFTER the `Queue:` section (after line 74) and BEFORE the `Completed:` section (line 77):

```go
// Blocked line (spec 092) — only rendered when a blocker is active.
if st.Blocked != nil {
    if st.Blocked.Missing > 0 {
        fmt.Fprintf(
            &b,
            "Blocked:  %03d (reason=%s, missing=%03d)\n",
            st.Blocked.Number,
            st.Blocked.Reason,
            st.Blocked.Missing,
        )
    } else {
        fmt.Fprintf(
            &b,
            "Blocked:  %03d (reason=%s)\n",
            st.Blocked.Number,
            st.Blocked.Reason,
        )
    }
}
```

The line has no leading whitespace (column 1). The two-space gap after `Blocked:` (`Blocked:  %03d`) is the chosen format and is consistent with the surrounding `Queue:`, `Current:`, `Completed:` lines, which all use a two-space `Name:  value` separator.

Verify the byte-stability for the non-blocked path: the new `if st.Blocked != nil` block is a no-op when the field is `nil`, so existing format tests that do NOT set `Blocked` produce identical output.

## 7. Counterfeiter mock regeneration

The `status.PromptManager` interface gained a new method. Run `cd /workspace && make generate-mocks` (or the equivalent target per the Makefile — verify with `grep -n 'generate-mocks\|counterfeiter:' /workspace/Makefile`). The regenerated `mocks/status-prompt-manager.go` will gain `GetBlockedPromptStub` / `GetBlockedPromptReturns` / `GetBlockedPromptArgsForCall` / `GetBlockedPromptCallCount` methods.

Verify the diff:
```
cd /workspace && git diff mocks/ | head -100
```

Expected: only `mocks/status-prompt-manager.go` has new methods. If other mock files are touched, revert those changes (you only modified the `status.PromptManager` interface).

## 8. Add ginkgo tests in `pkg/status/status_test.go` and `pkg/status/format_test.go`

### 8a. AC: `blocker-visible` (blocked path)

In `pkg/status/status_test.go`, add a `Describe("Blocked line", ...)` block. Use the existing test setup pattern (look for `BeforeEach` blocks that create a `status.NewChecker` and wire up mocks — match the shape).

```go
It("renders Blocked:  227 (reason=previous-prompt-not-completed, missing=226) when prompt 227 is gated by missing 226", func() {
    // Set up: mgr.ListQueued returns a single candidate 227 with spec: ["058"]
    // mgr.Load returns a PromptFile for 227 with Specs: ["058"]
    // mgr.GetBlockedPrompt returns (227, "previous-prompt-not-completed", 226, true)
    // Call checker.GetStatus(ctx)
    // Assert: status.Blocked != nil, status.Blocked.Number == 227, status.Blocked.Reason == "previous-prompt-not-completed", status.Blocked.Missing == 226
    // Assert: formatted output contains "Blocked:  227 (reason=previous-prompt-not-completed, missing=226)\n"
})
```

The `GetBlockedPrompt` mock setup: `mgr.GetBlockedPromptReturns(227, "previous-prompt-not-completed", 226, true)`.

### 8b. AC: `blocker-absent-empty` (empty queue)

```go
It("does NOT render Blocked line when queue is empty", func() {
    // mgr.ListQueued returns []
    // mgr.GetBlockedPrompt returns (0, "", 0, false)
    // Call checker.GetStatus(ctx)
    // Assert: status.Blocked == nil
    // Assert: formatted output does NOT contain "Blocked:"
})
```

### 8c. AC: `blocker-absent-advanceable` (advanceable queue)

```go
It("does NOT render Blocked line when the queue is advanceable", func() {
    // mgr.ListQueued returns a candidate 227 with spec: ["058"]
    // mgr.GetBlockedPrompt returns (0, "", 0, false) — no blocker
    // Call checker.GetStatus(ctx)
    // Assert: status.Blocked == nil
    // Assert: formatted output does NOT contain "Blocked:"
})
```

### 8d. AC: `status-log-parity`

```go
It("Blocker's three values match the prompt blocked log line", func() {
    // Capture slog output via a custom handler
    // Set up: scanner logs "prompt blocked file=227 reason=previous-prompt-not-completed spec=058 missing=226"
    //         status renders "Blocked:  227 (reason=previous-prompt-not-completed, missing=226)\n"
    // Assert: parse both, assert (227, previous-prompt-not-completed, 226) tuples are equal
})
```

This test is more involved — it requires both the scanner (or a stub of its log path) and the status checker to be set up. Verify the existing status_test.go fixture pattern before writing this; if a dual-setup is too complex, split the parity assertion into two simpler tests:
- 8d-i: "status Blocked carries the same values as a synthetic log line" (string-only)
- 8d-ii: "scanner's logBlockedOnce emits file/reason/spec/missing" (the log-line shape is verified by prompt 2's tests; status test only needs the input values)

### 8e. Existing format tests still pass

Do NOT modify any existing test in `pkg/status/format_test.go`. The new line is a no-op when `Blocked == nil`, so the byte-stability constraint is preserved. Run the existing format tests to confirm:

```bash
cd /workspace && go test -count=1 -mod=mod ./pkg/status/...
```

If any existing test fails, the new `if st.Blocked != nil` block is leaking into the non-blocked path — fix it before declaring done.

## 9. Run `cd /workspace && make precommit && make test`

All tests must pass. The new tests in `pkg/status/status_test.go` and `pkg/status/format_test.go` must compile (the `Blocked` struct and the new `GetBlockedPrompt` method must align with the mocks) and pass (the formatter must produce the exact string format the AC requires).

</requirements>

<constraints>

- Do NOT change the existing format of any line in `Format`. The byte-stability constraint is for the non-blocked path: a fixture with no blocker produces identical output to today's expectations. Verify by reading `pkg/status/format_test.go` lines 22-200 and confirming no existing assertion is invalidated.
- Do NOT add a new format field beyond `Blocked.Number`, `Blocked.Reason`, `Blocked.Missing`. The spec § Desired Behavior #1 enumerates the reason codes; the prompt's implementation hard-codes them as strings. A future typed-enum refactor is out of scope.
- Do NOT modify the public signature of `status.Checker`, `status.NewChecker`, `status.Formatter`, `status.NewFormatter`, or `factory.CreateStatusChecker`. The widening is internal — adding a new field on `Status` and a new method on the local `PromptManager` interface.
- Do NOT modify `pkg/prompt/prompt.go` outside of the new `GetBlockedPrompt` method. The `AllPreviousInSpecCompleted` / `FindMissingInSpecCompleted` methods are added by prompt 2 and used here as-is.
- Do NOT re-implement prompt 2's per-spec predecessor scan in this prompt. Call the exported helpers directly.
- Do NOT add a new CLI flag, config key, or metric. The spec § Non-goals forbid all three.
- Do NOT change the JSON encoding for the non-blocked path. The `omitempty` on `*Blocked` ensures the JSON omits the `blocked` key when no blocker is active — existing JSON parsers see identical output to today's. Verify by reading the new `Blocked` field tag and the `*Blocked` pointer type.
- Do NOT render the `Blocked` line for `Blocked.Missing == 0` differently from `Blocked.Missing > 0` other than the parenthetical — the line is rendered the same way except the `, missing=NNN` suffix is dropped when missing is zero.
- Do NOT commit — dark-factory handles git.
- File mode `0600` for any new test fixtures; `0750` for directories the test creates. Project convention.
- Branch is `dark-factory/daemon-blocked-queue-ux` (the spec's branch per frontmatter). Do not switch branches.
- Test files in `package status_test` (external) and `package status` (internal for format-only tests that need unexported access) — match the existing convention. Verify by reading the `package` declaration at the top of `pkg/status/status_test.go` and `pkg/status/format_test.go`.
- Counterfeiter mocks must be regenerated via `make generate-mocks` after the interface change. Do NOT hand-write the mock methods.

</constraints>

<verification>

Run from the repo root:

```
cd /workspace && make precommit && make test
```

All tests must pass. New test cases added in this prompt must compile and pass.

Spot checks:

```
cd /workspace
grep -n 'Blocked' pkg/status/status.go                                  # >= 4 lines: struct field, struct decl, populate call, comment
grep -n 'GetBlockedPrompt' pkg/status/prompt_manager.go                # 1 line (interface)
grep -n 'GetBlockedPrompt' pkg/prompt/prompt.go                        # 1 line (Manager method)
grep -n 'GetBlockedPrompt' mocks/status-prompt-manager.go              # >= 2 lines (counterfeiter-generated)
grep -n 'Blocked' pkg/status/formatter.go                              # >= 4 lines: import (none, stdlib), struct field read, format string
grep -nE 'Blocked:  ' pkg/status/formatter.go                          # >= 1 line
grep -cE 'It\(' pkg/status/status_test.go                              # >= existing + 4
grep -cE 'It\(' pkg/status/format_test.go                              # unchanged (no new tests; byte-stability verified by existing tests)
grep -cE 'Blocked:' pkg/status/status_test.go                          # >= 4
```

If any spot check fails or any test fails, fix the gap before declaring done.

</verification>
