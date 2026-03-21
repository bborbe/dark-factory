---
status: created
spec: ["033"]
created: "2026-03-21T00:00:00Z"
---

<summary>
- A new `cancelled` prompt status is added to the lifecycle
- Users can run `dark-factory prompt cancel <id>` to cancel an approved or executing prompt
- The cancel command only changes frontmatter — it never touches Docker directly
- Cancelling a prompt in any other status (completed, failed, cancelled, etc.) returns an error
- `dark-factory prompt requeue <id>` now accepts cancelled prompts (same as failed)
- The daemon's auto-promote logic skips cancelled prompts (does not reset them to approved)
- `dark-factory prompt list` shows cancelled prompts with the correct status label
</summary>

<objective>
Add `CancelledPromptStatus` to the prompt lifecycle and implement the `dark-factory prompt cancel <id>` CLI command. The command only changes frontmatter status; the daemon (handled in the next prompt) is responsible for container cleanup.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/prompt/prompt.go` — the `PromptStatus` enum, `AvailablePromptStatuses`, `PromptFile`, and `Mark*` methods.
Read `pkg/cmd/approve.go` — pattern to follow for the new `CancelCommand`.
Read `pkg/cmd/requeue.go` — needs to accept `cancelled` prompts.
Read `pkg/cmd/prompt_finder.go` — `FindPromptFile` used by existing commands.
Read `pkg/processor/processor.go` — `autoSetQueuedStatus` must not promote `cancelled` prompts.
Read `pkg/factory/factory.go` — `CreateApproveCommand` / `CreateRequeueCommand` as wiring examples.
Read `main.go` — `runPromptCommand` switch to add `cancel` case.
</context>

<requirements>
1. In `pkg/prompt/prompt.go`:
   - Add `CancelledPromptStatus PromptStatus = "cancelled"` to the `const` block, after `PendingVerificationPromptStatus`.
   - Add `CancelledPromptStatus` to `AvailablePromptStatuses`.
   - Add `MarkCancelled()` method on `*PromptFile`:
     ```go
     // MarkCancelled sets status to cancelled.
     func (pf *PromptFile) MarkCancelled() {
         pf.Frontmatter.Status = string(CancelledPromptStatus)
     }
     ```

2. Create `pkg/cmd/cancel.go` following the same pattern as `pkg/cmd/approve.go`:
   - Interface:
     ```go
     //counterfeiter:generate -o ../../mocks/cancel-command.go --fake-name CancelCommand . CancelCommand
     type CancelCommand interface {
         Run(ctx context.Context, args []string) error
     }
     ```
   - Struct `cancelCommand` with fields: `queueDir string`, `currentDateTimeGetter libtime.CurrentDateTimeGetter`.
   - Constructor:
     ```go
     func NewCancelCommand(queueDir string, currentDateTimeGetter libtime.CurrentDateTimeGetter) CancelCommand
     ```
   - `Run` requires exactly one argument (the prompt ID); returns `errors.Errorf(ctx, "usage: dark-factory prompt cancel <id>")` otherwise.
   - Implementation:
     - Use `FindPromptFile(ctx, a.queueDir, id)` to locate the prompt file.
     - If not found, return `errors.Errorf(ctx, "prompt not found: %s", id)`.
     - Load the file with `prompt.Load`.
     - Check current status:
       - If `ApprovedPromptStatus` or `ExecutingPromptStatus` → call `pf.MarkCancelled()` and `pf.Save(ctx)`.
       - Otherwise → return `errors.Errorf(ctx, "cannot cancel prompt with status %q (only approved or executing prompts can be cancelled)", pf.Frontmatter.Status)`.
     - On success, print `fmt.Printf("cancelled: %s\n", filepath.Base(path))`.

3. In `pkg/processor/processor.go`, update `autoSetQueuedStatus`:
   - Add `prompt.CancelledPromptStatus` to the `switch` cases that return `nil` early (do not override), alongside `ApprovedPromptStatus`, `ExecutingPromptStatus`, etc.
   - This prevents the daemon from auto-promoting a cancelled prompt back to approved.

4. In `pkg/cmd/requeue.go`, update `requeueFile` to also accept `cancelled` prompts:
   - After loading the file, the existing code calls `pf.MarkApproved()` unconditionally for any single-file requeue. No status guard is needed for `requeueFile` — it already works for any status.
   - In `requeueFailed`, the check `pf.Frontmatter.Status == string(prompt.FailedPromptStatus)` currently restricts `--failed` to only failed prompts. This is intentional and should NOT change.
   - However, verify that `requeueFile` (single-file form) does NOT restrict by status — it should accept `failed` or `cancelled` (and it currently does, since it calls `MarkApproved()` unconditionally). If it currently has a status guard, remove it; if it doesn't, leave it as-is.

5. In `pkg/factory/factory.go`, add:
   ```go
   // CreateCancelCommand creates a CancelCommand.
   func CreateCancelCommand(cfg config.Config) cmd.CancelCommand {
       return cmd.NewCancelCommand(cfg.Prompts.InProgressDir, libtime.NewCurrentDateTime())
   }
   ```

6. In `main.go`, add `cancel` case to `runPromptCommand`:
   ```go
   case "cancel":
       return factory.CreateCancelCommand(cfg).Run(ctx, args)
   ```
   Also update `printHelp()` to include:
   ```
   prompt cancel <id>     Cancel an approved or executing prompt
   ```

7. Generate the counterfeiter mock for `CancelCommand` by running:
   ```
   go generate ./pkg/cmd/...
   ```
   (The `//counterfeiter:generate` directive added in step 2 will produce `mocks/cancel-command.go`.)

8. Add tests in `pkg/cmd/cancel_test.go` (external package `cmd_test`, Ginkgo/Gomega, `pkg/cmd` test suite):
   - Cancel an `approved` prompt → sets status to `cancelled`, prints success message.
   - Cancel an `executing` prompt → sets status to `cancelled`.
   - Cancel a `completed` prompt → returns error with current status.
   - Cancel a `failed` prompt → returns error.
   - Cancel a `cancelled` prompt → returns error.
   - Cancel with no args → returns usage error.
   - Cancel with unknown ID → returns "prompt not found" error.

9. Add or update tests in `pkg/processor/processor_internal_test.go` (or `processor_test.go`) to cover `autoSetQueuedStatus` with `CancelledPromptStatus`:
   - Input status `cancelled` → status unchanged (no auto-promote to approved).
</requirements>

<constraints>
- The cancel CLI command must NOT stop containers or interact with Docker directly — it only changes frontmatter status
- The daemon owns all Docker lifecycle operations (handled in the next prompt)
- Existing prompt status transitions must not change — `cancelled` is additive
- All existing tests must continue to pass
- The `autoSetQueuedStatus` logic must not auto-promote `cancelled` prompts back to `approved`
- Do NOT commit — dark-factory handles git
- Use `errors.Errorf(ctx, ...)` and `errors.Wrap(ctx, ...)` — never `fmt.Errorf`
- Use `//counterfeiter:generate` directive on the interface — never manual mocks
- External test package naming: `package cmd_test`
- Copyright header required on all new files
</constraints>

<verification>
Run `make precommit` — must pass.
Run `go test ./pkg/cmd/... -run TestCancel` — confirm cancel tests pass.
</verification>
