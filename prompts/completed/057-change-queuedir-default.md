---
status: completed
spec: [004-directory-separation]
summary: Changed default QueueDir from prompts to prompts/queue
container: dark-factory-057-change-queuedir-default
dark-factory-version: v0.13.2
created: "2026-03-03T16:21:50Z"
queued: "2026-03-03T16:21:50Z"
started: "2026-03-03T16:21:50Z"
completed: "2026-03-03T16:27:31Z"
---
# Change queueDir default to prompts/queue

## Goal

Change the default value of `QueueDir` in `pkg/config/config.go` from `"prompts"` to `"prompts/queue"`.

## Current Behavior

`Defaults()` returns `QueueDir: "prompts"`, which means without a config file the inbox and queue are the same directory.

## Expected Behavior

`Defaults()` returns `QueueDir: "prompts/queue"`, matching the preferred setup where inbox, queue, and completed are separate directories:

- `inboxDir`: `prompts`
- `queueDir`: `prompts/queue`
- `completedDir`: `prompts/completed`

## Implementation

### 1. Update `pkg/config/config.go`

Change the `Defaults()` function:

```go
func Defaults() Config {
    return Config{
        Workflow:       WorkflowDirect,
        InboxDir:       "prompts",
        QueueDir:       "prompts/queue",   // was "prompts"
        CompletedDir:   "prompts/completed",
        LogDir:         "prompts/log",
        ContainerImage: "docker.io/bborbe/claude-yolo:v0.0.8",
        DebounceMs:     500,
        ServerPort:     0,
    }
}
```

### 2. Update `pkg/config/config_test.go`

Fix the Defaults test expectation:

```go
Expect(cfg.QueueDir).To(Equal("prompts/queue"))
```

### 3. Update README.md

Remove or update the note:
> "Without a config file, `queueDir` defaults to `prompts` (not `prompts/queue`). Create a `.dark-factory.yaml` to use the inbox/queue separation."

Replace with:
> "Without a config file, `queueDir` defaults to `prompts/queue`. The inbox/queue/completed separation works out of the box."

Also update the Configuration table example comment to match.

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Coverage ≥80% for changed packages
