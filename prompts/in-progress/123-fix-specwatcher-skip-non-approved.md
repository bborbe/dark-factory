---
status: queued
created: "2026-03-07T11:30:00Z"
queued: "2026-03-07T10:39:39Z"
---
<summary>
- SpecWatcher currently generates prompts for any file appearing in `specs/in-progress/` regardless of status
- Files with status `prompted` or `verifying` already have prompts — generation must be skipped
- Only `approved` status should trigger prompt generation
- Fixes spurious re-generation when spec files are migrated into `in-progress/` manually
- One new test: file with status `prompted` in in-progress dir does not trigger generation
</summary>

<objective>
SpecWatcher must check the spec file's frontmatter status before generating prompts. Only `status: approved` triggers generation. `prompted`, `verifying`, and any other status are skipped silently.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` and `go-testing.md` for patterns.
Read `pkg/specwatcher/specwatcher.go` — find the Create event handler that triggers prompt generation. This is where the status check must be added.
Read `pkg/spec/spec.go` — `StatusApproved`, `StatusPrompted`, `StatusVerifying` constants and `Load()`.
</context>

<requirements>
1. In the SpecWatcher Create event handler, after loading the spec file, check its status:
   ```go
   sf, err := spec.Load(ctx, path)
   if err != nil {
       slog.Warn("failed to load spec", "path", path, "error", err)
       return
   }
   if sf.Frontmatter.Status != string(spec.StatusApproved) {
       slog.Debug("skipping spec — not approved", "path", path, "status", sf.Frontmatter.Status)
       return
   }
   ```
   Only proceed to prompt generation if status is `approved`.

2. Add a test: a spec file with `status: prompted` placed in the in-progress dir does NOT trigger prompt generation.

<constraints>
- Do NOT change status values or spec frontmatter fields
- Do NOT commit — dark-factory handles git
- `make precommit` must pass
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
Run `go test ./pkg/specwatcher/... -v` — all tests pass.
</verification>
