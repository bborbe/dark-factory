---
status: completed
spec: [028-shared-branch-per-spec]
summary: Replaced workflow string enum with PR and Worktree boolean flags in Config, added deprecation mapping in loader, updated processor/factory/cmd to use pr bool, and updated all tests accordingly.
container: dark-factory-165-spec-028-config-booleans
dark-factory-version: v0.36.0-dirty
created: "2026-03-10T19:39:55Z"
queued: "2026-03-10T20:15:53Z"
started: "2026-03-10T20:15:56Z"
completed: "2026-03-10T20:29:32Z"
---
<summary>
- Dark-factory config supports two new boolean settings for controlling pull-request creation and worktree isolation, replacing the older workflow string
- Projects using the older workflow setting continue to work — dark-factory maps the old values to the new booleans and logs a deprecation warning at startup
- Setting both the old workflow and the new boolean settings in the same config file is a validation error with a clear message
- All internal checks that previously asked "is this a PR workflow?" now use the new boolean instead
- Existing projects with no explicit workflow setting see no behavior change
</summary>

<objective>
Replace the `workflow: direct|pr` string enum in `.dark-factory.yaml` with two explicit boolean flags `pr` (default false) and `worktree` (default false), while preserving backward compatibility with old configs via a deprecation mapping. All code that previously switched on `WorkflowPR` must switch on the new `PR` bool.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/config/config.go` — `Config` struct (~line 42), `Defaults()` (~line 68), `Validate()` (~line 99). Inside `Validate()`, find the `validation.Name("autoMerge", ...)` anonymous closure at ~line 136 that checks `c.Workflow != WorkflowPR`. Also find `validateAutoReview` method (~line 158).
Read `pkg/config/loader.go` — `partialConfig` (~line 53), `mergePartial()` (~line 112).
Read `pkg/config/workflow.go` — the `Workflow` type, `WorkflowDirect`, `WorkflowPR`, `AvailableWorkflows`.
Read `pkg/factory/factory.go` — `CreateRunner()` (~line 95), `CreateOneShotRunner()` (~line 138), and `CreatePromptVerifyCommand()` (~line 441): all three pass `cfg.Workflow` and need updating.
Read `pkg/processor/processor.go` — search for all `p.workflow == config.WorkflowPR` checks (~lines 420, 492, 541) and the `workflow config.Workflow` field in the struct.
Read `pkg/config/config_test.go` and `pkg/config/loader_test.go` — existing tests for validation and loading.
</context>

<requirements>
1. **Add `PR bool` and `Worktree bool` to `Config` struct** in `pkg/config/config.go`:
   ```go
   PR       bool `yaml:"pr,omitempty"`
   Worktree bool `yaml:"worktree,omitempty"`
   ```
   Place them after the existing `Workflow` field. Keep the `Workflow` field — it is not removed yet (backward compat).

2. **Update `Defaults()`** in `pkg/config/config.go`: add `PR: false, Worktree: false` (these are the default zero values; the explicit setting makes intent clear in documentation).

3. **Add `PR *bool` and `Worktree *bool` to `partialConfig`** in `pkg/config/loader.go` (use pointer so nil means "not set in YAML"):
   ```go
   PR       *bool `yaml:"pr"`
   Worktree *bool `yaml:"worktree"`
   ```

4. **Conflict detection in `Load()`** in `pkg/config/loader.go`: after `yaml.Unmarshal`, before `mergePartial`, check:
   ```go
   if partial.Workflow != nil && (partial.PR != nil || partial.Worktree != nil) {
       return Config{}, errors.Errorf(ctx, "config cannot set both 'workflow' and 'pr'/'worktree'; remove 'workflow' and use 'pr' and 'worktree' booleans instead")
   }
   ```

5. **Deprecation mapping in `Load()`** in `pkg/config/loader.go`: after `mergePartial`, if `partial.Workflow != nil`, log a deprecation warning and map the workflow value to the new booleans:
   ```go
   if partial.Workflow != nil {
       slog.Warn("'workflow' is deprecated in .dark-factory.yaml; replace with 'pr' and 'worktree' booleans",
           "workflow", cfg.Workflow)
       switch cfg.Workflow {
       case config.WorkflowDirect:
           cfg.PR = false
           cfg.Worktree = false
       case config.WorkflowPR:
           cfg.PR = true
           cfg.Worktree = true
       }
   }
   ```

6. **Merge PR and Worktree in `mergePartial()`** in `pkg/config/loader.go`:
   ```go
   if partial.PR != nil {
       cfg.PR = *partial.PR
   }
   if partial.Worktree != nil {
       cfg.Worktree = *partial.Worktree
   }
   ```

7. **Update the `autoMerge` validation closure** inside `Validate()` in `pkg/config/config.go` (the anonymous function at ~line 136 inside `validation.Name("autoMerge", ...)`): change `c.Workflow != WorkflowPR` to `!c.PR`:
   ```go
   if c.AutoMerge && !c.PR {
       return errors.Errorf(ctx, "autoMerge requires pr: true")
   }
   ```

8. **Update `validateAutoReview`** in `pkg/config/config.go`: change `c.Workflow != WorkflowPR` to `!c.PR`:
   ```go
   if c.Workflow != WorkflowPR { ... }  →  if !c.PR { ... }
   ```
   The error message should say `"autoReview requires pr: true"`.

9. **Update `processor` in `pkg/processor/processor.go`**: change the `workflow config.Workflow` field and all `p.workflow == config.WorkflowPR` checks to `p.pr bool` and `p.pr`:
   - Rename field `workflow config.Workflow` → `pr bool`
   - Update `NewProcessor()` (or equivalent constructor) to accept `pr bool` instead of `workflow config.Workflow`
   - Replace all `p.workflow == config.WorkflowPR` with `p.pr`

10. **Update `pkg/factory/factory.go`**: in `CreateRunner()` (~line 126), `CreateOneShotRunner()` (~line 180), and `CreatePromptVerifyCommand()` (~line 455), change `cfg.Workflow` argument to `cfg.PR`.

11. **Update `CreateProcessor()` signature** in `pkg/factory/factory.go` to accept `pr bool` instead of `workflow config.Workflow`. Thread it through to `processor.NewProcessor()`.

12. **Update `pkg/config/config_test.go`**: add tests for:
    - Config with `pr: true` passes validation
    - `autoMerge: true` with `pr: false` fails validation with message containing "pr: true"
    - `autoReview: true` with `pr: false` fails with message containing "pr: true"

13. **Update `pkg/config/loader_test.go`**: add tests for:
    - Config YAML with `workflow: direct` loads with `PR: false, Worktree: false` and logs deprecation (or just check the values)
    - Config YAML with `workflow: pr` loads with `PR: true, Worktree: true`
    - Config YAML with both `workflow: direct` and `pr: true` returns an error
    - Config YAML with `pr: true, worktree: true` loads correctly without deprecation
    - Config YAML with neither `workflow` nor `pr`/`worktree` loads with `PR: false, Worktree: false`

14. **Update processor tests** in `pkg/processor/processor_test.go`: replace all `config.WorkflowPR` arguments with `true` (the `pr bool` parameter) and `config.WorkflowDirect` (or empty) with `false`. Do a comprehensive find-and-replace since there are many occurrences.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- The `Workflow` field stays in `Config` — it is NOT removed (removing `workflow:` support is deferred to a future release)
- The `Workflow` type (`pkg/config/workflow.go`) and its constants are NOT modified
- Configs with no `pr`, `worktree`, or `workflow` field must default to `PR: false, Worktree: false` (direct behavior — no change)
- Existing tests must still pass after the processor signature change
- `make precommit` must pass
</constraints>

<verification>
```bash
# No WorkflowPR checks remaining in production code (excluding workflow.go and config.go's Workflow field/type)
grep -rn "WorkflowPR" pkg/ --include="*.go" | grep -v "workflow\.go" | grep -v "_test.go"
# Expected: only the deprecation mapping line in loader.go (WorkflowPR reference in the switch case)

make precommit
```
Must pass with no errors.
</verification>
