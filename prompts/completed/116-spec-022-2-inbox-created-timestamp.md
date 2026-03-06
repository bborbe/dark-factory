---
status: completed
spec: ["022"]
summary: Added inboxDir to watcher struct and NewWatcher, implemented stampCreatedTimestamps method that sets created timestamp on inbox prompt files lacking one, updated factory and all tests
container: dark-factory-116-spec-022-2-inbox-created-timestamp
dark-factory-version: v0.19.0
created: "2026-03-06T18:35:00Z"
queued: "2026-03-06T18:45:26Z"
started: "2026-03-06T18:54:03Z"
completed: "2026-03-06T19:03:23Z"
---
<summary>
- When a prompt file appears in the inbox (`prompts/`), dark-factory stamps it with a `created` timestamp
- Only stamps if `created` is not already present — never overwrites
- Scans the inbox directory (not the queue) after each debounced file event
- Adds `inboxDir` field to the watcher struct and `NewWatcher` constructor
- Two new tests: file without `created` gets stamped, file with existing `created` is untouched
</summary>

<objective>
When a new prompt file appears in the inbox (`prompts/` directory), dark-factory immediately adds a `created` timestamp to its frontmatter if one is not already present. This ensures every inbox file has an exact creation record from the moment it is first detected by the watcher.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` and `go-testing.md` for patterns.
Read `pkg/watcher/watcher.go` — `watcher` struct has `queueDir` but NOT `inboxDir`; both the struct and `NewWatcher` constructor need `inboxDir` added.
Read `pkg/factory/factory.go` — `CreateWatcher` calls `NewWatcher`; update the call to pass `cfg.InboxDir`.
Read `pkg/config/config.go` — `InboxDir` field already exists in `Config`.
Read `pkg/prompt/prompt.go` — `Frontmatter.Created` field (string, `yaml:"created,omitempty"`); `Load()` / `Save()` on `PromptFile`.
</context>

<requirements>
1. In `pkg/watcher/watcher.go`:
   a. Add `inboxDir string` field to the `watcher` struct (after `queueDir`).
   b. Add `inboxDir string` parameter to `NewWatcher` and assign it.
   c. Add `stampCreatedTimestamps` method that scans `w.inboxDir` (not `w.queueDir`):
      ```go
      func (w *watcher) stampCreatedTimestamps(ctx context.Context) {
          entries, err := os.ReadDir(w.inboxDir)
          if err != nil {
              slog.Debug("inbox scan failed for created stamping", "error", err)
              return
          }
          for _, entry := range entries {
              if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
                  continue
              }
              path := filepath.Join(w.inboxDir, entry.Name())
              pf, err := prompt.Load(ctx, path)
              if err != nil {
                  continue
              }
              if pf.Frontmatter.Created != "" {
                  continue
              }
              pf.Frontmatter.Created = time.Now().UTC().Format(time.RFC3339)
              if err := pf.Save(ctx); err != nil {
                  slog.Debug("failed to stamp created timestamp", "path", path, "error", err)
                  continue
              }
              slog.Info("stamped created timestamp", "file", entry.Name())
          }
      }
      ```
   d. Call `w.stampCreatedTimestamps(ctx)` in `handleFileEvent` after `NormalizeFilenames` and before the ready signal.

2. In `pkg/factory/factory.go`, update `CreateWatcher` to pass `cfg.InboxDir` as the new second argument to `NewWatcher`.

3. Add or extend `pkg/watcher/watcher_test.go`:
   - A `.md` file in the inbox with no `Created` field gets stamped after `handleFileEvent`.
   - A `.md` file in the inbox with an existing `Created` field is not modified.
   Use separate temp dirs for inbox and queue in tests.
</requirements>

<constraints>
- Scan `inboxDir` (not `queueDir`) — inbox is `prompts/`, queue is `prompts/queue/`
- Do NOT overwrite an existing `created` field
- Do NOT commit — dark-factory handles git
- `make precommit` must pass
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
Run `go test ./pkg/watcher/... -v` — all tests pass.
</verification>

<constraints>
- Do NOT overwrite an existing `created` field — only set it when the field is empty
- Existing `created`, `queued`, `started`, `completed` prompt timestamp fields are unchanged
- Do NOT commit — dark-factory handles git
- `make precommit` must pass
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
Run `go test ./pkg/watcher/... -v` — all tests pass.
</verification>
