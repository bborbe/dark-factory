---
status: created
created: "2026-03-11T16:45:24Z"
---

<summary>
- Time dependency is injected consistently instead of being constructed inline in business logic
- Package-level functions that create `libtime.NewCurrentDateTime()` inline now accept it as a parameter
- The `Lister`, `Counter`, and queue helper functions use injected time, making them testable with fixed timestamps
- Factory functions thread the shared `currentDateTimeGetter` through to all dependents
</summary>

<objective>
Replace inline `libtime.NewCurrentDateTime()` construction in business logic with injected `libtime.CurrentDateTimeGetter` dependencies. Currently six locations create this dependency inline instead of receiving it from their constructor or caller, breaking testability with fixed time.
</objective>

<context>
Read CLAUDE.md for project conventions.
The project uses `libtime "github.com/bborbe/time"` and the `libtime.CurrentDateTimeGetter` interface for time injection. Most structs already inject this correctly. The following locations construct it inline instead.
Read each file before editing.
</context>

<requirements>
1. **`pkg/spec/lister.go`**: Add `currentDateTimeGetter libtime.CurrentDateTimeGetter` field to the `lister` struct. Accept it as a parameter in `NewLister`. Use it in the `List` method instead of `libtime.NewCurrentDateTime()`.

2. **`pkg/prompt/counter.go`**: Add `currentDateTimeGetter libtime.CurrentDateTimeGetter` field to the `promptCounter` struct. Accept it as a parameter in `NewCounter`. Use it in `countInDir` instead of `libtime.NewCurrentDateTime()`.

3. **`pkg/prompt/prompt.go`**: The package-level functions `ListQueued`, `HasExecuting`, and similar functions that call `libtime.NewCurrentDateTime()` inline are wrapped by the `manager` methods. Since `manager` already has a `currentDateTimeGetter` field, thread it through: update the package-level functions to accept `currentDateTimeGetter libtime.CurrentDateTimeGetter` as a parameter, and have the `manager` methods pass `m.currentDateTimeGetter` when calling them.

4. **`pkg/server/queue_helpers.go`**: The `queueSingleFile` and `queueAllFiles` functions call `libtime.NewCurrentDateTime()` inline. Add `currentDateTimeGetter libtime.CurrentDateTimeGetter` as a parameter to both functions. Update their callers in `pkg/server/queue_action_handler.go` to pass the injected dependency (add the field to the handler struct if needed).

5. **`pkg/generator/generator.go`**: In `countCompletedPromptsForSpec`, replace inline `libtime.NewCurrentDateTime()` with `g.currentDateTimeGetter` (the `dockerSpecGenerator` struct already has this field).

6. Update `pkg/factory/factory.go` to pass `currentDateTimeGetter` to the updated constructors (`NewLister`, `NewCounter`, and any handler constructors that now need it). Use the existing `currentDateTimeGetter` that is already created in `CreateRunner`/`CreateOneShotRunner`.

7. Update all affected tests to pass a `currentDateTimeGetter` (use `libtime.NewCurrentDateTime()` in tests or a fixed-time fake).
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Use `libtime "github.com/bborbe/time"` — already imported in most files.
- Do not change the `CurrentDateTimeGetter` interface.
- Thread the dependency through constructors — do not use global variables.
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
