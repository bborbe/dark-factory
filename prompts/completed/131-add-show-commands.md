---
status: completed
summary: Added `spec show <id>` and `prompt show <id>` subcommands with --json support, factory wiring, main.go routing, counterfeiter mocks, and tests
container: dark-factory-131-add-show-commands
dark-factory-version: v0.24.0
created: "2026-03-07T21:10:00Z"
queued: "2026-03-07T20:16:53Z"
started: "2026-03-07T20:16:55Z"
completed: "2026-03-07T20:25:09Z"
---
<summary>
- Adds `dark-factory spec show <id>` to display a single spec's details (status, timestamps, linked prompts)
- Adds `dark-factory prompt show <id>` to display a single prompt's details (status, timestamps, spec link, summary, log path)
- Both reuse existing ID resolution logic (prefix match, exact name, path)
- Both support `--json` for machine-readable output
- Adds factory wiring and routing in `main.go` with help text
- Includes tests for valid ID, missing arg, and not-found cases
</summary>

<objective>
Add `show` subcommands for specs and prompts. Currently the CLI only has list views — operators debugging stuck specs or reviewing prompt details must open markdown files manually. `show` gives instant access to a single item's full details from the terminal.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `main.go` — command routing (`runSpecCommand`, `runPromptCommand` switch blocks) and `printHelp`.
Read `pkg/cmd/spec_complete.go` — reference pattern: takes `<id>` arg, uses `FindSpecFileInDirs` to find spec across dirs, calls `spec.Load` to read it. Constructor takes directory strings.
Read `pkg/cmd/spec_list.go` — reference pattern for `--json` output, uses `prompt.Counter` to count linked prompts.
Read `pkg/cmd/spec_finder.go` — `FindSpecFileInDirs(ctx, id, dirs...)` resolves ID by path, exact name, or prefix match.
Read `pkg/cmd/prompt_finder.go` — `FindPromptFile(ctx, dir, id)` resolves prompt ID by path, exact name, or prefix match. Searches one dir at a time.
Read `pkg/spec/spec.go` — `SpecFile` struct with `Frontmatter` (Status, timestamps), `Load(ctx, path)` function.
Read `pkg/prompt/prompt.go` — `Prompt` struct with `Frontmatter` (Status, timestamps, Spec field), `Load(ctx, path)` function.
Read `pkg/prompt/counter.go` — `Counter` interface with `CountBySpec(ctx, specName)`.
Read `pkg/factory/factory.go` — factory wiring pattern (`CreateSpec*Command`, `CreatePrompt*Command`).
</context>

<requirements>
1. Add `pkg/cmd/spec_show.go`:
   - Counterfeiter directive: `//counterfeiter:generate -o ../../mocks/spec-show-command.go --fake-name SpecShowCommand . SpecShowCommand`
   - Interface `SpecShowCommand` with `Run(ctx context.Context, args []string) error`.
   - Struct takes `inboxDir`, `inProgressDir`, `completedDir` strings (same as `specCompleteCommand`) and a `prompt.Counter`.
   - Constructor `NewSpecShowCommand(inboxDir, inProgressDir, completedDir string, counter prompt.Counter) SpecShowCommand`.
   - Takes one arg (spec ID). Error with "spec identifier required" if no arg.
   - Uses `FindSpecFileInDirs(ctx, id, inboxDir, inProgressDir, completedDir)` to find the file.
   - Calls `spec.Load(ctx, path)` to load the spec.
   - Uses `counter.CountBySpec(ctx, specName)` for linked prompt counts.
   - Prints: File, Status, non-zero timestamps (approved, prompted, verifying, completed — note: spec Frontmatter has no `created` field), Linked Prompts (completed/total).
   - `--json` flag outputs all fields as JSON.

2. Add `pkg/cmd/prompt_show.go`:
   - Counterfeiter directive: `//counterfeiter:generate -o ../../mocks/prompt-show-command.go --fake-name PromptShowCommand . PromptShowCommand`
   - Interface `PromptShowCommand` with `Run(ctx context.Context, args []string) error`.
   - Struct takes `inboxDir`, `inProgressDir`, `completedDir`, `logDir` strings.
   - Constructor `NewPromptShowCommand(inboxDir, inProgressDir, completedDir, logDir string) PromptShowCommand`.
   - Takes one arg (prompt ID). Error with "prompt identifier required" if no arg.
   - Searches all three dirs using `FindPromptFile(ctx, dir, id)` on each dir in order until found.
   - Calls `prompt.Load(ctx, path)` to load the prompt.
   - Prints: File, Status, Spec (linked spec IDs from `Frontmatter.Specs`), Summary, non-zero timestamps (created, queued, started, completed), Log path if log file exists (derive from `logDir`, not hardcoded).
   - `--json` flag outputs all fields as JSON.

3. Add `CreateSpecShowCommand` and `CreatePromptShowCommand` to `pkg/factory/factory.go`, following existing `Create*Command` patterns.

4. Add routing in `main.go`:
   - `case "show"` in `runSpecCommand` switch → call `SpecShowCommand.Run`.
   - `case "show"` in `runPromptCommand` switch → call `PromptShowCommand.Run`.
   - Add `spec show <id>` and `prompt show <id>` lines in `printHelp`.

5. Add tests:
   - `pkg/cmd/spec_show_test.go` — test valid ID (prints status/file), missing arg (error), not found (error).
   - `pkg/cmd/prompt_show_test.go` — test valid ID (prints status/file), missing arg (error), not found (error).

6. Generate counterfeiter mocks: run `go generate ./pkg/cmd/...`.

7. Remove any imports that become unused.
</requirements>

<constraints>
- Reuse `FindSpecFileInDirs` and `FindPromptFile` — do NOT reinvent ID matching logic
- Follow existing command patterns exactly (interface + private struct + constructor + counterfeiter)
- Do NOT modify existing commands
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
