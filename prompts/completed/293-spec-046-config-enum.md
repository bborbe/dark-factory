---
status: completed
spec: [046-workflow-enum-with-worktree-mode]
summary: 'Expanded workflow enum to four values (direct/branch/worktree/clone), inverted deprecation so workflow is primary and worktree: bool is legacy, rewrote loader legacy-mapping with Compatibility Matrix, added workflow: direct + pr: true validation, updated all tests to match new behavior, and added new table-driven tests for all six matrix rows.'
container: dark-factory-293-spec-046-config-enum
dark-factory-version: v0.111.2
created: "2026-04-16T12:00:00Z"
queued: "2026-04-16T15:00:45Z"
started: "2026-04-16T15:00:47Z"
completed: "2026-04-16T15:20:04Z"
branch: dark-factory/workflow-enum-with-worktree-mode
---

<summary>
- The `workflow` enum in `.dark-factory.yaml` gains four valid values: `direct`, `branch`, `worktree`, `clone`
- The old two-value enum (`direct`/`pr`) is superseded; `workflow: pr` is mapped forward to `workflow: clone, pr: true` at load time with a deprecation notice
- The legacy `worktree: bool` field is deprecated; the loader maps it to the new `workflow` enum using the six-row Compatibility Matrix, logging `slog.Info` or `slog.Warn` as specified
- `workflow: direct` with `pr: true` is rejected at config-load time with a clear error message
- The presence of both `workflow` (new value) and `worktree: bool` in the same file is not a hard error; `workflow` wins and a `slog.Warn` is emitted naming `worktree` as ignored
- After loading, `cfg.Worktree` is zeroed out so `dark-factory config` output shows only the resolved `workflow` and `pr` fields
- All six Compatibility Matrix rows and all new validation branches are covered by table-driven unit tests
- `make precommit` passes
</summary>

<objective>
Widen the `workflow` enum from two values to four (`direct`, `branch`, `worktree`, `clone`), invert the deprecation direction in the config loader so that `workflow` is now the primary field and `worktree: bool` is the legacy field, and add validation for the new enum values and the `workflow: direct, pr: true` conflict. No processor or executor changes are in scope — only the config layer.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.
Read `go-enum-type-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `docs/workflows.md` for the authoritative description of each workflow mode.

Files to read before editing:
- `pkg/config/workflow.go` — current enum with `WorkflowDirect` and `WorkflowPR`
- `pkg/config/loader.go` — full file; the legacy mapping at lines 124–149 must be replaced
- `pkg/config/config.go` — `Config` struct, `Defaults()`, `Validate()` method
- `pkg/config/loader_test.go` (if it exists) — understand existing test structure; otherwise look at the test suite file in `pkg/config/`
</context>

<requirements>

## 1. Expand `pkg/config/workflow.go`

Replace the existing constants and `AvailableWorkflows` slice with the following:

```go
// Workflow values — use these in .dark-factory.yaml.
const (
    WorkflowDirect   Workflow = "direct"
    WorkflowBranch   Workflow = "branch"
    WorkflowWorktree Workflow = "worktree"
    WorkflowClone    Workflow = "clone"

    // WorkflowPR is the legacy enum value kept for parsing only.
    // The loader maps it to WorkflowClone + pr: true before validation.
    // Do not use this constant in new code.
    WorkflowPR Workflow = "pr"
)

// AvailableWorkflows contains the four valid workflow values for new configs.
// WorkflowPR ("pr") is intentionally excluded — it is legacy and mapped at load time.
var AvailableWorkflows = Workflows{WorkflowDirect, WorkflowBranch, WorkflowWorktree, WorkflowClone}
```

Update `Validate` to:
1. Accept all four values in `AvailableWorkflows` as valid.
2. Remove the special-case error for `"worktree"` (that string is no longer an enum value — it is `WorkflowWorktree = "worktree"` now and IS valid).
3. For any other unknown value (including typos), return the existing error:
   `errors.Wrapf(ctx, validation.Error, "unknown workflow %q, valid values: %s", w, strings.Join(...))`
   (The error message MUST list the four valid values so the user knows what to type.)

**Do not validate `WorkflowPR` here** — that value is mapped to `WorkflowClone` by the loader before `Validate` is ever called.

## 2. Rewrite the legacy-mapping section in `pkg/config/loader.go`

The current loader treats `workflow` as deprecated and `pr`/`worktree` booleans as current. This is inverted. Replace lines 124–149 (the conflict-detection block and the deprecation-mapping switch) with the new logic described below.

The loader's `Load` method must perform these steps in order after calling `mergePartial`:

### Step A — `workflow: pr` legacy enum mapping

If `partial.Workflow != nil` AND `*partial.Workflow == WorkflowPR`:
- Set `cfg.Workflow = WorkflowClone`, `cfg.PR = true`
- Emit `slog.Info("'workflow: pr' is deprecated; use 'workflow: clone' with 'pr: true' instead", "resolved", "workflow: clone, pr: true")`
- Do NOT apply Steps B or C (PR and workflow are already resolved); still fall through to Step E (which always runs).

### Step B — new `workflow` value alongside legacy `worktree: bool`

If `partial.Workflow != nil` AND `*partial.Workflow != WorkflowPR` AND `partial.Worktree != nil`:
- `workflow` wins (already merged into `cfg.Workflow`).
- Emit `slog.Warn("'worktree' is ignored when 'workflow' is set; remove 'worktree' from .dark-factory.yaml", "workflow", cfg.Workflow)`
- `cfg.Worktree` is not touched here (it was already merged; it will be zeroed in Step E).

### Step C — legacy `worktree: bool` mapping (no `workflow` field)

If `partial.Workflow == nil` AND `partial.Worktree != nil`:
Apply the Compatibility Matrix:

| `cfg.Worktree` | `cfg.PR` | Resolved `cfg.Workflow` | Resolved `cfg.PR` | Log level | Extra action |
|---|---|---|---|---|---|
| `false` | `false` | `WorkflowDirect` | `false` | `slog.Info` | — |
| `false` | `true`  | `WorkflowBranch` | `true`  | `slog.Info` | — |
| `true`  | `true`  | `WorkflowClone`  | `true`  | `slog.Info` | — |
| `true`  | `false` | `WorkflowClone`  | `true`  | `slog.Warn` | Override message (see below) |

For all four rows emit a deprecation notice:
- Rows 1–3: `slog.Info("'worktree' is deprecated in .dark-factory.yaml; use 'workflow' instead", "resolved_workflow", cfg.Workflow, "resolved_pr", cfg.PR)`
- Row 4 additionally (WARN, separate call): `slog.Warn("'worktree: true, pr: false' overrides pr to true for compatibility; set 'pr: true' explicitly to silence this warning")`

### Step D — zero out `cfg.Worktree` (always runs)

Unconditionally — after Steps A, B, or C, regardless of which ran — set `cfg.Worktree = false` so the field does not appear in `dark-factory config` output (the field has `yaml:"worktree,omitempty"`). The resolved state is fully captured by `cfg.Workflow` + `cfg.PR`. This must run even when Step A took effect (legacy enum `workflow: pr` path).

Note: `workflow: direct` with `pr: true` rejection is NOT a loader step — it is a `Validate()` rule (see requirement 3). The loader calls `cfg.Validate(ctx)` at the end, which catches it.

### Step E — remove the old conflict-detection block entirely

Delete these lines (approximately 124–129 in the current file):
```go
// Conflict detection: cannot set both old workflow and new boolean flags
if partial.Workflow != nil && (partial.PR != nil || partial.Worktree != nil) {
    return Config{}, errors.Errorf(...)
}
```
This block is replaced by Steps A–C above. Setting `workflow` alongside `pr: bool` is explicitly NOT an error — `pr` is an orthogonal delivery flag, not a conflict.

## 3. Add `workflow: direct, pr: true` validation to `pkg/config/config.go`

In the `Validate()` method, add a new `validation.Name` entry **before** the `autoMerge` check:

```go
validation.Name("workflow", validation.HasValidationFunc(func(ctx context.Context) error {
    if c.Workflow == WorkflowDirect && c.PR {
        return errors.Errorf(ctx, "workflow 'direct' is incompatible with pr: true (no feature branch exists to open a PR from)")
    }
    return nil
})),
```

Do NOT modify the existing `autoMerge` validation. It already requires `pr: true`, and the new `workflow: direct + pr: true` rejection above already prevents the forbidden combination transitively. Leave the `autoMerge` block as-is.

## 4. Unit tests

Find or create the test file for the config loader (likely `pkg/config/loader_test.go` or a `*_suite_test.go` + individual `_test.go` files). Add the following test groups following existing Ginkgo/Gomega conventions.

### 4a. Compatibility Matrix — table-driven test

A `DescribeTable("legacy worktree: bool mapping", ...)` covering all six rows from the Compatibility Matrix in the spec (including the two loaders-internal rows: `workflow: pr` legacy enum, and new-workflow-with-worktree):

| Input (YAML fields) | Expected `cfg.Workflow` | Expected `cfg.PR` | Expected `cfg.Worktree` |
|---|---|---|---|
| `worktree: false, pr: false` | `direct` | `false` | `false` |
| `worktree: false, pr: true` | `branch` | `true` | `false` |
| `worktree: true, pr: true` | `clone` | `true` | `false` |
| `worktree: true, pr: false` | `clone` | `true` | `false` |
| `workflow: pr` (no booleans) | `clone` | `true` | `false` |
| `workflow: worktree, worktree: true` | `worktree` | `false` (as-set) | `false` |

For each row, load a synthetic `.dark-factory.yaml` through a `fileLoader` (write the YAML to a temp file, override `configPath`). Assert the resolved fields match, and that `cfg.Worktree == false` in every case.

For the Row 4 case (`worktree: true, pr: false`), additionally assert that the log output contains the warn substring `"worktree: true, pr: false"` (use the `slog.SetDefault` + `bytes.Buffer` capture pattern from `pkg/processor/processor_test.go`).

Also add a NEGATIVE log-capture test: loading `workflow: worktree, pr: true` (new-style, no legacy fields) produces NEITHER `slog.Info` nor `slog.Warn` about deprecation. `pr` alongside `workflow` must never trigger a warning.

### 4b. Validation tests

`Describe("Workflow.Validate", ...)`:
- `direct` → nil
- `branch` → nil
- `worktree` → nil
- `clone` → nil
- `"typo"` → non-nil error containing `"typo"` and listing all four valid values
- `""` → non-nil error (empty string is unknown)

`Describe("Config.Validate workflow+pr", ...)`:
- `workflow: direct, pr: false` → nil
- `workflow: direct, pr: true` → non-nil error containing `"direct"` and `"pr: true"`
- `workflow: branch, pr: true` → nil
- `workflow: clone, pr: false` → nil

### 4c. Sibling config compatibility test

`Describe("sibling project configs", ...)` — for each of the following YAML snippets (representing the current config of known sibling projects), load through the fileLoader and assert the resolved behavior matches the Compatibility Matrix:

```yaml
# billomat / mdm / commerce — currently: worktree: true (no explicit pr)
worktree: true
```
Expected: `workflow: clone, pr: true`

```yaml
# projects using direct mode — currently: (no workflow field at all)
projectName: some-project
```
Expected: `workflow: direct, pr: false` (defaults)

```yaml
# projects using workflow: direct explicitly
workflow: direct
```
Expected: `workflow: direct, pr: false`

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT change the processor, executor, factory, or any file outside `pkg/config/` and its test files.
- The `WorkflowPR` constant must be preserved in `workflow.go` for parsing — the loader reads it from YAML and maps it. It is simply not in `AvailableWorkflows`.
- `Validate()` must NOT accept `WorkflowPR` as a valid value (the loader maps it away before Validate is called; if somehow it reaches Validate it should fail with "unknown workflow").
- Wrap all non-nil errors with `errors.Wrap` / `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- Error messages must be lowercase, no file paths in the message body.
- Existing tests must still pass.
- Do not touch `go.mod` / `go.sum` / `vendor/`.
- Do not log secrets or file paths.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `echo 'worktree: true' > /tmp/test-cfg.yaml && dark-factory config` equivalent: load that YAML through the loader and assert `workflow: clone, pr: true`.
2. `echo 'workflow: typo' > /tmp/test-cfg.yaml`: loader returns a validation error listing the four valid values.
3. `echo 'workflow: direct\npr: true' > /tmp/test-cfg.yaml`: loader returns a validation error about direct + pr incompatibility.
</verification>
