---
status: queued
---

# Add structured logging

## Goal

Replace `log.Printf` with structured logging using `slog`.

## Implementation

### 1. Add slog handler

Use `slog.NewJSONHandler` for production output.

### 2. Update all log calls

Replace `log.Printf("message: %v", err)` with `slog.Error("message", "error", err)`.

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push
- Coverage >=80% for changed packages
