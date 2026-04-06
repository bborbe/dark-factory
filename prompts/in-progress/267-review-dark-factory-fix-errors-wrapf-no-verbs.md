---
status: approved
created: "2026-04-06T00:00:00Z"
queued: "2026-04-06T17:04:41Z"
---

<summary>
- Several files use errors.Wrapf with a plain message string and no format verbs
- When there are no format verbs, errors.Wrap should be used instead of errors.Wrapf
- The distinction matters because Wrapf is the formatted variant; using it without verbs is misleading
- Affected files are containerlock.go, reindex.go, and executor/checker.go
- The fix is a mechanical rename with no behavior change
</summary>

<objective>
Replace every `errors.Wrapf(ctx, err, "message")` call that has no format verbs with `errors.Wrap(ctx, err, "message")` in `pkg/containerlock/containerlock.go`, `pkg/reindex/reindex.go`, and `pkg/executor/checker.go`. The `Wrapf` variant is reserved for messages containing `%s`, `%v`, `%d`, or other format directives.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `pkg/containerlock/containerlock.go` — `errors.Wrapf` at ~lines 103 and 106
- `pkg/reindex/reindex.go` — `errors.Wrapf` at ~line 68
- `pkg/executor/checker.go` — `errors.Wrapf` at ~lines 45 and 103
</context>

<requirements>
1. In `pkg/containerlock/containerlock.go`:
   - Replace `errors.Wrapf(ctx, err, "unlock container lock file")` with `errors.Wrap(ctx, err, "unlock container lock file")`.
   - Replace `errors.Wrapf(ctx, err, "close container lock file")` with `errors.Wrap(ctx, err, "close container lock file")`.

2. In `pkg/reindex/reindex.go`:
   - Replace `errors.Wrapf(ctx, err, "collect entries")` with `errors.Wrap(ctx, err, "collect entries")`.

3. In `pkg/executor/checker.go`:
   - Replace `errors.Wrapf(ctx, err, "check container running")` with `errors.Wrap(ctx, err, "check container running")`.
   - Replace `errors.Wrapf(ctx, err, "docker ps for container count")` with `errors.Wrap(ctx, err, "docker ps for container count")`.

4. Verify no other `errors.Wrapf` calls without format verbs exist in `pkg/` (a quick grep for `errors.Wrapf(ctx, err, "` that do not contain `%` in the message is sufficient).
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap` for plain messages, `errors.Wrapf` only when the message contains format verbs
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
