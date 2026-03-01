---
status: completed
container: dark-factory-032-restructure-prompt-dirs
---


# Restructure prompt directories with configurable paths

## Goal

Replace single `promptsDir` config with three separate directory configs. Separates inbox (new ideas) from queue (ready to execute) from completed (done).

## Current Behavior

Single `promptsDir` (default: `prompts`) serves as both inbox and queue. Completed prompts go to `promptsDir/completed/`. No separation between "idea just dropped" and "ready to execute".

## Expected Behavior

Three configurable directories:

```yaml
# .dark-factory.yaml
inboxDir: prompts              # new prompts land here (/create-prompt target)
queueDir: prompts/queue        # watcher watches, processor executes from here
completedDir: prompts/completed # done prompts archived here
```

**Defaults** (backward compatible):
- `inboxDir: prompts`
- `queueDir: prompts` (same as inbox — current behavior)
- `completedDir: prompts/completed`

User opts into new structure by setting `queueDir: prompts/queue`.

## Implementation

### 1. Update existing `pkg/config/` — replace `promptsDir` with three dirs

Extend the existing `Config` struct from prompt 030. Replace `PromptsDir` field with three separate fields:

```go
type Config struct {
    Workflow       Workflow `yaml:"workflow"`
    InboxDir       string   `yaml:"inboxDir"`
    QueueDir       string   `yaml:"queueDir"`
    CompletedDir   string   `yaml:"completedDir"`
    ContainerImage string   `yaml:"containerImage"`
    DebounceMs     int      `yaml:"debounceMs"`
}
```

Update `Defaults()` and `Validate()` accordingly. The config is unmarshalled from `.dark-factory.yaml` into this struct.

Validation rules (in `Config.Validate()` using `github.com/bborbe/validation`):
- All three dirs must be non-empty strings
- `completedDir` must differ from `queueDir` (completed prompts can't live in queue)
- `completedDir` must differ from `inboxDir` (completed prompts can't live in inbox)
- If `inboxDir != queueDir`: both must exist and not overlap with completedDir
- `inboxDir == queueDir` is allowed (backward compat — single dir mode)
- Dark-factory must fail to start on invalid config (not silently use defaults)

### 2. Update watcher

Watcher watches `InboxDir` for new files. On file event:
- Normalize filenames
- If `InboxDir != QueueDir`: move normalized file from inbox to queue

### 3. Update processor

Processor scans `QueueDir` for `status: queued` prompts. On completion: moves to `CompletedDir`.

### 4. Update prompt manager

`ListQueued()` scans `QueueDir` (not inbox).
`MoveToCompleted()` moves to `CompletedDir`.
`AllPreviousCompleted()` checks `CompletedDir`.

### 5. Update lock file location

Lock file at project root `.dark-factory.lock` (not inside any prompt dir).

### 6. Create dirs on startup

Runner creates `InboxDir`, `QueueDir`, `CompletedDir` if they don't exist.

### 7. Tests

- Default config: `InboxDir == QueueDir == "prompts"` (backward compat)
- Custom config: separate inbox/queue/completed dirs
- Watcher watches inbox, processor reads queue
- File dropped in inbox → normalized → moved to queue (when dirs differ)
- File dropped in inbox → normalized in place (when dirs same)
- Completed prompt moves to completedDir
- Lock file at project root regardless of dir config
- Missing dirs created on startup

## Constraints

- Backward compatible — default dirs match current behavior exactly
- Run `make precommit` for validation only — do NOT commit, tag, or push
- Follow `~/.claude-yolo/docs/go-validation.md` for Config.Validate()
- Follow `~/.claude-yolo/docs/go-precommit.md` for linter limits
- Coverage ≥80% for changed packages
