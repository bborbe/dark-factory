---
status: completed
spec: [048-hide-git]
summary: Added HideGit bool to Config, hideGit bool field+parameter to dockerExecutor/NewDockerExecutor, buildHideGitArgs helper for .git masking logic, updated all call sites in factory.go and executor_test.go to pass false, and updated export_test.go helpers to accept and wire the new parameter.
container: dark-factory-299-spec-048-config-executor
dark-factory-version: v0.111.2
created: "2026-04-16T19:30:00Z"
queued: "2026-04-16T19:14:05Z"
started: "2026-04-16T19:14:07Z"
completed: "2026-04-16T19:26:11Z"
branch: dark-factory/hide-git
---

<summary>
- A new `hideGit bool` field is added to the `Config` struct so projects can opt in to `.git` masking via `hideGit: true` in `.dark-factory.yaml`
- A new `hideGit bool` field is added to the `dockerExecutor` struct; `NewDockerExecutor` gains a matching trailing parameter
- When `hideGit` is true, `buildDockerCommand` inspects `projectRoot/.git` before launching the container and adds exactly one conditional Docker flag based on what it finds
- Directory `.git` → anonymous volume (`-v /workspace/.git`); file `.git` (worktree pointer) → `/dev/null` bind (`-v /dev/null:/workspace/.git`); missing `.git` → no flag added
- `os.Stat` permission errors are logged at debug level and silently skipped — they never abort container launch
- When `hideGit` is false (the default), `buildDockerCommand` is byte-for-byte identical to the current version — no regressions for existing workflows
- All `export_test.go` helpers are updated to accept and wire the new `hideGit bool` parameter, keeping test helpers in sync with production constructors
- All existing call sites in `factory.go` and `executor_test.go` are updated to pass `false`, maintaining the status quo until the factory-wiring prompt wires the real value
- `make precommit` passes with existing tests unchanged
</summary>

<objective>
Add the `hideGit bool` plumbing through config → executor → docker command so that the mount-masking behavior is implementable and independently testable. The factory wiring that makes `worktree` always set `hideGit=true` is handled in the next prompt; this prompt delivers the foundation.
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, Counterfeiter).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read before editing:
- `pkg/config/config.go` — `Config` struct, `Defaults()`, `Validate()` (add `HideGit bool` field)
- `pkg/executor/executor.go` — `NewDockerExecutor`, `dockerExecutor` struct, `buildDockerCommand` (the mount logic goes right before `args = append(args, e.containerImage)` at the end of the extraMounts loop)
- `pkg/executor/export_test.go` — all exported helpers that mirror `dockerExecutor` constructor; must be kept in sync with the production struct
- `pkg/factory/factory.go` — `createDockerExecutor` helper (~line 527) and `CreateSpecGenerator` (~line 448); both call `executor.NewDockerExecutor` and must be updated to compile
- `pkg/executor/executor_test.go` — existing call sites for `BuildDockerCommandForTest` (~lines 373, 1538, 1570) and `NewDockerExecutorWithRunnerForTest` (~lines 1127, 1293, 1391) must each gain a trailing `false` argument
</context>

<requirements>

## 1. Add `HideGit bool` to `pkg/config/config.go`

In the `Config` struct, add the new field immediately after the `Worktree bool` field:

```go
HideGit  bool `yaml:"hideGit,omitempty"`
```

`omitempty` ensures the field does not appear in `dark-factory config` output when false (matching the behavior of `Worktree`).

Do NOT add `HideGit` to `Defaults()` — its zero value (`false`) is the correct default. Do NOT add any validation for this field — it is a plain boolean with no invalid values.

## 2. Add `hideGit bool` field to `dockerExecutor` and update `NewDockerExecutor`

### 2a. Struct field

In `pkg/executor/executor.go`, add `hideGit bool` to the `dockerExecutor` struct as the last field (after `formatter formatter.Formatter`):

```go
type dockerExecutor struct {
    containerImage        string
    projectName           string
    model                 string
    netrcFile             string
    gitconfigFile         string
    env                   map[string]string
    extraMounts           []config.ExtraMount
    claudeDir             string
    commandRunner         commandRunner
    maxPromptDuration     time.Duration
    currentDateTimeGetter libtime.CurrentDateTimeGetter
    formatter             formatter.Formatter
    hideGit               bool   // NEW: mask /workspace/.git from the container
}
```

### 2b. Update `NewDockerExecutor` signature

Add `hideGit bool` as the **last parameter** (after `fmtr formatter.Formatter`):

```go
func NewDockerExecutor(
    containerImage string,
    projectName string,
    model string,
    netrcFile string,
    gitconfigFile string,
    env map[string]string,
    extraMounts []config.ExtraMount,
    claudeDir string,
    maxPromptDuration time.Duration,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    fmtr formatter.Formatter,
    hideGit bool,
) Executor {
    return &dockerExecutor{
        containerImage:        containerImage,
        projectName:           projectName,
        model:                 model,
        netrcFile:             netrcFile,
        gitconfigFile:         gitconfigFile,
        env:                   env,
        extraMounts:           extraMounts,
        claudeDir:             claudeDir,
        commandRunner:         &defaultCommandRunner{},
        maxPromptDuration:     maxPromptDuration,
        currentDateTimeGetter: currentDateTimeGetter,
        formatter:             fmtr,
        hideGit:               hideGit,
    }
}
```

**FREEZE all other fields and logic in `NewDockerExecutor`** — only add the parameter and set the field.

## 3. Implement mount logic in `buildDockerCommand`

In `pkg/executor/executor.go`, inside `buildDockerCommand`, add the following block **immediately before** the line `args = append(args, e.containerImage)` (which is the very last append before the `exec.CommandContext` call):

```go
if e.hideGit {
    gitPath := filepath.Join(projectRoot, ".git")
    fi, statErr := os.Stat(gitPath)
    if statErr == nil {
        if fi.IsDir() {
            // Normal repo or clone: mask host directory with an anonymous Docker volume.
            // Docker creates an empty volume at this path, hiding the host .git contents.
            args = append(args, "-v", "/workspace/.git")
        } else {
            // Worktree pointer file or submodule: bind /dev/null over the pointer file.
            // Git inside the container fails cleanly as "not a repository" instead of
            // "gitdir not found" (dangling pointer error).
            args = append(args, "-v", "/dev/null:/workspace/.git")
        }
    } else if !os.IsNotExist(statErr) {
        // Permission error or other unexpected failure: skip masking, log at debug.
        slog.Debug("hideGit: stat .git failed, skipping mount mask", "error", statErr)
    }
    // If .git is missing entirely, no mount is added — there is nothing to hide.
}
```

This block must appear **after** the existing `extraMounts` loop (which ends with `args = append(args, "-v", mount)`) and **before** `args = append(args, e.containerImage)`. No other lines in `buildDockerCommand` are changed.

**FREEZE all other mount/flag lines in `buildDockerCommand`** — only add this one conditional block.

## 4. Update `pkg/executor/export_test.go`

### 4a. Update `NewDockerExecutorWithRunnerForTest`

Add `hideGit bool` as the **last parameter** (after `fmtr formatter.Formatter`) and wire it into the struct:

```go
func NewDockerExecutorWithRunnerForTest(
    containerImage string,
    projectName string,
    model string,
    netrcFile string,
    gitconfigFile string,
    env map[string]string,
    extraMounts []config.ExtraMount,
    claudeDir string,
    maxPromptDuration time.Duration,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    runner CommandRunnerForTest,
    fmtr formatter.Formatter,
    hideGit bool,
) Executor {
    return &dockerExecutor{
        containerImage:        containerImage,
        projectName:           projectName,
        model:                 model,
        netrcFile:             netrcFile,
        gitconfigFile:         gitconfigFile,
        env:                   env,
        extraMounts:           extraMounts,
        claudeDir:             claudeDir,
        commandRunner:         runner,
        maxPromptDuration:     maxPromptDuration,
        currentDateTimeGetter: currentDateTimeGetter,
        formatter:             fmtr,
        hideGit:               hideGit,
    }
}
```

### 4b. Update `BuildDockerCommandForTest`

Add `hideGit bool` as the **last parameter** and wire it into the constructed `dockerExecutor`:

```go
func BuildDockerCommandForTest(
    ctx context.Context,
    containerImage string,
    projectName string,
    model string,
    netrcFile string,
    gitconfigFile string,
    env map[string]string,
    extraMounts []config.ExtraMount,
    containerName string,
    promptFilePath string,
    projectRoot string,
    claudeConfigDir string,
    promptBaseName string,
    home string,
    hideGit bool,
) *exec.Cmd {
    e := &dockerExecutor{
        containerImage: containerImage,
        projectName:    projectName,
        model:          model,
        netrcFile:      netrcFile,
        gitconfigFile:  gitconfigFile,
        env:            env,
        extraMounts:    extraMounts,
        hideGit:        hideGit,
    }
    return e.buildDockerCommand(
        ctx,
        containerName,
        promptFilePath,
        projectRoot,
        claudeConfigDir,
        promptBaseName,
        home,
    )
}
```

## 5. Update all call sites in `pkg/factory/factory.go`

### 5a. Update `createDockerExecutor` helper

Add `hideGit bool` as the **last parameter** and pass it through to `executor.NewDockerExecutor`:

```go
func createDockerExecutor(
    containerImage string,
    projectName string,
    model string,
    netrcFile string,
    gitconfigFile string,
    env map[string]string,
    extraMounts []config.ExtraMount,
    claudeDir string,
    maxPromptDuration time.Duration,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    hideGit bool,
) executor.Executor {
    return executor.NewDockerExecutor(
        containerImage, projectName, model, netrcFile, gitconfigFile, env, extraMounts, claudeDir,
        maxPromptDuration, currentDateTimeGetter, formatter.NewFormatter(),
        hideGit,
    )
}
```

### 5b. Update the call to `createDockerExecutor` (~line 605)

There is exactly one call site of `createDockerExecutor` inside `factory.go`. Pass `false` as the trailing `hideGit` argument:

```go
createDockerExecutor(
    containerImage, projectName, model, netrcFile,
    gitconfigFile, env, extraMounts, claudeDir, maxPromptDuration,
    currentDateTimeGetter,
    false, // hideGit — wired correctly in prompt 2 (spec-048-factory-docs)
),
```

### 5c. Update the direct call in `CreateSpecGenerator` (~line 455)

Pass `false` as the new trailing `hideGit` argument to `executor.NewDockerExecutor`:

```go
executor.NewDockerExecutor(
    containerImage,
    project.Name(cfg.ProjectName),
    cfg.Model,
    cfg.NetrcFile,
    cfg.GitconfigFile,
    cfg.Env,
    cfg.ExtraMounts,
    cfg.ResolvedClaudeDir(),
    cfg.ParsedMaxPromptDuration(),
    currentDateTimeGetter,
    formatter.NewFormatter(),
    false, // hideGit — spec generators never need .git masking
),
```

**Do not miss this call site** — skipping it will break compilation.

## 6. Update call sites in `pkg/executor/executor_test.go`

There are **three** calls to `NewDockerExecutorWithRunnerForTest` (approximately lines 1127, 1293, 1391) and **three** calls to `BuildDockerCommandForTest` (approximately lines 373, 1538, 1570). Each must gain a trailing `false` argument.

For `NewDockerExecutorWithRunnerForTest` calls — add `false` after the existing last argument (`fakeFormatter` or equivalent):
```go
return executor.NewDockerExecutorWithRunnerForTest(
    // ... all existing arguments unchanged ...
    fakeFormatter,
    false, // hideGit
)
```

For `BuildDockerCommandForTest` calls — add `false` after the existing last argument (`home` or equivalent):
```go
executor.BuildDockerCommandForTest(
    // ... all existing arguments unchanged ...
    home,
    false, // hideGit
)
```

Find all call sites precisely by running `grep -n "BuildDockerCommandForTest\|NewDockerExecutorWithRunnerForTest" pkg/executor/executor_test.go` before editing — do not guess line numbers.

## 7. Run `make test` to verify

After all changes, run `make test` in `/workspace`. All existing tests must pass. Do not add new tests in this prompt — the comprehensive test suite is in prompt 2.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT add new tests in this prompt (tests come in prompt 2)
- The `HideGit` field must NOT appear in `Defaults()` — its zero value is the correct default
- Do NOT add validation for `HideGit` in `Config.Validate()` — it is a plain boolean
- **FREEZE `buildDockerCommand`** except for the ONE new conditional block; every existing mount and flag line stays byte-for-byte identical
- Wrap all non-nil errors with `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors` (no new error paths expected in this prompt)
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Existing tests must still pass — every changed call site gets `false` as the new trailing argument
- `filepath.Join(projectRoot, ".git")` is the correct way to construct the `.git` path — do NOT hardcode `/workspace/.git` on the host side
- Use `os.IsNotExist(statErr)` to distinguish "missing" from "permission error" in the stat branch
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "hideGit" pkg/executor/executor.go` — three occurrences: struct field, constructor initializer, and the conditional in `buildDockerCommand`
2. `grep -n "hideGit" pkg/executor/export_test.go` — two occurrences: one in each helper function
3. `grep -n "hideGit\|HideGit" pkg/config/config.go` — one occurrence: the struct field
4. `grep -n "hideGit\|false.*hideGit\|hideGit.*false" pkg/factory/factory.go` — one occurrence in `createDockerExecutor` body, one in the call site, one in `CreateSpecGenerator`
5. `grep -c "BuildDockerCommandForTest\|NewDockerExecutorWithRunnerForTest" pkg/executor/executor_test.go` — returns 6; each call has been updated with trailing `false`
</verification>
