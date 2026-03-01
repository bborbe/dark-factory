---
status: executing
container: dark-factory-039-disable-server-when-no-port
dark-factory-version: v0.9.1
---


# Disable HTTP server when serverPort is 0 or missing

## Goal

REST API server should only start when `serverPort` is explicitly configured with a positive value. Default should be 0 (disabled).

## Current Behavior

Server always starts on port 8080 (default). No way to disable it.

## Expected Behavior

- `serverPort: 0` or missing → no HTTP server started
- `serverPort: 8080` → server starts on port 8080
- Default in `Defaults()` changes from `8080` to `0`

## Implementation

### 1. Change default serverPort to 0

In `pkg/config/config.go`, change `Defaults()`:
```go
ServerPort: 0,  // was 8080
```

### 2. Update validation

Remove the "must be between 1-65535" validation. Allow 0 (disabled). Only reject negative or >65535:
```go
if c.ServerPort < 0 || c.ServerPort > 65535 {
    return errors.Errorf(ctx, "serverPort must be 0 (disabled) or 1-65535, got %d", c.ServerPort)
}
```

### 3. Conditionally start server in runner

In `CreateRunner` or runner's `Run()`, only start the server goroutine if `serverPort > 0`:

```go
if cfg.ServerPort > 0 {
    // add server to goroutine list
}
```

If using `run.CancelOnFirstError`, conditionally include the server function.

### 4. Tests

- Default config has `ServerPort: 0`
- Config with `serverPort: 0` validates OK
- Config with `serverPort: 8080` validates OK
- Config with `serverPort: -1` fails validation
- Runner without server port: only watcher + processor run
- Runner with server port: watcher + processor + server run

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push
- Coverage ≥80% for changed packages
