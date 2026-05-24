---
status: draft
created: "2026-05-24T00:00:00Z"
---

<summary>
- Added shell metacharacter validation for preflightCommand in config validation
- Rejects config with preflightCommand containing spaces, pipes, semicolons, or other shell metacharacters
- Replaces the #nosec G204 suppression with proper input validation
</summary>

<objective>
Add validation for preflightCommand config field to prevent arbitrary shell command injection.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `pkg/preflight/preflight.go` — line ~124, exec.CommandContext with sh -c
- `pkg/config/config.go` — where preflightCommand is validated (look for preflight or PreflightCommand)
- `pkg/config/config_test.go` — existing validation test patterns
</context>

<requirements>
1. In `pkg/config/config.go`, add validation for `preflightCommand` that:
   - Rejects values containing shell metacharacters: spaces, |, ;, &, <, >, (, ), $, `, \, ", ', *, ?, [, ], {, }, !, #, %, ^, newline
   - Allows alphanumeric, dash, underscore, slash, colon, dot, equals only
   - If invalid, return error via validation framework

2. Add tests for the validation:
   - Valid: simple command like `echo hello`
   - Invalid: `echo hello; rm -rf /`, `echo "test" | bash`, etc.

3. In `pkg/preflight/preflight.go`, remove the `#nosec G204` comment (no longer needed with validation in place).
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
make precommit
</verification>
