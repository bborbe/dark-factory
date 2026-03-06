---
status: created
spec: ["020"]
created: "2026-03-06T17:15:00Z"
---

<objective>
Fix SpecWatcher to scan for already-approved specs on startup, not only on fsnotify events.
</objective>

<context>
The processor does this in processExistingQueued() (pkg/processor/processor.go:115) — scans queue dir on startup before the watcher fires. SpecWatcher needs the same pattern: on Watch() entry, scan all .md files in specsDir and call generator.Generate() for any with status approved.

Read pkg/specwatcher/watcher.go — add startup scan there.
Read pkg/processor/processor.go — see processExistingQueued() for the pattern to follow.
Read pkg/spec/spec.go — for Load() and StatusApproved.
</context>

<requirements>
1. At the start of Watch(), before entering the fsnotify event loop, scan all .md files in specsDir.
2. For each file with status == approved, call generator.Generate(ctx, path). Log errors but do not abort.
3. Add test: approved spec present on startup → generator called before any fsnotify events.
4. Existing tests must still pass.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- make precommit must pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
