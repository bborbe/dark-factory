---
status: draft
created: "2026-03-30T18:55:33Z"
---

<summary>
- `dark-factory prompt --help` and `dark-factory spec --help` show available subcommands instead of erroring
- `-h` and `help` also work as aliases
- Running `dark-factory prompt` or `dark-factory spec` with no args shows help instead of an error
- Help output follows the style of the existing top-level `printHelp()`
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

When `dark-factory prompt` or `dark-factory spec` is called with no subcommand, print the subcommand help instead of an error.

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
