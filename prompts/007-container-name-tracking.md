---
status: queued
---
# Track Docker container name in prompt frontmatter

## Goal

Give each container a predictable name based on the prompt filename. Store the container name in the prompt's YAML frontmatter so we can identify which container ran which prompt.

## Current Behavior

- `docker run --rm` with no `--name` flag → random container name
- No record of which container executed a prompt

## Expected Behavior

```yaml
---
status: executing
container: dark-factory-005-log-output
---
```

- Container name derived from prompt basename: `dark-factory-{basename}` (without `.md`)
- Container name written to frontmatter before `docker run` starts
- Container name stays in frontmatter after completion (in `completed/` file)

## Implementation

### pkg/executor/executor.go

1. Add `containerName string` parameter to `Execute` interface
2. Add `--name`, containerName to the `docker run` args
3. Keep `--rm` so container is cleaned up after exit

### pkg/factory/factory.go

In `processPrompt()`:

1. Derive container name: `dark-factory-` + `strings.TrimSuffix(filepath.Base(p.Path), ".md")`
2. Write container name to frontmatter via `prompt.SetField()` before calling executor
3. Pass container name to `executor.Execute()`

### pkg/prompt/prompt.go

1. Add `Container` field to `Frontmatter` struct: `Container string \`yaml:"container"\``
2. Add `SetField(ctx, path, key, value)` function or extend `SetStatus` to handle arbitrary fields
   - Alternative: just add `SetContainer(ctx, path, container)` if simpler

### Update counterfeiter mock

After changing the Executor interface, run `make generate` to regenerate `mocks/executor.go`.

### Update tests

- Update factory tests to verify container name is passed to executor
- Use counterfeiter's `ExecuteArgsForCall(0)` to check the container name arg

## Constraints

- Container name must be valid for Docker (alphanumeric, hyphens, underscores)
- Run `make precommit` before finishing
