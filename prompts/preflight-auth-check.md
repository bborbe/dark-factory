---
status: created
created: "2026-03-09T20:10:37Z"
---

<objective>
Add a preflight check that verifies the Claude config directory contains a valid, non-expired OAuth token before starting Docker execution. Fail fast with an actionable error message instead of letting the container exit with a cryptic exit code 4.
</objective>

<context>
- `pkg/executor/executor.go` — `resolveClaudeConfigDir()` determines the config dir (env `DARK_FACTORY_CLAUDE_CONFIG_DIR` or `~/.claude`)
- The config file is `{claudeConfigDir}/.claude.json` — JSON with optional `oauthAccount` object containing `accessToken` and `refreshToken`
- When OAuth tokens are expired/missing, Claude CLI inside Docker prints "Not logged in" and exits with code 4 in ~1 second
- The check should run in `Execute()` after resolving `claudeConfigDir` (line ~101) and before building the Docker command
</context>

<requirements>
1. Add a function `validateClaudeAuth(ctx context.Context, configDir string) error` in `pkg/executor/executor.go`
2. The function reads `{configDir}/.claude.json`, parses JSON, and checks:
   a. File exists — if not, return error: `"Claude config not found: {configDir}/.claude.json\n\nFix: Run 'CLAUDE_CONFIG_DIR={configDir} claude' and use /login"`
   b. Has `oauthAccount` with non-empty `accessToken` — if missing, return error: `"Claude OAuth token missing or expired in {configDir}\n\nFix: Run 'CLAUDE_CONFIG_DIR={configDir} claude' and use /login"`
3. Call `validateClaudeAuth` in `Execute()` after `resolveClaudeConfigDir` (line ~101), before `removeContainerIfExists`
4. If validation fails, return the error immediately (prompt is never started)
5. Use only `os`, `encoding/json` — no new dependencies
6. Do NOT validate token expiry timestamp (we cannot know the server-side state) — just check the token field is non-empty
7. Add tests in `pkg/executor/executor_internal_test.go`:
   - Config file missing → error with fix hint
   - Config file has no `oauthAccount` → error with fix hint
   - Config file has `oauthAccount` but empty `accessToken` → error with fix hint
   - Config file has valid `oauthAccount` with `accessToken` → no error
   - Config file with `ANTHROPIC_API_KEY` env var set → skip check (API key auth doesn't need OAuth)
</requirements>

<constraints>
- Do NOT change the Docker command or entrypoint
- Do NOT add new dependencies
- Keep the check fast (file read + JSON parse only, no network calls)
- The fix hint must use the short `~` form when the config dir is under home (use `resolveClaudeConfigDir` output as-is since it already uses `~` when from env)
- Follow existing test patterns in `executor_internal_test.go` (Ginkgo/Gomega, `Describe`/`Context`/`It`)
</constraints>

<verification>
```bash
make test
```
All tests pass including new auth validation tests.

Manual test: rename `.claude.json` in config dir, run `dark-factory run` — should fail immediately with actionable error instead of starting Docker.
</verification>

<success_criteria>
- `dark-factory run` with expired/missing OAuth token fails before Docker starts
- Error message includes the exact `CLAUDE_CONFIG_DIR=... claude` command to fix it
- `dark-factory run` with valid token or `ANTHROPIC_API_KEY` works unchanged
- No new dependencies added
</success_criteria>
