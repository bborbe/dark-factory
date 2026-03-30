---
status: completed
summary: Added per-group help to prompt and spec commands — --help, -h, help, and no-args now print subcommand listings instead of returning an error
container: dark-factory-230-add-subcommand-help
dark-factory-version: v0.69.0
created: "2026-03-30T18:55:33Z"
queued: "2026-03-30T19:59:24Z"
started: "2026-03-30T20:42:36Z"
completed: "2026-03-30T20:57:33Z"
---

<summary>
- Users see available subcommands when running prompt or spec commands with help flags
- No-args invocation shows help instead of an error
- Help output is consistent with the main help screen
- Multiple help triggers work: --help, -h, help subcommand
- Tests cover help flags and no-args behavior
</summary>

<objective>
Add per-group help to `prompt` and `spec` commands so that `--help`, `-h`, `help`, and no-args all print available subcommands with descriptions instead of returning an error.
</objective>

<context>
Read CLAUDE.md for project conventions.

Key files to read before making changes:
- `main.go` — `printHelp()`, `runPromptCommand()`, `runSpecCommand()`
</context>

<requirements>

## 1. Add subcommand help

Currently `dark-factory spec --help` returns "unknown spec subcommand: --help". Fix this:

- Add `case "--help", "-h", "help"` to `runPromptCommand()` that prints prompt-specific help (list of prompt subcommands with descriptions)
- Add `case "--help", "-h", "help"` to `runSpecCommand()` that prints spec-specific help (list of spec subcommands with descriptions)
- Each subcommand help should list all available subcommands for that group with one-line descriptions

## 2. Also handle no-args case

In `runPromptCommand()` and `runSpecCommand()`, when `subcommand` is empty string (no args passed), call the same help printer instead of falling through to the default error case.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Match the output style of the existing `printHelp()` function
- Existing tests must still pass
- Add test cases in `parse_args_test.go` for help flags and no-args behavior
</constraints>

<verification>
```bash
make precommit
```
</verification>
