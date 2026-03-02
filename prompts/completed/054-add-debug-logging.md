---
status: completed
container: dark-factory-054-add-debug-logging
dark-factory-version: v0.12.2
created: "2026-03-02T22:32:16Z"
queued: "2026-03-02T22:32:16Z"
started: "2026-03-02T22:32:16Z"
completed: "2026-03-02T22:36:46Z"
---
<objective>
Add a -debug flag that enables verbose debug logging using Go's log/slog (stdlib).
Replace all existing log.Printf calls with slog.Info/slog.Debug as appropriate.
Debug-level messages show every file touch, move, docker operation, and internal state change.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `main.go` — entry point, parseArgs, command routing.
Read `pkg/config/config.go` — Config struct, Defaults, Validate.
Read `pkg/factory/factory.go` — wiring: CreateRunner, CreateProcessor, CreateWatcher.
Read `pkg/runner/runner.go` — startup, lock acquisition, orchestration.
Read `pkg/processor/processor.go` — prompt execution, setupPromptMetadata, processPrompt.
Read `pkg/watcher/watcher.go` — file event handling, NormalizeFilenames.
Read `pkg/prompt/prompt.go` — Load, Save, file operations.
Read `pkg/executor/executor.go` — Docker container execution.
Read `pkg/git/git.go` — commit, tag, push operations.
</context>

<requirements>

## 1. Add -debug flag to main.go

Parse a `-debug` boolean flag before command routing. Pass it through to where slog is configured.

```go
func run() error {
    debug, command, args := parseArgs()

    // Configure slog
    level := slog.LevelInfo
    if debug {
        level = slog.LevelDebug
    }
    slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

    // ... rest unchanged
}
```

Update `parseArgs` to extract `-debug` flag from os.Args before parsing the command.
The flag can appear anywhere: `dark-factory -debug`, `dark-factory -debug run`, `dark-factory run -debug`.

## 2. Replace all log.Printf with slog calls

Convert every `log.Printf("dark-factory: ...")` to structured slog calls.

**Info level** (always shown — current behavior):
- Startup/shutdown messages
- Prompt found/executing/completed/failed
- Docker container exit code
- File moved to completed
- Version tags and commits
- Lock acquired/released
- Watcher started
- PR created

**Debug level** (only with -debug):
- File Load/Save with path and body size
- Frontmatter state changes (status before → after)
- Queue scan results (count of queued/completed)
- Docker container full command/args
- File watcher raw events (CREATE, WRITE, RENAME)
- NormalizeFilenames each rename operation
- Completion report raw JSON parsing
- Git operations (commit message, tag name)
- Config loaded with all values
- Skipping prompt (with reason)
- Debounce timer events

Use structured key-value pairs, not format strings:
```go
// Before:
log.Printf("dark-factory: found queued prompt: %s", filepath.Base(pr.Path))

// After:
slog.Info("found queued prompt", "file", filepath.Base(pr.Path))

// Debug example:
slog.Debug("file loaded", "path", path, "bodySize", len(pf.Body), "hasStatus", pf.Frontmatter.Status != "")
```

## 3. Add debug logging to pkg/prompt/prompt.go

Add slog.Debug calls to:
- `Load()`: path, body size, frontmatter status
- `Save()`: path, body size, frontmatter status
- `SetStatus()`: path, old status, new status
- `MoveToCompleted()`: source, destination
- `NormalizeFilenames()`: each rename

## 4. Add debug logging to pkg/executor/executor.go

Add slog.Debug calls for:
- Docker command args (image, container name, mounts, env vars)
- Prompt content size being passed

## 5. Add debug logging to pkg/watcher/watcher.go

Add slog.Debug calls for:
- Every fsnotify event (operation, path)
- Debounce timer reset/fired
- NormalizeFilenames call with directory

## 6. Add debug logging to pkg/processor/processor.go

Add slog.Debug calls for:
- Queue scan: number of queued prompts found
- Prompt validation result
- AllPreviousCompleted check result
- Completion report raw JSON before parsing
- Git workflow decision (PR vs direct)

## 7. Add debug logging to pkg/git/git.go

Add slog.Debug calls for:
- Commit message
- Tag name
- Branch operations
- Push operations

## 8. Remove "dark-factory: " prefix

With slog, the structured format already identifies the source. Remove the "dark-factory: " prefix
from all messages. Keep messages lowercase for slog convention.

## 9. Update tests

Update any tests that assert on log output to work with slog.
If tests don't check log output, no changes needed.

</requirements>

<constraints>
- Use ONLY `log/slog` from stdlib — no external logging libraries
- Do NOT add slog as a dependency parameter to structs/constructors — use the global slog.Default()
- Do NOT change any business logic — only logging changes
- Do NOT modify function signatures (except parseArgs in main.go)
- Keep all existing Info-level messages — debug flag only ADDS more output
- Use Ginkgo v2 + Gomega for any new tests
</constraints>

<verification>
Run: `make test`
Run: `make precommit`
Run: `go build -o /tmp/df . && /tmp/df -debug status` (verify debug output appears)
</verification>

<success_criteria>
- `-debug` flag enables verbose output
- All log.Printf replaced with slog.Info or slog.Debug
- Debug output shows file operations, docker args, queue state
- Normal mode (no flag) shows same output as before (minus "dark-factory: " prefix)
- All tests pass
- `make precommit` passes
</success_criteria>
