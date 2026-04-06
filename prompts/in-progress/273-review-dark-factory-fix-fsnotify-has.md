---
status: approved
created: "2026-04-06T00:00:00Z"
queued: "2026-04-06T17:05:26Z"
---

<summary>
- Three files use the old bit-mask operator form to check fsnotify event types
- fsnotify v1.6.0 introduced the event.Has() method as the idiomatic replacement
- Using Has() makes intent clearer and is the documented best practice for v1.6+
- The fix is a mechanical substitution with no behavior change
- Existing tests continue to pass without modification
</summary>

<objective>
Replace old-style bit-mask fsnotify event checks (`event.Op & fsnotify.Write == 0`) with the `event.Has()` method (`!event.Has(fsnotify.Write)`) in `pkg/watcher/watcher.go`, `pkg/specwatcher/watcher.go`, and `pkg/processor/processor.go`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `pkg/watcher/watcher.go` — ~lines 117–118
- `pkg/specwatcher/watcher.go` — ~line 109
- `pkg/processor/processor.go` — `watchForCancellation` method (~line 997)
</context>

<requirements>
1. In each of the three files, find every expression of the form:
   ```go
   event.Op & fsnotify.Write == 0
   event.Op & fsnotify.Create == 0
   event.Op & fsnotify.Chmod == 0
   ```
   and replace with:
   ```go
   !event.Has(fsnotify.Write)
   !event.Has(fsnotify.Create)
   !event.Has(fsnotify.Chmod)
   ```

2. For the negated forms (`event.Op & fsnotify.Write != 0`), replace with `event.Has(fsnotify.Write)`.

3. Read each file fully to ensure all bit-mask event checks are found and replaced, not just those at the known line numbers.

4. Do not change any other logic, function signatures, or imports.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- The behavior of the watcher logic must not change — only the expression form changes
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
