---
status: approved
spec: ["026"]
created: "2026-03-07T22:30:00Z"
queued: "2026-03-07T22:21:54Z"
---
<summary>
- When the verification gate is enabled, successfully executed prompts pause instead of completing immediately
- Dark-factory logs the prompt's verification instructions as a hint to the human on what to check
- The queue blocks while a prompt is pending verification — no new prompt starts until the human verifies
- When the gate is disabled (default), behavior is unchanged — prompts complete as before
- The status auto-correction logic leaves pending-verification prompts untouched
- Tests cover both gate-on and gate-off paths
</summary>

<objective>
Wire the verification gate into the processor's post-execution flow. When enabled, a successfully-executed prompt enters `pending_verification` instead of completing immediately. The queue blocks until the human verifies (via the CLI command added in the next prompt). No git operations are skipped yet — they are deferred, not deleted.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/processor/processor.go` — the full file. Focus on:
  - `processor` struct and `NewProcessor` constructor (fields and params)
  - `handlePostExecution` — where to intercept after validateCompletionReport succeeds
  - `processExistingQueued` — where queue blocking must be added
  - `autoSetQueuedStatus` — must NOT change `pending_verification` to approved
Read `pkg/prompt/prompt.go` — `PendingVerificationPromptStatus`, `MarkPendingVerification()`, `VerificationSection()` (added in spec-026 prompt 1).
Read `pkg/config/config.go` — `VerificationGate bool` field (added in spec-026 prompt 1).
Read `pkg/factory/factory.go` — `CreateProcessor` and `CreateRunner` to know where to thread the new param.
Read `pkg/processor/processor_test.go` if it exists — follow the existing test structure.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega, counterfeiter mocks.
</context>

<requirements>
1. Add `verificationGate bool` to the `processor` struct after `validationCommand string`.

2. Add `verificationGate bool` as the last parameter of `NewProcessor`. Assign it to the struct field.

3. In `autoSetQueuedStatus`, add `PendingVerificationPromptStatus` to the switch cases that are left unchanged:
   ```go
   case prompt.ApprovedPromptStatus,
       prompt.ExecutingPromptStatus,
       prompt.CompletedPromptStatus,
       prompt.FailedPromptStatus,
       prompt.PendingVerificationPromptStatus:
       // Already in a valid processing state — don't override
       return nil
   ```
   This prevents `pending_verification` from being reset to `approved` on the next scan cycle.

4. Add a `hasPendingVerification` helper method on `*processor`:
   ```go
   // hasPendingVerification returns true if any prompt in queueDir has pending_verification status.
   func (p *processor) hasPendingVerification(ctx context.Context) bool
   ```
   Implementation: scan `p.queueDir` for `.md` files, read each frontmatter, return true if any has `status == string(prompt.PendingVerificationPromptStatus)`. Return false on any read error (best-effort scan).

5. In `processExistingQueued`, add a pending-verification block check at the TOP of the function body, before the for loop:
   ```go
   // Block if any prompt is pending human verification
   if p.hasPendingVerification(ctx) {
       slog.Info("queue blocked: prompt pending verification")
       return nil
   }
   ```

6. Add `enterPendingVerification` method on `*processor`:
   ```go
   // enterPendingVerification transitions a prompt to pending_verification state and logs the verification hint.
   func (p *processor) enterPendingVerification(ctx context.Context, pf *prompt.PromptFile, promptPath string) error
   ```
   Implementation:
   - Call `pf.MarkPendingVerification()` then `pf.Save(ctx)`.
   - Extract the verification hint: `hint := pf.VerificationSection()`.
   - If hint is not empty, log it:
     ```go
     slog.Info("prompt pending verification — run the following checks, then: dark-factory prompt verify <file>",
         "file", filepath.Base(promptPath),
         "verification", hint,
     )
     ```
   - If hint is empty:
     ```go
     slog.Info("prompt pending verification",
         "file", filepath.Base(promptPath),
         "hint", "run: dark-factory prompt verify <file> when ready",
     )
     ```
   - Return nil.

7. In `handlePostExecution`, after the summary is stored (`pf.SetSummary` + `pf.Save`, ~line 428-433) and BEFORE the `isAutoReviewPR` block (~line 438), add the gate check:
   ```go
   // Verification gate: pause before git operations if enabled
   if p.verificationGate {
       return p.enterPendingVerification(ctx, pf, promptPath)
   }
   ```
   This must be placed AFTER `context.WithoutCancel` (line 436) and BEFORE the `isAutoReviewPR` check — the gate short-circuits all git operations.

8. In `pkg/factory/factory.go`:
   - Add `verificationGate bool` as the last parameter of `CreateProcessor`.
   - Pass it as the last argument to `processor.NewProcessor`.
   - In `CreateRunner`, pass `cfg.VerificationGate` as the new last argument to `CreateProcessor`.

9. Add or extend processor tests (file: `pkg/processor/processor_test.go`, package `processor_test`):
   - **Gate disabled (existing behavior)**: after successful execution, `handlePostExecution` proceeds to git ops (existing tests should still pass).
   - **Gate enabled, execution succeeds**: processor calls `MarkPendingVerification` and logs; does NOT call `moveToCompletedAndCommit`; returns nil.
   - **Gate enabled, execution fails**: processor marks failed as normal (gate only activates on success).
   - **hasPendingVerification returns true**: queue scan returns nil without processing.
   - **hasPendingVerification returns false**: queue scan proceeds normally.
   Use counterfeiter mocks for all dependencies — do NOT create manual mocks.

10. Remove any imports that become unused.
</requirements>

<constraints>
- Gate is ONLY applied when `verificationGate == true` — all existing behavior unchanged when false
- Do NOT skip the `validateCompletionReport` step — the gate applies only AFTER a successful report
- Do NOT change how failed prompts are handled — `MarkFailed` path is unaffected
- `autoSetQueuedStatus` must leave `pending_verification` unchanged (added in requirement 3)
- `ListQueued` already skips `pending_verification` (added in prompt 1) — do not duplicate that logic
- Do NOT add the `prompt verify` CLI command in this prompt — that is prompt 3
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Check coverage:
```bash
go test -cover ./pkg/processor/... ./pkg/factory/...
```
Coverage must be ≥80% for `pkg/processor`.
</verification>
