<objective>
Add a `SpecGenerator` to dark-factory that runs the `/generate-prompts-for-spec` claude-yolo command for a given spec file. This is the Go side of spec 020: it receives a spec path, runs the YOLO container with the slash command as the prompt, and transitions the spec to `prompted` on success.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/executor/executor.go — SpecGenerator reuses the existing Executor interface.
Read pkg/spec/spec.go — for StatusPrompted, Load(), Save(), SetStatus().
Read pkg/generator/ — create this package (it does not exist yet).
Read ~/.claude-yolo/docs/go-patterns.md and go-testing.md for patterns.
</context>

<requirements>
1. Create `pkg/generator/generator.go` with:
   - Interface:
     ```go
     //counterfeiter:generate -o ../../mocks/spec-generator.go --fake-name SpecGenerator . SpecGenerator
     type SpecGenerator interface {
         Generate(ctx context.Context, specPath string) error
     }
     ```
   - Implementation `dockerSpecGenerator` that:
     a. Builds the prompt content: `/generate-prompts-for-spec <specPath>`
     b. Derives a container name from the spec filename (e.g. `dark-factory-gen-020-auto-prompt-generation`)
     c. Derives a log file path: `<logDir>/gen-<specBasename>.log`
     d. Calls `executor.Execute(ctx, promptContent, logFile, containerName)`
     e. Counts `.md` files in `inboxDir` before and after execution
     f. If no new files were created, returns an error: "generation produced no prompt files"
     g. On success, loads the spec file, calls `SetStatus(string(spec.StatusPrompted))`, saves it

   - Constructor:
     ```go
     func NewSpecGenerator(executor executor.Executor, inboxDir string, specsDir string, logDir string) SpecGenerator
     ```

2. Create `pkg/generator/generator_test.go` covering:
   - Success path: executor called, new file appears in inbox, spec set to `prompted`
   - No files produced: executor succeeds but inbox unchanged → error returned, spec stays unchanged
   - Executor error: error returned, spec stays unchanged

3. Run `make generate` to create the counterfeiter mock.
</requirements>

<constraints>
- Package name: `generator`
- Use `github.com/bborbe/errors` for error wrapping (not fmt.Errorf)
- Use `github.com/bborbe/dark-factory/pkg/executor` — do NOT import the docker implementation directly
- Use `github.com/bborbe/dark-factory/pkg/spec` for status constants and Load/Save
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
