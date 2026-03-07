---
status: completed
summary: Made Docker executor Claude config dir configurable via DARK_FACTORY_CLAUDE_CONFIG_DIR env var, defaulting to ~/.claude, with ~ and $HOME expansion support
container: dark-factory-128-make-claude-config-dir-configurable
dark-factory-version: v0.23.2
created: "2026-03-07T19:33:54Z"
queued: "2026-03-07T19:33:54Z"
started: "2026-03-07T19:33:59Z"
completed: "2026-03-07T19:41:28Z"
---
<summary>
- Removes hardcoded assumption about where Claude's config lives on the host machine
- Lets different developers point the Docker executor at their own Claude config directory
- Controlled by an environment variable with a sensible default
- Existing tests updated to cover both the override and the default case
</summary>

<objective>
The Docker executor's Claude config directory is machine-configurable via an environment variable, defaulting to `~/.claude`. No project config changes required.
</objective>

<context>
Read CLAUDE.md for project conventions.

Key files:
- `pkg/executor/executor.go:~106` — debug log with hardcoded `home+"/.claude-yolo:/home/node/.claude"`
- `pkg/executor/executor.go:~188` — `-v` mount argument with hardcoded `home+"/.claude-yolo:/home/node/.claude"` inside `buildDockerCommand`
- `pkg/executor/executor.go:~34` — `NewDockerExecutor` constructor
- `pkg/executor/executor_test.go` — executor tests
- `pkg/executor/executor_internal_test.go:~259` — internal test asserting `"/home/user/.claude-yolo:/home/node/.claude"` in volume mounts (will break after change)
</context>

<requirements>
1. In `pkg/executor/executor.go`, read `DARK_FACTORY_CLAUDE_CONFIG_DIR` from the environment in the `Execute` method (after resolving `home`). If empty, default to `home + "/.claude"`. Expand `~` or `$HOME` if present.

2. Pass the resolved config dir path to `buildDockerCommand`. Update `buildDockerCommand` signature to accept the resolved path (instead of constructing it from `home`). Use it in both the `-v` mount argument (~line 188) and the debug log (~line 106).

3. Do NOT change `NewDockerExecutor` signature — the env var is read at execution time, not construction time.

4. Update tests in `pkg/executor/executor_test.go` and `pkg/executor/executor_internal_test.go`:
   - Test that `DARK_FACTORY_CLAUDE_CONFIG_DIR` is used when set
   - Test that default `~/.claude` is used when env var is unset
   - Fix the existing internal test at ~line 259 that asserts `"/home/user/.claude-yolo:/home/node/.claude"` — update to match the new default `"/home/user/.claude:/home/node/.claude"`
   - Use `t.Setenv` (or Ginkgo equivalent) to set/unset the env var in tests
</requirements>

<constraints>
- Do NOT add a config field to `Config` struct — this is an env var only (machine-specific, not project-specific)
- Do NOT change `NewDockerExecutor` signature
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
