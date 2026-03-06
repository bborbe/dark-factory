---
status: completed
spec: ["021"]
summary: Added `dark-factory spec verify <file>` command that transitions a spec from `verifying` to `completed`, including shared findSpecFile helper, counterfeiter mock, factory wiring, and main.go routing
container: dark-factory-113-spec-021-2-verify-command
dark-factory-version: v0.18.2
created: "2026-03-06T18:00:00Z"
queued: "2026-03-06T16:59:40Z"
started: "2026-03-06T16:59:40Z"
completed: "2026-03-06T17:08:34Z"
---
<objective>
Add `dark-factory spec verify <file>` command that transitions a spec from `verifying` to `completed`. This is the human's explicit gate to close a spec after evaluating its acceptance criteria.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/cmd/spec_approve.go — SpecVerifyCommand follows this pattern exactly (same findSpec helper, same single-file structure).
Read pkg/spec/spec.go — StatusVerifying and StatusCompleted constants, MarkCompleted() method (added/confirmed in spec-021-1).
Read pkg/factory/factory.go — add CreateSpecVerifyCommand following the CreateSpecApproveCommand pattern.
Read main.go — wire spec verify into runSpecCommand switch and printHelp.
Read pkg/cmd/spec_approve_test.go — follow this test structure for spec_verify_test.go.
</context>

<requirements>
1. Create `pkg/cmd/spec_verify.go`:
   - Interface with counterfeiter annotation:
     ```go
     //counterfeiter:generate -o ../../mocks/spec-verify-command.go --fake-name SpecVerifyCommand . SpecVerifyCommand
     type SpecVerifyCommand interface {
         Run(ctx context.Context, args []string) error
     }
     ```
   - Private struct:
     ```go
     type specVerifyCommand struct {
         specsDir string
     }
     ```
   - Constructor:
     ```go
     func NewSpecVerifyCommand(specsDir string) SpecVerifyCommand {
         return &specVerifyCommand{specsDir: specsDir}
     }
     ```
   - `Run` method:
     - Requires exactly one arg (the spec identifier); return error if missing.
     - Calls `findSpec(ctx, id)` — copy the exact `findSpec` method from `specApproveCommand` (or share via the struct; since it's an exact duplicate, copy it into `specVerifyCommand`).
     - Loads the spec with `spec.Load(ctx, path)`.
     - If `sf.Frontmatter.Status != string(spec.StatusVerifying)`, return:
       ```go
       errors.Errorf(ctx, "spec is not in verifying state (current: %s)", sf.Frontmatter.Status)
       ```
     - Calls `sf.MarkCompleted()` then `sf.Save(ctx)`.
     - Prints: `fmt.Printf("verified: %s\n", filepath.Base(path))`

2. Create `pkg/cmd/spec_verify_test.go`:
   - Use `package cmd_test` with Ginkgo/Gomega (follow spec_approve_test.go for suite setup).
   - Test cases:
     - No args → error "spec identifier required" (or similar — match what the implementation returns)
     - Spec in `verifying` state → transitions to `completed`, prints `verified: <filename>`
     - Spec in `draft` state → error containing "not in verifying state"
     - Spec in `approved` state → error containing "not in verifying state"
     - Spec in `completed` state → error containing "not in verifying state"
     - Spec file not found → error (propagated from findSpec)

3. Run `make generate` to create the counterfeiter mock at `mocks/spec-verify-command.go`.

4. In `pkg/factory/factory.go`, add:
   ```go
   // CreateSpecVerifyCommand creates a SpecVerifyCommand.
   func CreateSpecVerifyCommand(cfg config.Config) cmd.SpecVerifyCommand {
       return cmd.NewSpecVerifyCommand(cfg.SpecDir)
   }
   ```
   Place it after `CreateSpecApproveCommand`.

5. In `main.go`, update `runSpecCommand`:
   - Add `"verify"` case:
     ```go
     case "verify":
         return factory.CreateSpecVerifyCommand(cfg).Run(ctx, args)
     ```

6. In `main.go`, update `printHelp` to add the new command in the spec section:
   ```
   "  spec verify <id>       Mark a spec as verified (verifying → completed)\n"+
   ```
   Place it after `"  spec approve <id>      Approve a spec\n"`.
</requirements>

<constraints>
- Do NOT modify `CheckAndComplete` or the verifying transition logic — that was done in spec-021-1
- `spec verify` must return a clear error when spec status is not `verifying` — never silently succeed
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Smoke test (requires a spec file on disk):
```bash
# Create a test spec in verifying state
echo -e "---\nstatus: verifying\n---\n# Test" > specs/test-verify.md
dark-factory spec verify test-verify.md
# Should print: verified: test-verify.md
grep 'status: completed' specs/test-verify.md

# Test wrong state
echo -e "---\nstatus: draft\n---\n# Test" > specs/test-draft.md
dark-factory spec verify test-draft.md
# Should print error: spec is not in verifying state (current: draft)

# Cleanup
rm specs/test-verify.md specs/test-draft.md
```
</verification>
