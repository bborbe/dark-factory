---
status: completed
container: dark-factory-037-fix-log-dir
dark-factory-version: v0.8.1
---



# Fix log directory path

## Goal

Log files are written to `prompts/queue/log/` because processor uses `queueDir + "/log"`. Should be `prompts/log/` by default.

## Implementation

### 1. Add `logDir` to Config

```go
type Config struct {
    // ... existing fields ...
    LogDir string `yaml:"logDir"`
}
```

Default: `prompts/log`

Add validation: non-empty string.

### 2. Pass `logDir` to processor

Update `NewProcessor` to accept `logDir string`. Use it instead of `filepath.Join(p.queueDir, "log", ...)`.

### 3. Update factory

Pass `cfg.LogDir` to `CreateProcessor`.

### 4. Update status command

`CreateStatusCommand` has hardcoded `logDir := "prompts/log"` — use `cfg.LogDir` instead.

### 5. Tests

- Default config has `LogDir: "prompts/log"`
- Processor writes logs to configured logDir
- Config validation rejects empty logDir

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push
- Coverage ≥80% for changed packages
