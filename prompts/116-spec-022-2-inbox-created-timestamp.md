---
spec: ["022"]
status: created
created: "2026-03-06T18:35:00Z"
---
<summary>
- When a prompt file appears in the inbox (`prompts/`), dark-factory stamps it with a `created` timestamp
- Only stamps if `created` is not already present — never overwrites
- Scans the inbox directory (not the queue) after each debounced file event
- Two new tests: file without `created` gets stamped, file with existing `created` is untouched
- NOTE: current prompt scans `queueDir` instead of `inboxDir` — this needs to be fixed before queuing
</summary>

<objective>
When a new prompt file appears in the inbox (`prompts/` directory), dark-factory immediately adds a `created` timestamp to its frontmatter if one is not already present. This ensures every inbox file has an exact creation record from the moment it is first detected by the watcher.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` and `go-testing.md` for patterns.
Read `pkg/watcher/watcher.go` — `handleFileEvent` is called after every debounced write/create event; it currently calls `NormalizeFilenames` then signals the processor.
Read `pkg/prompt/prompt.go` — `Frontmatter.Created` field (string, `yaml:"created,omitempty"`); `Load()` / `Save()` on `PromptFile`.
</context>

<requirements>
1. In `pkg/watcher/watcher.go`, add a new unexported method `stampCreatedTimestamps`:
   ```go
   // stampCreatedTimestamps scans the inbox directory and adds a created timestamp
   // to any .md file whose frontmatter has an empty Created field.
   func (w *watcher) stampCreatedTimestamps(ctx context.Context) {
       entries, err := os.ReadDir(w.queueDir)
       if err != nil {
           slog.Debug("inbox scan failed for created stamping", "error", err)
           return
       }
       for _, entry := range entries {
           if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
               continue
           }
           path := filepath.Join(w.queueDir, entry.Name())
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

2. Call `stampCreatedTimestamps` inside `handleFileEvent`, after `NormalizeFilenames` succeeds and before the ready signal:
   ```go
   func (w *watcher) handleFileEvent(ctx context.Context) {
       slog.Debug("normalizing filenames", "dir", w.queueDir)

       renames, err := w.promptManager.NormalizeFilenames(ctx, w.queueDir)
       if err != nil {
           slog.Info("failed to normalize filenames", "error", err)
           return
       }

       for _, rename := range renames {
           slog.Debug("renamed file",
               "from", filepath.Base(rename.OldPath),
               "to", filepath.Base(rename.NewPath))
       }

       w.stampCreatedTimestamps(ctx)

       select {
       case w.ready <- struct{}{}:
           slog.Debug("signaled processor ready")
       default:
           slog.Debug("processor already working, signal skipped")
       }
   }
   ```

3. Add or extend `pkg/watcher/watcher_test.go` to cover the new behaviour:
   - A `.md` file with no frontmatter (or empty `Created`) gets a `Created` timestamp after `handleFileEvent` is called.
   - A `.md` file that already has a `Created` timestamp is not modified (the timestamp is not overwritten).
   Use a temporary directory for the inbox in tests. Load the file after the call and inspect `Frontmatter.Created`.
</requirements>

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
