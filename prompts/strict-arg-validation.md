---
status: draft
created: "2026-04-03T00:00:00Z"
---

<summary>
- All commands reject unknown arguments and flags with a clear error message
- `--help` and `-h` work on every command and subcommand without side effects (no lock, no config load)
- Unknown commands show global help, unknown subcommands show command help
- Unknown arguments show the relevant help for the command that received them
</summary>

<objective>
Make the CLI strict: every command defines what arguments it accepts, rejects anything else with an error and the relevant help text. `--help` is intercepted in `ParseArgs` before any command runs, so it never acquires locks or loads config.
</objective>

<context>
Read CLAUDE.md for project conventions.

Key file: `main.go` — contains `ParseArgs()` (~line 243), `run()` (~line 30), `printHelp()` (~line 176), `printPromptHelp()` (~line 206), `printSpecHelp()` (~line 222), and all command dispatch logic.

Current problems:
1. `dark-factory run --help` passes `--help` as `args` to `run`, which acquires a lock and fails
2. `dark-factory run banana` silently ignores `banana` — the `args` slice is passed but never validated
3. Same issue for `daemon`, `kill`, `status`, `list`, `config` — all accept arbitrary args without validation
4. `prompt` and `spec` subcommands handle `--help` correctly (line 101, 134) but their subcommands (e.g., `prompt approve --help`) do not

How `ParseArgs` works today:
- Extracts `-debug` and `--auto-approve` flags from anywhere in args
- First remaining arg becomes `command`, rest becomes `args`
- `--help`/`-h` as command → returns `"help"` (correct)
- `run`, `daemon`, `kill`, `status`, `list`, `config` → returns `(command, "", rest)` — `rest` is never validated
- `prompt`, `spec` → returns `(command, rest[0], rest[1:])` — subcommand extracted, remaining args not validated
</context>

<requirements>
1. **Intercept `--help`/`-h` in `ParseArgs` for all commands**:

   After extracting `command` and `rest`, check if `rest` contains `--help`, `-help`, or `-h`. If so, return a help command instead of the actual command:

   - Top-level commands (`run`, `daemon`, etc.) with help flag → return `command="help-run"` (or similar) so the dispatcher can show command-specific help
   - `prompt`/`spec` subcommands with help flag → return `subcommand="help"` (already works for `prompt --help`, extend to `prompt approve --help`)

   Alternative approach: check for `--help` in `args` at the dispatch site in `run()` before executing the command. Choose whichever is simpler — the key requirement is that `--help` never reaches the command implementation.

2. **Define allowed args per command**:

   Add arg validation before each command executes in `run()`:

   - `run`: no positional args (only `--auto-approve` flag, already extracted)
   - `daemon`: no positional args
   - `kill`: no positional args
   - `status`: no positional args
   - `list`: no positional args
   - `config`: no positional args
   - `prompt approve <slug>`: exactly 1 arg (the slug)
   - `prompt requeue <slug>`: 1 arg or `--failed` flag
   - `prompt cancel <slug>`: exactly 1 arg
   - `prompt complete <slug>`: exactly 1 arg
   - `prompt unapprove <slug>`: exactly 1 arg
   - `prompt show <slug>`: exactly 1 arg
   - `prompt list`: no args
   - `prompt status`: no args
   - `spec approve <slug>`: exactly 1 arg
   - `spec unapprove <slug>`: exactly 1 arg
   - `spec complete <slug>`: exactly 1 arg
   - `spec show <slug>`: exactly 1 arg
   - `spec list`: no args
   - `spec status`: no args

3. **Reject unknown args with error + help**:

   When a command receives args it doesn't expect:
   - Print to stderr: `"unknown argument: %q\n"` followed by the command's help text
   - Return an error

   For top-level commands, create per-command help functions (e.g., `printRunHelp()`, `printDaemonHelp()`) or a generic one-liner (e.g., `"Usage: dark-factory run [--auto-approve]\n\nProcess all queued prompts and exit.\n"`).

4. **Reject unknown flags in args**:

   Any arg starting with `-` that isn't a known flag for that command should be rejected. This catches `dark-factory run --foo` and `dark-factory daemon -x`.

5. **Add tests to `main_test.go`** (or create if needed):

   Test `ParseArgs`:
   - `["run", "--help"]` → help for run (not actual run)
   - `["daemon", "-h"]` → help for daemon
   - `["prompt", "approve", "--help"]` → help for prompt approve
   - `["run", "banana"]` → still returns run command (validation happens at dispatch, not parse)

   Test arg validation (if extracted into a helper):
   - `run` with `["banana"]` → error
   - `run` with `[]` → ok
   - `prompt approve` with `["my-slug"]` → ok
   - `prompt approve` with `[]` → error (missing slug)
   - `prompt approve` with `["a", "b"]` → error (too many args)

6. **Update `printHelp()`** to mention `--auto-approve` flag for `run` command.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- All existing tests must pass
- `make precommit` must pass
- Keep `ParseArgs` signature unchanged — it's tested and used in one place
- Error messages go to stderr, help text to stdout (match existing pattern)
- Use `github.com/bborbe/errors` for error wrapping (never `fmt.Errorf`)
</constraints>

<verification>
Run `make precommit` — must pass.

Manual checks:
```bash
# Help flags work without side effects
dark-factory run --help    # shows run help, no lock acquired
dark-factory daemon -h     # shows daemon help
dark-factory prompt approve --help  # shows prompt help

# Unknown args rejected
dark-factory run banana    # error: unknown argument "banana" + run help
dark-factory daemon foo    # error: unknown argument "foo" + daemon help
dark-factory status --foo  # error: unknown flag "--foo" + status help

# Valid commands still work
dark-factory status        # shows status
dark-factory prompt list   # lists prompts
dark-factory config        # shows config
```
</verification>
