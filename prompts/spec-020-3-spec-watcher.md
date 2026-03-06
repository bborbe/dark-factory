<objective>
Add a `SpecWatcher` to dark-factory that watches the `specs/` directory with fsnotify. When a spec file changes and its status is `approved`, it signals the SpecGenerator. This mirrors the existing queue Watcher pattern exactly.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/watcher/watcher.go — SpecWatcher follows this pattern exactly (fsnotify, debounce, ready channel).
Read pkg/spec/spec.go — for Load() and StatusApproved.
Read pkg/generator/generator.go (created in previous prompt) — SpecWatcher calls generator.Generate().
Read ~/.claude-yolo/docs/go-patterns.md and go-testing.md for patterns.
</context>

<requirements>
1. Create `pkg/specwatcher/watcher.go` with:
   - Interface:
     ```go
     //counterfeiter:generate -o ../../mocks/spec-watcher.go --fake-name SpecWatcher . SpecWatcher
     type SpecWatcher interface {
         Watch(ctx context.Context) error
     }
     ```
   - Implementation `specWatcher` that:
     a. Uses fsnotify to watch `specsDir`
     b. On `.md` file Write/Create/Chmod events, debounces with the configured duration
     c. After debounce fires: reads the changed file, checks if `status == approved`
     d. If approved: calls `generator.Generate(ctx, specPath)` — if error, logs it (does not crash)
     e. Only one generation runs at a time (use a mutex or single-flight pattern)

   - Constructor:
     ```go
     func NewSpecWatcher(specsDir string, generator generator.SpecGenerator, debounce time.Duration) SpecWatcher
     ```

2. Create `pkg/specwatcher/watcher_test.go` covering:
   - Approved spec detected → generator called with correct path
   - Non-approved spec (draft/completed) → generator NOT called
   - Generator error → logged, watcher continues (does not return error)

3. Run `make generate` to create the counterfeiter mock.
</requirements>

<constraints>
- Package name: `specwatcher`
- Use `github.com/fsnotify/fsnotify` — already a project dependency
- Use `github.com/bborbe/errors` for error wrapping
- Use `github.com/bborbe/dark-factory/pkg/spec` for Load() and StatusApproved
- Use `github.com/bborbe/dark-factory/pkg/generator` for the SpecGenerator interface
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
