---
status: draft
created: "2026-03-11T16:45:24Z"
queued: "2026-03-11T18:25:03Z"
---

<summary>
- Private struct definitions now appear below their constructor functions in five more files
- The canonical Go file layout is consistently enforced across the entire codebase
- `generator.go`, `prompt.go`, `spec.go`, `cloner.go`, and `collaborator_fetcher.go` are reordered
- In `collaborator_fetcher.go`, sub-types (`ghRepoNameFetcher`, `ghCollaboratorLister`) are also reordered below their constructors
- No behavioral changes — purely structural reordering of type and function declarations
</summary>

<objective>
Fix file layout ordering violations in five remaining files where the private struct appears above the constructor. Continuation of the layout standardization.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read each file listed below before editing. Move struct definitions below their constructors. Do not change any code — only reorder declarations.
</context>

<requirements>
1. In `pkg/generator/generator.go`: move the `dockerSpecGenerator` struct (currently above `NewSpecGenerator`) to appear immediately after the `NewSpecGenerator` function.

2. In `pkg/prompt/prompt.go`: move the `manager` struct (currently above `NewManager` at ~line 474) to appear immediately after the `NewManager` function (~line 483).

3. In `pkg/spec/spec.go`: move the `autoCompleter` struct (currently above `NewAutoCompleter` at ~line 204) to appear immediately after the `NewAutoCompleter` function (~line 214).

4. In `pkg/git/cloner.go`: move the `cloner` struct (currently above `NewCloner` at ~line 25) to appear immediately after the `NewCloner` function (~line 28).

5. In `pkg/git/collaborator_fetcher.go`: move the `collaboratorFetcher` struct (currently above `NewCollaboratorFetcher` at ~line 35) to appear immediately after the `NewCollaboratorFetcher` function (~line 42). Also fix the two sub-types in the same file: move `ghRepoNameFetcher` struct below `NewGHRepoNameFetcher`, and `ghCollaboratorLister` struct below `NewGHCollaboratorLister`.

For each, the final order must be: Interface → Constructor → Struct → Methods.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Do not change any logic, imports, or function bodies — only reorder declarations.
- Keep doc comments attached to their respective declarations when moving.
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
