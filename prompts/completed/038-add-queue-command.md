---
status: completed
container: dark-factory-038-add-queue-command
dark-factory-version: v0.9.1
---



# Add CLI and API endpoint to queue prompts

## Goal

Add `dark-factory queue` CLI command and `POST /queue` API endpoint to move prompts from inbox to queue. Replaces manual `mv prompts/x.md prompts/queue/`.

## CLI

```bash
dark-factory queue my-feature.md    # move specific file from inbox to queue
dark-factory queue                  # move all .md files from inbox to queue
```

Output:
```
queued: my-feature.md -> 038-my-feature.md
```

## REST API

```
POST /queue         {"file": "my-feature.md"}  # queue specific file
POST /queue/all                                 # queue all inbox files
GET  /inbox                                     # list inbox .md files
```

Response:
```json
{"queued": [{"old": "my-feature.md", "new": "038-my-feature.md"}]}
```

## Implementation

### 1. Add `queue` command to `parseArgs()` in main.go

```go
case "queue":
    queueCmd := factory.CreateQueueCommand(cfg)
    return queueCmd.Run(ctx, args)
```

### 2. Add `pkg/cmd/queue.go`

```go
type QueueCommand interface {
    Run(ctx context.Context, args []string) error
}
```

Logic:
- If args has a filename: move that file from `inboxDir` to `queueDir`
- If no args: move all `.md` files from `inboxDir` to `queueDir`
- Skip subdirectories, non-.md files
- Normalize filename (NNN- prefix) during move
- Set `status: queued` in frontmatter
- Print each queued file

### 3. Add `POST /queue` and `GET /inbox` handlers to server

- `POST /queue`: accepts JSON body with `file` field, queues that file
- `POST /queue/all`: queues all inbox files
- `GET /inbox`: lists .md files in inboxDir

### 4. Add to factory

```go
func CreateQueueCommand(cfg config.Config) cmd.QueueCommand
```

### 5. Tests

- Queue single file: moves from inbox to queue with NNN- prefix
- Queue all: moves all .md files, skips subdirs
- Queue nonexistent file: error
- Queue already-queued file: error (not in inbox)
- API POST /queue: returns queued files
- API GET /inbox: returns inbox listing

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push
- Follow `~/.claude-yolo/docs/go-patterns.md` for interface + constructor
- Coverage ≥80% for changed packages
