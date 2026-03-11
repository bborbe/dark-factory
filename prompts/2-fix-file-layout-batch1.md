---
status: created
created: "2026-03-11T16:45:24Z"
---

<summary>
- Private struct definitions now appear below their constructor functions, not above
- The canonical Go file layout (Interface → Constructor → Struct → Methods) is enforced in six core files
- `processor.go`, `runner.go`, `oneshot.go`, `watcher.go`, `specwatcher/watcher.go`, and `executor.go` are reordered
- Doc comments remain attached to their respective declarations after reordering
- No behavioral changes — purely structural reordering of type and function declarations
</summary>

<objective>
Fix file layout ordering violations in six files where the private struct appears above the constructor. The project convention requires: Interface → Constructor (`New*`) → Struct → Methods.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read each file listed below before editing. The fix is purely mechanical: move the struct definition (with its doc comment) to appear after the constructor function. Do not change any code — only reorder the declarations.
</context>

<requirements>
1. In `pkg/processor/processor.go`: move the `processor` struct (currently above `NewProcessor`) to appear immediately after the `NewProcessor` function. Keep the doc comment `// processor implements Processor.` attached to the struct.

2. In `pkg/runner/runner.go`: move the `runner` struct (currently above `NewRunner`) to appear immediately after the `NewRunner` function.

3. In `pkg/runner/oneshot.go`: move the `oneShotRunner` struct (currently above `NewOneShotRunner`) to appear immediately after the `NewOneShotRunner` function.

4. In `pkg/watcher/watcher.go`: move the `watcher` struct (currently above `NewWatcher`) to appear immediately after the `NewWatcher` function.

5. In `pkg/specwatcher/watcher.go`: move the `specWatcher` struct (currently above `NewSpecWatcher`) to appear immediately after the `NewSpecWatcher` function.

6. In `pkg/executor/executor.go`: move the `dockerExecutor` struct (currently above `NewDockerExecutor`) to appear immediately after the `NewDockerExecutor` function.

For each file, the final order must be:
```
// Interface doc comment
type FooInterface interface { ... }

// Constructor doc comment
func NewFoo(...) FooInterface { ... }

// Struct doc comment
type foo struct { ... }

// Methods...
```
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
