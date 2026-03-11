---
status: approved
created: "2026-03-11T16:45:24Z"
queued: "2026-03-11T18:25:03Z"
---

<summary>
- Counterfeiter generation directives are placed consistently across the codebase
- Directives no longer appear inside GoDoc comment blocks where they pollute generated documentation
- The directive in `cloner.go` is extracted from inside the doc comment to stand alone above it
- The directive in `processor.go` is verified as already correct and left unchanged
- The canonical pattern is: directive → blank line → doc comment → type interface
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

2. In `pkg/processor/processor.go`, verify the directive placement is already correct (directive → blank line → doc comment → type interface). If it matches, leave it unchanged — no edits needed.
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
