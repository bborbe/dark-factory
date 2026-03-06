---
status: queued
---

Add three new subcommands: `list`, `requeue`, and `retry`.

## Context

Read the following before making changes:
- `pkg/cmd/queue.go` — style reference for a cmd implementation
- `pkg/cmd/status.go` — style reference
- `pkg/prompt/prompt.go` — status constants and MarkQueued()
- `pkg/factory/factory.go` — where commands are wired up
- `main.go` — where commands are dispatched

## Commands

### `dark-factory list`

List all prompts across inbox, queue, and completed dirs with their status.

Output format (human-readable table):
```
LOCATION   STATUS     FILE
inbox      created    fix-something.md
queue      queued     078-configurable-model.md
queue      failed     080-workflow-test-coverage.md
completed  completed  077-wire-auto-merge.md
completed  completed  076-auto-merge-config.md
```

Flags:
- `--queue` — show queue only
- `--failed` — show failed only
- `--json` — JSON output

Implementation: scan `inboxDir`, `queueDir`, `completedDir` — load frontmatter status from each `.md` file — print table sorted by location order (inbox → queue → completed), then by filename.

### `dark-factory requeue [file]`

Reset a prompt's status to `queued` so it will be picked up again.

```bash
dark-factory requeue 080-workflow-test-coverage.md   # specific file in queue dir
dark-factory requeue --failed                         # requeue all failed in queue dir
```

Implementation:
- With filename: find file in `queueDir`, load it, call `MarkQueued()`, save
- With `--failed`: scan `queueDir`, requeue all with `status: failed`
- Print: `requeued: <filename>`

### `dark-factory retry`

Shorthand for `requeue --failed`. No arguments.

Implementation: delegates to requeue logic with `--failed` flag.

## Wiring

### `pkg/cmd/list.go`

New file. Interface + implementation for `list` command. Follow the same pattern as `queue.go`:
```go
//counterfeiter:generate -o ../../mocks/list-command.go --fake-name ListCommand . ListCommand
type ListCommand interface {
    Run(ctx context.Context, args []string) error
}
```

### `pkg/cmd/requeue.go`

New file. Interface + implementation for `requeue` command:
```go
//counterfeiter:generate -o ../../mocks/requeue-command.go --fake-name RequeueCommand . RequeueCommand
type RequeueCommand interface {
    Run(ctx context.Context, args []string) error
}
```

`retry` reuses `RequeueCommand.Run(ctx, []string{"--failed"})` — no separate interface needed.

### `pkg/factory/factory.go`

Add:
```go
func CreateListCommand(cfg config.Config) cmd.ListCommand
func CreateRequeueCommand(cfg config.Config) cmd.RequeueCommand
```

### `main.go`

Add cases to the dispatch switch:
```go
case "list":
    return factory.CreateListCommand(cfg).Run(ctx, args)
case "requeue":
    return factory.CreateRequeueCommand(cfg).Run(ctx, args)
case "retry":
    return factory.CreateRequeueCommand(cfg).Run(ctx, []string{"--failed"})
```

Also add to `parseArgs()` known commands: `"list"`, `"requeue"`, `"retry"`.

Update `--help` output to include the new commands.

### Mocks

Run `go generate ./...` to regenerate mocks after adding counterfeiter directives.

## Tests

Add `pkg/cmd/list_test.go` and `pkg/cmd/requeue_test.go` using Ginkgo v2. Follow style of `pkg/cmd/queue_test.go`.

Key test cases:
- `list`: shows prompts from all dirs
- `list --failed`: shows only failed
- `requeue <file>`: resets status to queued
- `requeue --failed`: resets all failed
- `retry`: same as requeue --failed

## Verification

Run `make precommit` — must pass.
