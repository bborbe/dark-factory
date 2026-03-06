<objective>
Wire the SpecWatcher and SpecGenerator into the dark-factory Runner and Factory. After this prompt, approving a spec automatically triggers prompt generation.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/runner/runner.go — add specWatcher as a third goroutine alongside watcher and processor.
Read pkg/factory/factory.go — add CreateSpecGenerator and CreateSpecWatcher factory functions, wire into CreateRunner.
Read pkg/config/config.go — check if specsDir is already in Config (it is passed as "specs" hardcoded in CreateRunner currently).
Read pkg/generator/generator.go and pkg/specwatcher/watcher.go (created in previous prompts).
Read ~/.claude-yolo/docs/go-patterns.md for patterns.
</context>

<requirements>
1. In `pkg/runner/runner.go`:
   - Add `specWatcher specwatcher.SpecWatcher` field to `runner` struct
   - Add `specWatcher specwatcher.SpecWatcher` parameter to `NewRunner`
   - Add `r.specWatcher.Watch` to the `runners` slice in `Run()` alongside `r.watcher.Watch` and `r.processor.Process`

2. In `pkg/factory/factory.go`:
   - Add `CreateSpecGenerator` factory function:
     ```go
     func CreateSpecGenerator(cfg config.Config, containerImage string) generator.SpecGenerator {
         return generator.NewSpecGenerator(
             executor.NewDockerExecutor(containerImage, project.Name(cfg.ProjectName), cfg.Model),
             cfg.InboxDir,
             cfg.SpecDir,
             cfg.LogDir,
         )
     }
     ```
   - Add `CreateSpecWatcher` factory function:
     ```go
     func CreateSpecWatcher(cfg config.Config, gen generator.SpecGenerator) specwatcher.SpecWatcher {
         return specwatcher.NewSpecWatcher(cfg.SpecDir, gen, time.Duration(cfg.DebounceMs)*time.Millisecond)
     }
     ```
   - Update `CreateRunner` to create and pass the SpecWatcher:
     ```go
     specGen := CreateSpecGenerator(cfg, cfg.ContainerImage)
     specWatcher := CreateSpecWatcher(cfg, specGen)
     ```
     Pass `specWatcher` to `runner.NewRunner(...)`.

3. Update `runner.NewRunner` signature to include `specWatcher specwatcher.SpecWatcher`.

4. Update all call sites and tests for `NewRunner` to pass the new parameter.
</requirements>

<constraints>
- Do NOT change the behavior of the existing watcher or processor
- Do NOT commit — dark-factory handles git
- Existing tests must still pass — update test call sites for NewRunner
- Use `github.com/bborbe/dark-factory/pkg/generator` and `pkg/specwatcher`
</constraints>

<verification>
Run `make precommit` — must pass.

Smoke test:
```bash
dark-factory spec approve specs/020-auto-prompt-generation.md
# touch the file to trigger fsnotify
touch specs/020-auto-prompt-generation.md
# check logs for "spec generator called" or similar
```
</verification>
