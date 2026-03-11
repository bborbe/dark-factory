---
status: created
created: "2026-03-11T16:45:24Z"
---

<summary>
- Counterfeiter generation directives are placed consistently across the codebase
- Directives no longer appear inside GoDoc comment blocks where they pollute generated documentation
- Directives are on the line directly above their interface declaration
</summary>

<objective>
Fix counterfeiter directive placement in two files where the directive is either inside the doc comment or separated from the interface by a blank line. The canonical placement is: directive on the line directly above the `type ... interface` declaration, with the doc comment above the directive.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/git/cloner.go` and `pkg/processor/processor.go` before editing.
The correct pattern used everywhere else in the codebase is:
```go
//counterfeiter:generate -o ../../mocks/foo.go --fake-name Foo . Foo

// Foo does something.
type Foo interface {
```
</context>

<requirements>
1. In `pkg/git/cloner.go`, the directive is currently inside the doc comment block:
   ```go
   // Cloner handles local git clone operations.
   //
   //counterfeiter:generate -o ../../mocks/cloner.go --fake-name Cloner . Cloner
   type Cloner interface {
   ```
   Fix to:
   ```go
   //counterfeiter:generate -o ../../mocks/cloner.go --fake-name Cloner . Cloner

   // Cloner handles local git clone operations.
   type Cloner interface {
   ```

2. In `pkg/processor/processor.go`, the directive is separated from the interface by a blank line and the doc comment:
   ```go
   //counterfeiter:generate -o ../../mocks/processor.go --fake-name Processor . Processor

   // Processor processes queued prompts.
   type Processor interface {
   ```
   This is actually the correct pattern already. Verify it matches and leave it unchanged if correct. If there is a blank line between the directive and the doc comment that breaks the association, remove the extra blank line so the structure is:
   ```
   directive
   (blank line)
   doc comment
   type interface
   ```
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Do not change any logic — only reorder comment lines.
- Run `go generate ./...` after changes to verify counterfeiter still picks up the directives.
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
