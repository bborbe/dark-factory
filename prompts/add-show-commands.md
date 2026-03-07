---
status: ""
created: "2026-03-07T21:10:00Z"
---
<summary>
- Adds `dark-factory spec show <id>` command to display a single spec's details
- Adds `dark-factory prompt show <id>` command to display a single prompt's details
- Spec show: status, timestamps, acceptance criteria, linked prompts with their status
- Prompt show: status, timestamps, spec linkage, summary, log file path
</summary>

<objective>
Add `show` subcommands for specs and prompts so users can inspect a single item's full details without reading the markdown file manually.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `main.go` — command routing and help text.
Read `pkg/cmd/spec_list.go` — existing spec command pattern (interface, struct, constructor, counterfeiter generate).
Read `pkg/cmd/list.go` — existing prompt list command pattern.
Read `pkg/spec/spec.go` — `Spec` struct, `Frontmatter`, `Lister`, `Finder` interfaces.
Read `pkg/prompt/prompt.go` — `Prompt` struct, `Frontmatter`, finder/lister interfaces.
Read `pkg/prompt/counter.go` — `Counter` interface for counting prompts by spec.
Read `pkg/factory/factory.go` — factory wiring pattern for new commands.
Read `pkg/cmd/spec_complete.go` — example of a command that takes an `<id>` argument and finds a spec by ID.
</context>

<requirements>
1. Add `pkg/cmd/spec_show.go`:
   - Interface `SpecShowCommand` with `Run(ctx context.Context, args []string) error`.
   - Counterfeiter generate directive for mock.
   - Constructor `NewSpecShowCommand` accepting spec `Finder` and prompt `Counter`.
   - Takes one arg: spec ID (numeric prefix or full name). Error if no arg.
   - Finds the spec file, loads it, prints:
     - **File**: filename
     - **Status**: current status
     - **Timestamps**: created, approved, prompted, verifying, completed (only non-zero)
     - **Linked Prompts**: count completed/total, list each prompt file with its status
   - Support `--json` flag for JSON output.

2. Add `pkg/cmd/prompt_show.go`:
   - Interface `PromptShowCommand` with `Run(ctx context.Context, args []string) error`.
   - Counterfeiter generate directive for mock.
   - Constructor `NewPromptShowCommand` accepting a prompt finder/loader.
   - Takes one arg: prompt ID (numeric prefix or full name). Error if no arg.
   - Finds the prompt file across inbox/in-progress/completed dirs, loads it, prints:
     - **File**: filename
     - **Status**: current status
     - **Spec**: linked spec IDs (from frontmatter `spec` field)
     - **Summary**: summary text
     - **Timestamps**: created, queued, started, completed (only non-zero)
     - **Log**: path to log file if it exists
   - Support `--json` flag for JSON output.

3. Add `CreateSpecShowCommand` and `CreatePromptShowCommand` to `pkg/factory/factory.go`.

4. Add routing in `main.go`:
   - `case "show"` under `runSpecCommand` → call `SpecShowCommand`.
   - `case "show"` under `runPromptCommand` → call `PromptShowCommand`.
   - Add help text lines: `spec show <id>` and `prompt show <id>`.

5. Add tests for both commands:
   - `pkg/cmd/spec_show_test.go` — test with valid ID, missing arg, not found.
   - `pkg/cmd/prompt_show_test.go` — test with valid ID, missing arg, not found.

6. Generate counterfeiter mocks by running `go generate ./pkg/cmd/...`.

7. Remove any imports that become unused.
</requirements>

<constraints>
- Follow existing command patterns exactly (interface + struct + constructor + counterfeiter)
- Spec ID matching must use int prefix parsing (same as spec_complete.go)
- Prompt ID matching must use int prefix parsing (same pattern)
- Do NOT modify existing commands
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
