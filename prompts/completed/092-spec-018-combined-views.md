---
spec: ["018"]
status: completed
summary: Implemented CombinedStatusCommand and CombinedListCommand that show both prompt and spec information together, wired into top-level dark-factory status and list commands with --json flag support
container: dark-factory-092-spec-019-combined-views
dark-factory-version: v0.17.15
created: "2026-03-06T10:57:15Z"
queued: "2026-03-06T10:57:15Z"
started: "2026-03-06T11:51:20Z"
completed: "2026-03-06T12:00:23Z"
---
<objective>
Implement the combined top-level `dark-factory status` and `dark-factory list` commands that show both prompt and spec information together.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read main.go for command routing.
Read pkg/cmd/status.go for prompt status command.
Read pkg/cmd/spec_status.go for spec status command.
Read pkg/cmd/list.go for prompt list command.
Read pkg/cmd/spec_list.go for spec list command.
</context>

<requirements>
1. Update `dark-factory status` (top-level) to show combined output:
   - First: prompt status (existing output)
   - Blank line separator
   - Then: spec status (from spec status command)
   - Support `--json` flag: output a JSON object with `prompts` and `specs` keys

2. Update `dark-factory list` (top-level) to show combined output:
   - First: prompt list with header `PROMPTS:`
   - Blank line separator
   - Then: spec list with header `SPECS:`
   - Support `--json` flag: output a JSON object with `prompts` and `specs` arrays

3. Create combined command implementations in `pkg/cmd/`:
   - `CombinedStatusCommand` that delegates to both prompt and spec status
   - `CombinedListCommand` that delegates to both prompt and spec list

4. Wire in main.go and factory.

5. Add tests for combined commands.
</requirements>

<constraints>
- `dark-factory prompt status` and `dark-factory spec status` still work independently
- Combined view is additive — does not change individual command output
- `make precommit` must pass
- Do NOT commit, tag, or push
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
