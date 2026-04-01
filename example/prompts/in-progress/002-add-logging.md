---
spec: ["002"]
status: approved
created: "2026-01-01T00:00:00Z"
---

<summary>
- Replaces all log.Printf calls with slog structured logging
- Configurable handler: JSON for production, text for development
</summary>

<objective>
Replace `log.Printf` with structured logging using `slog`.
</objective>

<requirements>
1. Use `slog.NewJSONHandler` for production output
2. Replace `log.Printf("message: %v", err)` with `slog.Error("message", "error", err)`
</requirements>

<constraints>
- Run `make precommit` for validation only
- Do NOT commit, tag, or push
- Coverage >=80% for changed packages
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
