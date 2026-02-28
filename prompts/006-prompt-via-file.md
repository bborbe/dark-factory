---
status: failed
---
# Pass prompt content via file instead of env var

## Goal

Fix broken prompt passing. Currently the full prompt content is passed via `-e YOLO_PROMPT=...` which breaks when the content contains special characters (backticks, quotes, newlines, `---`). Docker/shell interprets them as arguments.

## Current Behavior

```go
cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
    "-e", "YOLO_PROMPT="+promptContent,
    ...
)
```

Fails with: `error: unknown option '---'` when prompt contains YAML frontmatter or markdown code blocks.

## Expected Behavior

- Prompt content written to a temp file on the host
- Temp file mounted into the container
- Container reads prompt from the mounted file
- Temp file cleaned up after container exits
- Works with any content (markdown, YAML, code blocks, special characters)

## Implementation

### pkg/executor/executor.go

In `DockerExecutor.Execute()`:

1. Create a temp file: `os.CreateTemp("", "dark-factory-prompt-*.md")`
2. Write `promptContent` to the temp file
3. Close the temp file (so Docker can read it)
4. Add volume mount: `-v`, tempFile.Name()+":/tmp/prompt.md:ro"
5. Replace `-e YOLO_PROMPT=...` with `-e YOLO_PROMPT_FILE=/tmp/prompt.md`
6. `defer os.Remove(tempFile.Name())` to clean up

### Docker image / CLAUDE.md

The claude-yolo container entrypoint needs to read from `YOLO_PROMPT_FILE` if `YOLO_PROMPT` is empty. Check how the container currently consumes `YOLO_PROMPT`:

- If it uses `$YOLO_PROMPT` in a shell script → change to `cat $YOLO_PROMPT_FILE`
- If the entrypoint is configurable → just pass the file path

**Alternative (simpler):** Keep passing content via env var but use `cmd.Env` instead of `-e` flag to avoid shell escaping:

```go
cmd.Env = append(os.Environ(), "YOLO_PROMPT="+promptContent)
```

This avoids the shell parsing issue entirely since Go's `exec.Command` passes args directly to the process (no shell). But the `-e` flag still goes through Docker's argument parser.

**Recommended approach:** Use `-v` mount with temp file. Most robust.

### Tests

- Test with prompt containing backticks, quotes, `---`, code blocks
- Verify temp file is cleaned up after execution

## Constraints

- Temp file must be readable by the Docker container (permissions)
- Clean up temp file even on error (use defer)
- Run `make precommit` before finishing
