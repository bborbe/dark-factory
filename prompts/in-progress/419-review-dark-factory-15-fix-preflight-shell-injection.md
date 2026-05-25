---
status: failed
container: dark-factory-exec-419-review-dark-factory-15-fix-preflight-shell-injection
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T14:51:20Z"
started: "2026-05-25T18:31:53Z"
completed: "2026-05-25T18:33:23Z"
lastFailReason: 'validate completion report: completion report status: partial'
---

<summary>
- Added shell metacharacter validation for preflightCommand in config validation
- Rejects config with preflightCommand containing pipes, semicolons, redirection, command substitution, or other injection characters
- Spaces ARE allowed (default value is `make precommit`)
- Keeps `#nosec G204` in preflight.go with updated justification (validation is cross-package, gosec can't trace it)
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
   - **Allows** alphanumeric, dash, underscore, slash, colon, dot, equals, AND **space** (spaces are required because the default value is `make precommit`)
   - **Rejects** shell metacharacters that enable injection: `|`, `;`, `&`, `<`, `>`, `(`, `)`, `$`, backtick, `\`, `"`, `'`, `*`, `?`, `[`, `]`, `{`, `}`, `!`, `#`, `%`, `^`, newline, tab
   - If invalid, return error via validation framework using `errors.Errorf(ctx, ...)`

2. Add tests for the validation:
   - **Valid**: `echo hello`, `make precommit`, `go test ./...`, `true`, empty string
   - **Invalid**: `echo hello; rm -rf /`, `echo "test" | bash`, `$(whoami)`, `` `id` ``, `echo $HOME`, `cmd & background`, `cmd > /tmp/out`

3. In `pkg/preflight/preflight.go`, **KEEP** the `#nosec G204` comment but update its justification text to reference the config validation:

   ```go
   // #nosec G204 -- preflightCommand validated by Config.validatePreflightCommand to contain only safe characters
   cmd := exec.CommandContext(ctx, "sh", "-c", c.command)
   ```

   **Rationale:** gosec performs intra-file analysis and cannot trace cross-package validation, so removing `#nosec G204` would re-introduce the lint warning. The comment stays; the justification reflects the new safety guarantee.
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
make precommit
</verification>
