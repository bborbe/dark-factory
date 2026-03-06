---
status: prompted
---

# Structured Logging

## Problem

The project uses `log.Printf` for all logging. Output is unstructured plain text — hard to search, filter, or pipe into log aggregators. There is no consistent field format across log call sites.

## Goal

All logging uses `slog` with consistent key-value fields. Output is structured JSON in production and human-readable text in development.

## Non-goals

- No log aggregation setup (that's infrastructure, not code)
- No changes to log levels beyond what already exists

## Desired Behavior

1. All `log.Printf` / `log.Println` calls are replaced with `slog` equivalents.
2. Errors are logged with a consistent `"error"` key: `slog.Error("msg", "error", err)`.
3. The handler is configurable: JSON for production, text for development.

## Constraints

- Use stdlib `log/slog` — no third-party logging libraries
- Run `make precommit` for validation
- Do NOT commit — dark-factory handles git
- Coverage ≥ 80% for changed packages

## Failure Modes

| Trigger | Expected behavior |
|---------|------------------|
| slog not available | Won't happen — stdlib since Go 1.21 |
| Mixed old/new log calls | Caught by grep in verification |

## Acceptance Criteria

- [ ] No `log.Printf` / `log.Println` remaining in non-test code
- [ ] All error logs use `"error"` key
- [ ] `make precommit` passes

## Verification

```
grep -r "log\.Printf\|log\.Println" --include="*.go" .
make precommit
```

## Do-Nothing Option

Keep `log.Printf`. Works fine for small projects. Becomes painful when logs need to be searched or aggregated at scale.
