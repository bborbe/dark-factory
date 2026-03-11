---
status: created
created: "2026-03-11T16:45:24Z"
---

<summary>
- All error returns in the processor and git packages now include contextual wrapping
- Error messages from clone operations identify which step failed (remove stale, clone, set remote)
- Error messages from collaborator fetching identify the GitHub CLI operation that failed
</summary>

<objective>
Wrap bare `return err` statements with `errors.Wrap(ctx, err, "...")` to preserve error context in the call chain. Bare error returns lose context about which operation failed.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read each file before editing. The project uses `github.com/bborbe/errors` for error wrapping with context.
</context>

<requirements>
1. In `pkg/processor/processor.go`, in the `ProcessQueue` method, find the bare `return err` after calling `p.autoSetQueuedStatus(ctx, &pr)` (~line 231). Wrap it:
   ```go
   // Old:
   return err
   // New:
   return errors.Wrap(ctx, err, "auto-set queued status")
   ```

2. In `pkg/git/cloner.go`, in the `Clone` method, wrap the three bare `return err` calls:
   ```go
   // Old (~line 39-40):
   if err := c.removeStale(ctx, destDir); err != nil {
       return err
   }
   // New:
   if err := c.removeStale(ctx, destDir); err != nil {
       return errors.Wrap(ctx, err, "remove stale clone")
   }

   // Old (~line 42):
   if err := c.gitClone(ctx, srcDir, destDir); err != nil {
       return err
   }
   // New:
   if err := c.gitClone(ctx, srcDir, destDir); err != nil {
       return errors.Wrap(ctx, err, "git clone")
   }

   // Old (~line 44-45):
   if err := c.setRealRemote(ctx, srcDir, destDir); err != nil {
       return err
   }
   // New:
   if err := c.setRealRemote(ctx, srcDir, destDir); err != nil {
       return errors.Wrap(ctx, err, "set real remote")
   }
   ```

3. In `pkg/git/collaborator_fetcher.go`, in `ghRepoNameFetcher.Fetch` (~line 103), wrap the raw `cmd.Output()` error. And in `ghCollaboratorLister.List` (~line 131), wrap its `cmd.Output()` error similarly. Both should use the pattern:
   ```go
   output, err := cmd.Output()
   if err != nil {
       return ..., errors.Wrap(ctx, err, "description of gh CLI call")
   }
   ```
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Use `github.com/bborbe/errors` — already imported in all three files.
- Do not change any logic — only add error wrapping.
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
