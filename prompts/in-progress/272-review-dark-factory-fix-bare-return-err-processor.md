---
status: approved
created: "2026-04-06T00:00:00Z"
queued: "2026-04-06T17:05:25Z"
---

<summary>
- The processor package has many error return sites where ctx is available but errors are returned unwrapped
- Unwrapped errors lose stack trace and context values the project-wide error library attaches
- Each bare return is at a call boundary where the function name provides useful context to attach
- The fix is mechanical: wrap each bare return with context and a descriptive message using the project error library
- No logic changes are needed — only error wrapping is added
</summary>

<objective>
Wrap all bare `return err` statements in `pkg/processor/processor.go` where `ctx context.Context` is available in scope, using `errors.Wrap(ctx, err, "message")` from `github.com/bborbe/errors`. This ensures consistent stack trace and context propagation for all processor error paths.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/dod.md` for Definition of Done.

Files to read before making changes (read ALL first):
- `pkg/processor/processor.go` — the file to fix; it is large (~1674 lines); read it completely to understand function boundaries before making changes
</context>

<requirements>
1. In `pkg/processor/processor.go`, find every `return err` where:
   - `ctx context.Context` is in scope (as a function parameter or outer variable), AND
   - the error is being propagated from a sub-call (i.e., it is not already wrapped at this site)

2. Wrap each such bare `return err` with:
   ```go
   return errors.Wrap(ctx, err, "<short description of what failed>")
   ```
   where `<short description>` is a lowercase phrase naming the sub-call that failed, e.g.:
   - After `p.syncWithRemote(ctx)` → `"sync with remote"`
   - After `preparePromptForExecution(...)` → `"prepare prompt for execution"`
   - After `p.setupWorkflow(...)` → `"setup workflow"`
   - After `p.prepareContainerSlot(...)` → `"prepare container slot"`
   - After `validateCompletionReport(...)` → `"validate completion report"`
   - After `p.moveToCompletedAndCommit(...)` → `"move to completed and commit"`
   - After `p.handleDirectWorkflow(...)` → `"handle direct workflow"`
   - After `p.handleBranchCompletion(...)` → `"handle branch completion"`
   - After `p.findOrCreatePR(...)` → `"find or create PR"`

3. Ensure `"github.com/bborbe/errors"` is imported (it should already be present).

4. Do NOT wrap errors that already use `errors.Wrap` or `errors.Wrapf` at the same site.

5. Do NOT wrap errors inside closures that have no `ctx` in scope.

6. Do NOT change any function signatures, logic, or test files.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap(ctx, err, "message")` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err` where ctx is available
- Only modify `pkg/processor/processor.go`
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
