---
status: approved
created: "2026-03-30T22:10:00Z"
queued: "2026-03-30T20:15:19Z"
---

<summary>
- Status output shows which project directory dark-factory resolved to
- Helps when running from subdirectories to confirm the correct project root
- Project dir appears as the first line after the header in human-readable output
- JSON output includes a new project_dir field
- All three NewChecker() call sites updated with the new parameter
</summary>

<objective>
Add project directory to `dark-factory status` output so users can see which project root was resolved, especially when running from subdirectories.
</objective>

<context>
Read CLAUDE.md for project conventions.

Key files to read before making changes:
- `pkg/status/status.go` — `Status` struct (~line 26), `NewChecker()` (~line 75), `checker` struct (~line 65)
- `pkg/status/formatter.go` — `Format()` method (~line 30)
- `pkg/factory/factory.go` — three `NewChecker()` call sites: `CreateServer()` (~line 525), `CreateStatusCommand()` (~line 566), `CreateCombinedStatusCommand()` (~line 727)
</context>

<requirements>

## 1. Add `ProjectDir` to Status struct

In `pkg/status/status.go`:
- Add `ProjectDir string` field to `Status` struct with JSON tag `json:"project_dir,omitempty"`

## 2. Add project dir to checker

In `pkg/status/status.go`:
- Add `projectDir string` field to `checker` struct
- Add `projectDir string` as first parameter to `NewChecker()`
- Set `status.ProjectDir = s.projectDir` in `GetStatus()` before returning

## 3. Show project dir in formatter

In `pkg/status/formatter.go`, in `Format()`, add `Project:` line right after the "Dark Factory Status" header and before the Daemon line:
```
Dark Factory Status
  Project:    /path/to/project
  Daemon:     running (pid 41936)
```

If `ProjectDir` is empty, omit the line.

## 4. Update all three factory call sites

The project directory is the current working directory (where `.dark-factory.yaml` lives). Use `os.Getwd()` or pass it through.

In `pkg/factory/factory.go`, update all three `NewChecker()` calls at:
- `CreateServer()` (~line 525)
- `CreateStatusCommand()` (~line 566)
- `CreateCombinedStatusCommand()` (~line 727)

Each must pass the project directory as the first argument.

## 5. Update tests

- Update `pkg/status/status_test.go` — add `projectDir` to all `NewChecker()` calls
- Update `pkg/status/formatter_test.go` — verify `Project:` line appears in output when set, omitted when empty
- Update any other files that call `NewChecker()` (grep for `NewChecker(` to find all)

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Follow existing patterns in status.go and formatter.go
</constraints>

<verification>
```bash
make precommit
```
</verification>
