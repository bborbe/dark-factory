---
status: draft
tags:
    - dark-factory
    - prompt
---

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

<verification>
make precommit
</verification>
