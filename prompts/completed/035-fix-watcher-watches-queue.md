---
status: completed
container: dark-factory-035-fix-watcher-watches-queue
dark-factory-version: v0.6.1
---



# Fix watcher to watch queueDir instead of inboxDir

## Goal

Watcher currently watches `inboxDir` and moves files to `queueDir`. This is wrong. The watcher should watch `queueDir` directly. `inboxDir` is a passive drop zone — no automation, no watcher.

## Current (wrong) behavior

1. Watcher watches `inboxDir` (`prompts/`)
2. Normalizes filenames in `inboxDir`
3. Moves normalized files from `inboxDir` to `queueDir`
4. Processor reads from `queueDir`

## Expected behavior

1. User drops file in `inboxDir` (`prompts/`) — nothing happens
2. User moves file to `queueDir` (`prompts/queue/`) when ready
3. Watcher watches `queueDir` — detects new file
4. Watcher normalizes filename (adds NNN- prefix, sets frontmatter)
5. Processor picks up normalized prompt from `queueDir`
6. On completion: moves to `completedDir`

## Implementation

### 1. Update watcher to watch `queueDir`

- Remove `inboxDir` from watcher — it doesn't need it
- Watch `queueDir` instead
- Normalize filenames in `queueDir`
- Remove `moveInboxToQueue()` — user moves files manually
- Constructor: `NewWatcher(queueDir string, promptManager, ready, debounce)`

### 2. Update factory

Pass `queueDir` (not `inboxDir`) to watcher constructor.

### 3. Update log messages

```
dark-factory: watcher started on prompts/queue
```

### 4. Tests

- Watcher watches queueDir, not inboxDir
- File dropped in inboxDir: nothing happens
- File moved to queueDir: watcher normalizes and signals processor
- moveInboxToQueue removed

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Coverage ≥80% for changed packages
