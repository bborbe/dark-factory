---
status: completed
spec: [026-prompt-verification-gate]
summary: Added PendingVerificationPromptStatus constant, MarkPendingVerification() and VerificationSection() methods on PromptFile, updated ListQueued to skip pending_verification, added VerificationGate config field with loader support, and tests for all new behavior.
container: dark-factory-135-spec-026-status-and-config
dark-factory-version: v0.26.0
created: "2026-03-07T22:30:00Z"
queued: "2026-03-07T22:21:54Z"
started: "2026-03-07T22:21:56Z"
completed: "2026-03-07T22:29:43Z"
---
<summary>
- Prompts can now represent a "pending verification" state — a human gate between execution and completion
- The queue recognizes pending-verification prompts and does not pick them up for execution
- A prompt's verification instructions can be extracted programmatically (for logging to the human)
- Projects can opt in to the verification gate via config (disabled by default — no behavior change)
- All new capabilities are covered by tests
</summary>

<objective>
Lay the foundation for the prompt verification gate: add the `pending_verification` status to the prompt status model and add a `verificationGate` config field. This enables the subsequent processor and CLI prompts to build on a complete data model without touching any control flow yet.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/prompt/prompt.go` — PromptStatus type, constants, AvailablePromptStatuses, PromptFile methods (MarkCompleted, MarkFailed, MarkApproved), and ListQueued skip logic.
Read `pkg/config/config.go` — Config struct, Defaults(), PromptsConfig.
Read `/home/node/.claude/docs/go-patterns.md` — interface/constructor/struct pattern, error wrapping.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega conventions, external test packages, counterfeiter.
Read `/home/node/.claude/docs/go-enum-pattern.md` — enum constant naming convention.
</context>

<requirements>
1. In `pkg/prompt/prompt.go`, add the new status constant following the existing pattern:
   ```go
   PendingVerificationPromptStatus PromptStatus = "pending_verification"
   ```
   Add it to `AvailablePromptStatuses` after `InReviewPromptStatus`.

2. In `ListQueued` (pkg/prompt/prompt.go line ~535), add `PendingVerificationPromptStatus` to the skip list alongside `FailedPromptStatus`. Pending-verification prompts should NOT be returned as candidates for execution:
   ```go
   fm.Status == string(PendingVerificationPromptStatus) ||
   ```

3. Add `MarkPendingVerification()` method on `*PromptFile` following the pattern of `MarkFailed()`:
   ```go
   // MarkPendingVerification sets status to pending_verification.
   func (pf *PromptFile) MarkPendingVerification() {
       pf.Frontmatter.Status = string(PendingVerificationPromptStatus)
   }
   ```
   No timestamp field is needed — pending_verification is a transient human-gate state.

4. Add `VerificationSection()` method on `*PromptFile` that extracts the content of the `<verification>` XML tag from `pf.Body`:
   ```go
   // VerificationSection extracts the content of the <verification> tag from the prompt body.
   // Returns an empty string if no <verification> tag is found.
   func (pf *PromptFile) VerificationSection() string
   ```
   Implementation: scan `pf.Body` as a string, find `<verification>` and `</verification>` delimiters (case-sensitive, exact match), return the content between them trimmed of leading/trailing whitespace. If either delimiter is absent, return "".

5. In `pkg/config/config.go`, add `VerificationGate bool` field to the `Config` struct:
   ```go
   VerificationGate bool `yaml:"verificationGate"`
   ```
   Place it after `AutoRelease bool`. Default is `false` (Go zero value — no change to `Defaults()` needed).

6. Add tests in `pkg/prompt/prompt_test.go` (external package `package prompt_test`, Ginkgo):
   - `PendingVerificationPromptStatus` is in `AvailablePromptStatuses`.
   - `AvailablePromptStatuses.Contains(PendingVerificationPromptStatus)` returns true.
   - `MarkPendingVerification()` sets `Frontmatter.Status` to `"pending_verification"`.
   - `VerificationSection()` returns the trimmed inner content when `<verification>…</verification>` is present.
   - `VerificationSection()` returns `""` when no `<verification>` tag is in the body.
   - `VerificationSection()` returns `""` when only the opening tag is present (no closing tag).
   - `ListQueued` skips a file with `status: pending_verification`.

7. Add tests in `pkg/config/config_test.go` (external package, Ginkgo) if the file already exists, or in a new `pkg/config/config_test.go`:
   - `Defaults()` returns a Config with `VerificationGate == false`.
   - Marshaling/unmarshaling a Config with `verificationGate: true` round-trips correctly.
   Check whether `pkg/config/config_test.go` already exists before writing new tests — if it does, append to the existing Ginkgo Describe block.

8. Run `make generate` if any counterfeiter annotations changed (they should not for this prompt — no interfaces changed).

9. Remove any imports that become unused.
</requirements>

<constraints>
- Default behavior unchanged — `VerificationGate` is false by default; no existing test should break
- Do NOT add `PendingVerificationPromptStatus` to the `ResetFailed` or `ResetExecuting` logic — those are for failed/executing states only
- Do NOT change any processor, factory, or main.go in this prompt — data model only
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Spot-check coverage:
```bash
go test -cover ./pkg/prompt/... ./pkg/config/...
```
Coverage must be ≥80% for both packages.
</verification>
