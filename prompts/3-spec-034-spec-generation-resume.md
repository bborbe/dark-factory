---
status: created
spec: ["034"]
created: "2026-03-21T18:00:00Z"
branch: dark-factory/resume-executing-on-restart
---

<summary>
- Spec generation containers (dark-factory-gen-*) are also resumed on restart instead of killed and restarted
- The SpecGenerator checks whether the generation container is already running before launching a new one
- If the container is running, Reattach is used to monitor it to completion instead of Execute
- After reattach, the same post-generation flow runs (scan inbox for new prompts, set spec status to prompted)
- The ContainerChecker interface (from prompt 1) is injected into the generator via its constructor and the factory
- No behavioral change when no generation container is running (the common case)
</summary>

<objective>
Extend the re-attach mechanism to spec prompt generation containers. When dark-factory restarts while a spec generation container is running, the generator detects the live container and reattaches to it instead of killing and restarting it, avoiding wasted work.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read all `/home/node/.claude/docs/go-*.md` docs before starting.

**This prompt builds on prompts 1 and 2** (container liveness checker + Reattach method). Both are prerequisites.

Read `pkg/generator/generator.go` — `SpecGenerator` interface, `dockerSpecGenerator` struct, `Generate` method.
Read `pkg/executor/checker.go` — `ContainerChecker` interface (from prompt 1).
Read `pkg/executor/executor.go` — `Executor` interface including `Reattach` method (from prompt 2).
Read `pkg/factory/factory.go` — `CreateSpecGenerator` function (line ~312), where executor and dirs are wired.
Read `pkg/generator/generator_test.go` — existing test patterns using the Executor mock.
Read `mocks/executor.go` — the counterfeiter mock for Executor (now includes Reattach from prompt 2).
Read `mocks/container-checker.go` — the counterfeiter mock for ContainerChecker (from prompt 1).
</context>

<requirements>
1. **Add `ContainerChecker` field to `dockerSpecGenerator`** in `pkg/generator/generator.go`:
   - Add `containerChecker executor.ContainerChecker` field to the private struct.
   - Add `containerChecker executor.ContainerChecker` parameter to `NewSpecGenerator` (after the `executor` parameter).
   - Update the constructor body to assign the new field.

2. **Modify `Generate` in `pkg/generator/generator.go`** — check for a running container before executing:
   After deriving `containerName` (line ~66) and `logFile` (line ~69), but BEFORE the `beforeFiles` snapshot, add:
   ```go
   // Check if the generation container is already running (dark-factory restarted mid-generation)
   running, err := g.containerChecker.IsRunning(ctx, containerName)
   if err != nil {
       slog.Warn("failed to check container liveness, starting fresh generation",
           "container", containerName, "error", err)
       running = false
   }
   if running {
       slog.Info("reattaching to running spec generation container", "container", containerName)
       if err := g.executor.Reattach(ctx, logFile, containerName); err != nil {
           return errors.Wrap(ctx, err, "reattach to spec generation container")
       }
       slog.Info("spec generation container exited via reattach", "container", containerName)
       // After reattach: check if prompts were generated (generation may have completed before restart)
       afterFiles, err := listMDFiles(g.inboxDir)
       if err != nil {
           return errors.Wrap(ctx, err, "count inbox files after reattach")
       }
       // Use empty beforeFiles to capture any files created during original run
       newFiles := diffFiles(map[string]struct{}{}, afterFiles)
       // Filter to only files that belong to this spec (match specBasename pattern)
       newFiles = filterBySpec(newFiles, specBasename)
       if len(newFiles) == 0 {
           slog.Info("no new prompt files found after reattach; spec generation may have completed earlier",
               "spec", specBasename)
           return nil
       }
       // Apply spec metadata inheritance and set spec to prompted
       sf, err := spec.Load(ctx, specPath, g.currentDateTimeGetter)
       if err != nil {
           return errors.Wrap(ctx, err, "load spec file after reattach")
       }
       specBranch := sf.Frontmatter.Branch
       specIssue := sf.Frontmatter.Issue
       if specBranch != "" || specIssue != "" {
           if err := inheritFromSpec(ctx, newFiles, specBranch, specIssue, g.currentDateTimeGetter); err != nil {
               return errors.Wrap(ctx, err, "inherit spec metadata to prompts after reattach")
           }
       }
       sf.SetStatus(string(spec.StatusPrompted))
       if err := sf.Save(ctx); err != nil {
           return errors.Wrap(ctx, err, "save spec file after reattach")
       }
       return nil
   }
   ```
   The rest of `Generate` (beforeFiles snapshot → Execute → afterFiles → diff → inherit → save) runs unchanged when `running == false`.

3. **Add `filterBySpec(files []string, specBasename string) []string`** (package-level function in `pkg/generator/generator.go`):
   Returns only files whose basename contains the spec number extracted from `specBasename`.
   - Extract the numeric prefix from `specBasename` using the existing `specnum.Parse` (imported from `pkg/specnum`).
   - If no numeric prefix, return `files` unchanged (cannot filter, be conservative).
   - Otherwise, filter to files whose basename contains the spec number (e.g., `specnum.Parse("034-...")` → `34`, format as `%03d`).
   - Actually, a simpler approach: since the generated prompts include the spec ID in their `spec:` frontmatter field, this filtering by filename may not be reliable. Instead, just return all files in `afterFiles` that were not there before (using empty `beforeFiles` is fine — we want any file that appeared since the original generation start, and since generation containers only create files for their spec, this is safe).
   - Simplify: remove the `filterBySpec` approach entirely. Just use `diffFiles(map[string]struct{}{}, afterFiles)` to get all current inbox files. Accept that this may include pre-existing files if any existed before the original generation. To avoid this, use a `countCompletedPromptsForSpec` check (already exists) as a fallback if `newFiles` is empty.
   - **Revised approach for reattach**: After reattach, scan inbox for files with `spec:` frontmatter matching this spec:
     ```go
     // After reattach: scan inbox for prompt files belonging to this spec
     newFiles, err := g.listPromptsForSpec(ctx, g.inboxDir, specBasename)
     if err != nil {
         return errors.Wrap(ctx, err, "list prompts for spec after reattach")
     }
     if len(newFiles) == 0 {
         // Generation may have completed and moved files before restart
         slog.Info("no prompt files found in inbox for spec after reattach",
             "spec", specBasename)
         return nil
     }
     ```
   - Implement `listPromptsForSpec(ctx, dir, specID string) ([]string, error)` (private method on `dockerSpecGenerator`) that scans `dir` for `.md` files and returns those with `spec: ["NNN"]` frontmatter matching `specID` using `pf.Frontmatter.HasSpec(specID)`.

4. **Update `CreateSpecGenerator` in `pkg/factory/factory.go`**:
   Pass `executor.NewDockerContainerChecker()` as the second argument to `generator.NewSpecGenerator` (after the `executor.NewDockerExecutor(...)` call, before `cfg.Prompts.InboxDir`).

5. **Update tests in `pkg/generator/generator_test.go`**:
   - Add a `containerChecker *mocks.ContainerChecker` field to the test setup.
   - Initialize it in `BeforeEach`: `containerChecker = &mocks.ContainerChecker{}`
   - Pass it to `generator.NewSpecGenerator` in the test setup (second arg after executor).
   - Update all existing `NewSpecGenerator` calls in tests to pass `containerChecker`.
   - Add new test cases:
     - **Case: container is running → Reattach called, not Execute; prompts found in inbox → spec set to prompted**
     - **Case: container is running → Reattach called; no prompts found in inbox → returns nil without error**
     - **Case: container check returns error → falls back to fresh Execute**
   - In existing tests, set `containerChecker.IsRunningReturns(false, nil)` so they continue to test the fresh-execute path.

6. **GoDoc** for all new methods and the updated constructor.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- The reattach path must NOT call `executor.Execute` — it only calls `executor.Reattach`
- The fresh execution path (when container is not running) must remain unchanged
- `listPromptsForSpec` is best-effort: if frontmatter cannot be parsed, skip the file silently
- `filterBySpec` / `listPromptsForSpec` must handle an empty or non-existent inbox dir gracefully
- `#nosec` comments are not needed here since we are not running shell commands directly in the generator
- Follow existing error-wrapping style: `errors.Wrapf(ctx, err, ...)` — never `fmt.Errorf`
- The `containerChecker` field must use the `executor.ContainerChecker` interface type (not a concrete type)
- Test coverage ≥80% for changed code in `pkg/generator/`
</constraints>

<verification>
Run `make precommit` — must pass with exit code 0.
Spot-check: `grep -r "IsRunning" pkg/generator/` shows the container check in Generate.
Spot-check: `grep -r "Reattach" pkg/generator/` shows the reattach call.
Spot-check: `grep -r "containerChecker" pkg/generator/` shows the field and constructor param.
Spot-check: `grep -r "ContainerChecker" pkg/factory/` shows the checker being passed to CreateSpecGenerator.
</verification>
