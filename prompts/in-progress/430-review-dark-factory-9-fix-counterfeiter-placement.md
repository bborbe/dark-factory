---
status: approved
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T14:51:20Z"
---

<summary>
- Fixed counterfeiter directive placement in 3 files
- subproc.go: moved directive directly above interface (was separated by const block)
- generator.go: reordered so constructor NewSpecGenerator comes after dockerSpecGenerator struct
- queuescanner.go: reordered so NewScanner comes after scanner struct
</summary>

<objective>
Fix file layout violations where constructors appear before the structs they construct, and counterfeiter directives not directly above interfaces.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` for the Interface → Constructor → Struct pattern.

Files to read before making changes:
- `pkg/subproc/subproc.go` — line ~20 counterfeiter directive, line ~26 Runner interface, const block in between
- `pkg/generator/generator.go` — line ~28 SpecGenerator interface, line ~34 NewSpecGenerator constructor, line ~71 dockerSpecGenerator struct
- `pkg/queuescanner/scanner.go` — line ~25 counterfeiter for Scanner, line ~31 PromptProcessor interface, line ~47 Scanner interface, line ~59 NewScanner, line ~74 scanner struct
</context>

<requirements>
1. In `pkg/subproc/subproc.go`: Move the `//counterfeiter:generate ...` directive to the line directly above `type Runner interface`. Move the const block below the interface declaration.

2. In `pkg/generator/generator.go`: Move `func NewSpecGenerator(...)` to after `type dockerSpecGenerator struct`. The canonical order is: Interface → Struct → Constructor.

3. In `pkg/queuescanner/scanner.go`: Move `func NewScanner(...)` to after `type scanner struct`. Also ensure `PromptProcessor` and `PromptManager` interfaces have their counterfeiter directives directly above them (not separated by other declarations).

4. Run `go generate ./...` after changes to regenerate mocks.
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
- Interface → Struct → Constructor is the canonical order
- Counterfeiter directive must be directly above the interface it targets
</constraints>

<verification>
make precommit
</verification>
