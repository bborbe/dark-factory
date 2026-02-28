---
status: executing
---
# Save container output to log file

## Goal

Save Docker container stdout/stderr to `prompts/log/{prompt-name}.log` while still streaming to the terminal. Logs stay in `log/` permanently — they don't move when prompts move to `completed/`.

## Current Behavior

Container output streams to `os.Stdout`/`os.Stderr` only. Once gone, it's gone.

## Expected Behavior

```
prompts/
  log/
    005-test.log      ← container output (stays forever)
  completed/
    005-test.md       ← prompt moved here after success
```

- Output streams to terminal AND log file simultaneously (`io.MultiWriter`)
- Log file created before container starts
- Log file named after prompt file (without `.md`, with `.log`)
- `prompts/log/` directory created automatically if missing
- On failure, log file still exists for debugging
- `MoveToCompleted()` does NOT move log files

## Implementation

### Executor interface change

Add log file path parameter to `Execute`:

```go
type Executor interface {
    Execute(ctx context.Context, promptContent string, logFile string) error
}
```

### pkg/executor/executor.go

In `DockerExecutor.Execute()`:

1. Create parent directory for logFile (`os.MkdirAll`)
2. Open logFile for writing (create/truncate)
3. Use `io.MultiWriter(os.Stdout, logFile)` for cmd.Stdout
4. Use `io.MultiWriter(os.Stderr, logFile)` for cmd.Stderr
5. Close logFile after cmd.Run() completes

### pkg/factory/factory.go

In `processPrompt()`:

1. Derive log path from prompt path: `prompts/log/{basename}.log` (replace `.md` with `.log`)
2. Pass log path to `executor.Execute()`

### Tests

- Update mock executor in factory tests to accept new logFile parameter
- Test that log file is created and contains output (unit test with fake command)
- Test that log directory is created automatically

## Constraints

- Don't move or delete log files — they accumulate in `prompts/log/`
- Ensure log file is closed even on error (defer)
- Run `make precommit` before finishing
