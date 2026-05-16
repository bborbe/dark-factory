---
status: committing
spec: [080-container-naming-project-role-prefix]
summary: Added optional `project:` field to Config with validation, threaded resolved project name into spec-generation container naming across generator, lifecycle, health_check, and factory; renamed containers from dark-factory-gen-<spec> to <project>-gen-<spec> with legacy-compat probe in startup recovery.
container: dark-factory-388-spec-080-config-and-gen-naming
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-16T12:00:00Z"
queued: "2026-05-16T12:15:33Z"
started: "2026-05-16T12:15:34Z"
branch: dark-factory/container-naming-project-role-prefix
---

<summary>
- Projects can now set a `project:` field in `.dark-factory.yaml` to control the Docker container name prefix; absent field continues to use the git root directory basename
- Setting `project: ""` (empty) or `project: "   "` (whitespace-only) is rejected at daemon startup with a clear error naming the field — no silent fallback
- Spec-generation containers are renamed from `dark-factory-gen-<spec-basename>` to `<project>-gen-<spec-basename>`
- The hardcoded string `"dark-factory-gen-"` is removed from all production code in `pkg/`; test files may retain it only inside assertions that it is NOT produced
- Operators running multiple dark-factory projects can now filter generation containers by project: `docker ps --filter name=maintainer-gen-`
- The `project.Resolve()` fallback chain is unaffected — when neither `project:` nor `projectName:` is set, the git root directory basename is used
- No existing `.dark-factory.yaml` file requires a change — all existing configs continue to work unchanged
</summary>

<objective>
Rename spec-generation Docker containers from the hardcoded `dark-factory-gen-<spec>` schema to the project-aware `<project>-gen-<spec>` schema. Introduce an optional `project:` YAML field in Config that overrides the project name used in container names, with validation that rejects empty/whitespace values. Thread the resolved project name into the spec generator and the restart-recovery code paths (lifecycle and health check).
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, Counterfeiter, no fmt.Errorf).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-validation-framework-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` for which test types to write for each code change.

Files to read in full before editing:
- `pkg/config/config.go` — Config struct (lines ~84–125), `Defaults()`, `Validate()` (lines ~169–244); the import block; `strings` is already imported
- `pkg/config/config_test.go` — read 50 lines around line 54 to understand the Validate test pattern (Config struct literal + `cfg.Validate(ctx)`)
- `pkg/generator/generator.go` — full file; focus on `dockerSpecGenerator` struct (lines ~67–83), `NewSpecGenerator` constructor (lines ~33–65), and the container-name derivation at line ~113
- `pkg/runner/lifecycle.go` — `resumeOrResetGenerating` package-level function (lines ~219–241) and `resumeOrResetGeneratingEntry` (lines ~244–278)
- `pkg/runner/runner.go` — `runner` struct (lines ~100–128) and the `r.resumeOrResetGenerating` method (lines ~283–291)
- `pkg/runner/health_check.go` — `runHealthCheckLoop` (lines ~28–57) and `checkGeneratingSpecs` (lines ~236–288)
- `pkg/factory/factory.go` — the three occurrences of `project.Resolve(cfg.ProjectName)` (lines ~313, ~481, ~612) and `CreateSpecGenerator` (lines ~601–638)
- `pkg/runner/runner_test.go` — lines 960–1020 to understand how `resumeOrResetGenerating` is tested (package-level function call pattern)
</context>

<requirements>

## 1. Add `Project *string` field to Config in `pkg/config/config.go`

In the `Config` struct, add the new field immediately after `ProjectName string yaml:"projectName"`:

```go
ProjectName            string              `yaml:"projectName"`
Project                *string             `yaml:"project,omitempty"`
```

Using `*string` (pointer) allows YAML unmarshaling to distinguish between "field absent" (nil) and "field explicitly set to empty string" (&""), which is the only way to reject `project: ""` at validation time.

Do NOT add `Project` to `Defaults()` — its zero value (nil) is the correct default meaning "not set".

## 2. Add validation for the `project` field in `Config.Validate()`

In `Config.Validate()`, add the following validation entry to the `validation.All{}` slice. Place it immediately after the `validation.Name("workflow", c.Workflow)` entry:

```go
validation.Name("project", validation.HasValidationFunc(func(ctx context.Context) error {
    if c.Project == nil {
        return nil // not set — project.Resolve() handles the fallback chain
    }
    if strings.TrimSpace(*c.Project) == "" {
        return errors.Errorf(ctx, "project must not be empty or whitespace-only when set")
    }
    return nil
})),
```

## 3. Add `ResolvedProjectOverride()` method to Config

Add a new method to Config immediately below the `Validate()` method:

```go
// ResolvedProjectOverride returns the effective project name override string for project.Resolve().
// The explicit `project:` field takes precedence over the legacy `projectName:` field.
// Returns an empty string when neither is set; project.Resolve() handles the git-root fallback.
func (c Config) ResolvedProjectOverride() string {
    if c.Project != nil && strings.TrimSpace(*c.Project) != "" {
        return *c.Project
    }
    return c.ProjectName
}
```

## 4. Update `pkg/factory/factory.go`: replace `project.Resolve(cfg.ProjectName)` with `project.Resolve(cfg.ResolvedProjectOverride())`

Run `grep -n "project.Resolve(cfg.ProjectName)" pkg/factory/factory.go` to find all occurrences (expect 3). Replace each one:

```go
// OLD:
projectName := project.Resolve(cfg.ProjectName)
// NEW:
projectName := project.Resolve(cfg.ResolvedProjectOverride())
```

```go
// OLD (inside CreateSpecGenerator):
project.Resolve(cfg.ProjectName).String(),
// NEW:
project.Resolve(cfg.ResolvedProjectOverride()).String(),
```

Do not change any other logic in factory.go in this prompt — the status-checker threading is handled in prompt 2.

## 5. Add `projectName project.Name` to `dockerSpecGenerator` in `pkg/generator/generator.go`

### 5a. Import `pkg/project`

Add the import to the existing import block:

```go
"github.com/bborbe/dark-factory/pkg/project"
```

### 5b. Add `projectName` field to `dockerSpecGenerator` struct

Add as the FIRST field in the struct (before `executor`):

```go
type dockerSpecGenerator struct {
    projectName            project.Name    // NEW
    executor               executor.Executor
    // ... all existing fields unchanged ...
}
```

### 5c. Add `projectName project.Name` as the LAST parameter of `NewSpecGenerator`

The current last parameter is `queueDir string`. Add after it:

```go
func NewSpecGenerator(
    executor executor.Executor,
    containerChecker executor.ContainerChecker,
    inboxDir string,
    completedDir string,
    specsDir string,
    logDir string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    slugMigrator slugmigrator.Migrator,
    generateCommand string,
    additionalInstructions string,
    maxPromptDuration time.Duration,
    pm PromptManager,
    autoApprovePrompts bool,
    queueDir string,
    projectName project.Name,    // NEW — last parameter
) SpecGenerator {
    return &dockerSpecGenerator{
        projectName:            projectName,    // NEW
        executor:               executor,
        // ... all existing fields unchanged ...
    }
}
```

**FREEZE all other logic in `NewSpecGenerator`.**

## 6. Update container name derivation in `Generate` (generator.go ~line 113)

Replace the hardcoded `dark-factory-gen-` prefix:

```go
// OLD:
containerName := "dark-factory-gen-" + specBasename

// NEW:
containerName := string(prompt.ContainerName(string(g.projectName) + "-gen-" + specBasename).Sanitize())
```

`prompt.ContainerName.Sanitize()` is already the established sanitization path and `prompt` is already imported in this file. **FREEZE all other lines in `Generate`.**

## 7. Update `CreateSpecGenerator` in `pkg/factory/factory.go` to pass `projectName`

In `CreateSpecGenerator`, the call to `generator.NewSpecGenerator` currently ends with `cfg.Prompts.InProgressDir`. Add the resolved project name as the final argument:

```go
return generator.NewSpecGenerator(
    executor.NewDockerExecutor(
        containerImage,
        project.Resolve(cfg.ResolvedProjectOverride()).String(),  // already updated in req 4
        // ... all existing executor args unchanged ...
    ),
    executor.NewDockerContainerChecker(currentDateTimeGetter),
    cfg.Prompts.InboxDir,
    cfg.Prompts.CompletedDir,
    cfg.Specs.InboxDir,
    cfg.Specs.LogDir,
    currentDateTimeGetter,
    slugMigrator,
    cfg.GenerateCommand,
    cfg.AdditionalInstructions,
    cfg.ParsedMaxPromptDuration(),
    promptManager,
    cfg.AutoApprovePrompts,
    cfg.Prompts.InProgressDir,
    project.Resolve(cfg.ResolvedProjectOverride()),    // NEW: last argument
)
```

Confirm the call site is complete using: `grep -A30 "return generator.NewSpecGenerator" pkg/factory/factory.go`

## 8. Thread `projectName` through `resumeOrResetGenerating` in `pkg/runner/lifecycle.go`

### 8a. Update the package-level `resumeOrResetGenerating` function signature

Add `projectName string` as the last parameter:

```go
func resumeOrResetGenerating(
    ctx context.Context,
    specsInProgressDir string,
    checker executor.ContainerChecker,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    projectName string,    // NEW
) error {
```

Update the loop body to pass `projectName` to `resumeOrResetGeneratingEntry`:

```go
if err := resumeOrResetGeneratingEntry(ctx, specsInProgressDir, entry.Name(), checker, currentDateTimeGetter, projectName); err != nil {
```

### 8b. Update `resumeOrResetGeneratingEntry` function signature

Add `projectName string` as the last parameter:

```go
func resumeOrResetGeneratingEntry(
    ctx context.Context,
    specsInProgressDir string,
    name string,
    checker executor.ContainerChecker,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    projectName string,    // NEW
) error {
```

Update the container name derivation (currently line ~261) to probe BOTH the new and legacy prefixes so a legacy `dark-factory-gen-<spec>` container still running after upgrade is correctly resumed (spec 080 Failure Modes row 6 — "Resume logic uses the name recorded at spawn time"):

```go
// OLD:
containerName := "dark-factory-gen-" + specBasename

// NEW:
newName := projectName + "-gen-" + specBasename
legacyName := "dark-factory-gen-" + specBasename
running, err := checker.IsRunning(ctx, newName)
if err != nil {
    return errors.Wrapf(ctx, err, "check container running %s", newName)
}
containerName := newName
if !running {
    legacyRunning, lerr := checker.IsRunning(ctx, legacyName)
    if lerr != nil {
        return errors.Wrapf(ctx, lerr, "check legacy container running %s", legacyName)
    }
    if legacyRunning {
        containerName = legacyName
    }
}
```

If the existing code already calls `checker.IsRunning` once and branches on the result, restructure the surrounding logic so the existing branch is reused — do NOT duplicate the "container is gone, reset to approved" path. Read `resumeOrResetGeneratingEntry` in full before editing to find the existing seam. The literal string `"dark-factory-gen-"` may appear ONLY here in `pkg/runner/lifecycle.go` (legacy-probe) and is the one exception to the constraint below; mark it with a comment `// legacy-compat: probe pre-spec-080 container name` so it is grep-recognizable as intentional.

**FREEZE all other logic in `resumeOrResetGeneratingEntry`.**

### 8c. Update `r.resumeOrResetGenerating` method in `pkg/runner/runner.go`

The method calls the package-level function. Add `r.projectName.String()` as the trailing argument:

```go
func (r *runner) resumeOrResetGenerating(ctx context.Context) error {
    return resumeOrResetGenerating(
        ctx,
        r.specsInProgressDir,
        r.containerChecker,
        r.currentDateTimeGetter,
        r.projectName.String(),    // NEW
    )
}
```

## 9. Thread `projectName` through `checkGeneratingSpecs` in `pkg/runner/health_check.go`

### 9a. Update `checkGeneratingSpecs` function signature

Add `projectName string` as the last parameter:

```go
func checkGeneratingSpecs(
    ctx context.Context,
    specsInProgressDir string,
    checker executor.ContainerChecker,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    projectName string,    // NEW
) error {
```

Update the container name derivation (currently line ~262):

```go
// OLD:
containerName := "dark-factory-gen-" + specBasename

// NEW:
containerName := projectName + "-gen-" + specBasename
```

### 9b. Update the call to `checkGeneratingSpecs` inside `runHealthCheckLoop`

`runHealthCheckLoop` already has `projectName string` as a parameter. Thread it through:

```go
// OLD:
if err := checkGeneratingSpecs(ctx, specsInProgressDir, checker, currentDateTimeGetter); err != nil {

// NEW:
if err := checkGeneratingSpecs(ctx, specsInProgressDir, checker, currentDateTimeGetter, projectName); err != nil {
```

## 10. Update test call sites for the new signatures

Audited call sites that MUST be updated (do not skip any):

1. `pkg/runner/export_test.go` — `CheckGeneratingSpecsForTest` wrapper must grow a `projectName string` parameter and forward it to `checkGeneratingSpecs`.
2. `pkg/runner/health_check_test.go:622` — asserts the legacy `"dark-factory-gen-001-myspec"` container name. Change to `"test-project-gen-001-myspec"` and pass `"test-project"` through the test wrapper.
3. `pkg/runner/runner_test.go:1047` — asserts the legacy `"dark-factory-gen-042-my-spec"` container name. Change to `"test-project-gen-042-my-spec"`; update the surrounding call to `resumeOrResetGenerating` (or its test wrapper) to pass `"test-project"` as the trailing argument.
4. `pkg/runner/runner_test.go` — any other call to `resumeOrResetGenerating` or `resumeOrResetGeneratingEntry`. Confirm with `grep -n "resumeOrResetGenerating\|resumeOrResetGeneratingEntry" pkg/runner/runner_test.go` after editing.
5. `pkg/generator/generator_test.go` — the existing `Equal("dark-factory-gen-...")` / `HavePrefix("dark-factory-gen-")` assertions (around L113 and L664 — confirm with `grep -n "dark-factory-gen-" pkg/generator/generator_test.go`). Update each to the new `<test-project>-gen-<spec>` form and pass `project.Name("test-project")` (or whatever fixture matches the surrounding test) into `NewSpecGenerator`.

Test files in `pkg/formatter/` (e.g. `formatter_test.go:142,154`) intentionally keep legacy strings as fixtures of pre-080 status entries — DO NOT change those.

## 11. Run `make test` and verify

`r.projectName` already exists on the `runner` struct (`pkg/runner/runner.go:116`) as `project.Name` — no struct change needed in req 8c, only the method body.

After all changes, run `make test` in `/workspace`. All tests must pass. Fix any compilation errors from missed call sites by grepping:

```bash
grep -rn "resumeOrResetGenerating\|resumeOrResetGeneratingEntry\|checkGeneratingSpecs\|NewSpecGenerator" pkg/ --include="*.go" | grep -v "_test.go"
```

## 12. Add tests

### 12a. Config validation tests (add to `pkg/config/config_test.go`)

Add a new `Describe("project field", ...)` block inside the existing `Describe("Validate", ...)` block. Use the same `Config` literal pattern already used in that file:

```go
Describe("project field", func() {
    validBase := config.Config{
        Workflow: config.WorkflowDirect,
        Prompts: config.PromptsConfig{
            InboxDir:      "prompts",
            InProgressDir: "prompts/in-progress",
            CompletedDir:  "prompts/completed",
            LogDir:        "prompts/log",
        },
        ContainerImage: pkg.DefaultContainerImage,
        Model:          "claude-sonnet-4-6",
        DebounceMs:     500,
    }

    It("succeeds when project field is absent (nil)", func() {
        cfg := validBase
        cfg.Project = nil
        Expect(cfg.Validate(ctx)).To(Succeed())
    })

    It("succeeds when project is set to a non-empty value", func() {
        cfg := validBase
        s := "maintainer"
        cfg.Project = &s
        Expect(cfg.Validate(ctx)).To(Succeed())
    })

    It("fails when project is set to empty string", func() {
        cfg := validBase
        s := ""
        cfg.Project = &s
        err := cfg.Validate(ctx)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("project"))
    })

    It("fails when project is whitespace-only", func() {
        cfg := validBase
        s := "   "
        cfg.Project = &s
        err := cfg.Validate(ctx)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("project"))
    })
})

Describe("ResolvedProjectOverride", func() {
    It("returns ProjectName when Project is nil", func() {
        cfg := config.Config{ProjectName: "legacy-name"}
        Expect(cfg.ResolvedProjectOverride()).To(Equal("legacy-name"))
    })

    It("returns *Project when Project is set and non-empty", func() {
        s := "maintainer"
        cfg := config.Config{ProjectName: "legacy-name", Project: &s}
        Expect(cfg.ResolvedProjectOverride()).To(Equal("maintainer"))
    })

    It("returns ProjectName when Project is nil and ProjectName is empty", func() {
        cfg := config.Config{}
        Expect(cfg.ResolvedProjectOverride()).To(Equal(""))
    })
})
```

### 12b. Generator container name test (add to `pkg/generator/generator_test.go`)

Find the existing test pattern for `Generate` (look for `FakeExecutor.ExecuteCallCount()` or similar). Add a test that verifies the container name passed to executor:

```go
Describe("Generate container name", func() {
    It("uses <project>-gen-<spec-basename> as container name", func() {
        fakeExecutor.ExecuteStub = func(ctx context.Context, content, logFile, containerName string) error {
            return nil
        }
        specPath := filepath.Join(specsDir, "031-my-spec.md")
        // write a minimal spec file with approved status
        // ... (follow existing test pattern for creating spec files)
        gen := generator.NewSpecGenerator(
            fakeExecutor,
            fakeContainerChecker,
            inboxDir,
            completedDir,
            specsDir,
            logDir,
            currentDateTimeGetter,
            fakeMigrator,
            "/generate",
            "",
            time.Minute,
            fakePromptManager,
            false,
            queueDir,
            project.Name("maintainer"),    // projectName
        )
        _ = gen.Generate(ctx, specPath)
        Expect(fakeExecutor.ExecuteCallCount()).To(Equal(1))
        _, _, _, containerName := fakeExecutor.ExecuteArgsForCall(0)
        Expect(containerName).To(Equal("maintainer-gen-031-my-spec"))
    })
})
```

Follow the existing test infrastructure in `generator_test.go` exactly — reuse existing fake setup, directory variables, and spec-file creation helpers. Do NOT invent new test helpers. For spec-file creation, copy the `os.WriteFile(specPath, []byte(...), 0644)` pattern from the nearest existing `Describe` block in `generator_test.go` — find with `grep -n 'os.WriteFile' pkg/generator/generator_test.go`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Container names MUST satisfy Docker's `[a-zA-Z0-9][a-zA-Z0-9_.-]*` rule. The `ContainerName.Sanitize()` call in req 6 handles this for the generator; lifecycle and health_check use the projectName verbatim (already sanitized by `project.Resolve()` which returns a basename)
- The `Project *string` field MUST NOT appear in `Defaults()` — nil is the correct default
- Do NOT add `HideGit`-style logic or any new sanitization rules — reuse existing `ContainerName.Sanitize()` from `pkg/prompt/`
- `SpecGenerator` interface (`Generate(ctx, specPath)`) MUST NOT change — only the constructor changes
- `Public Go API of pkg/generator/, pkg/processor/, pkg/executor/, pkg/runner/, and pkg/status/ MUST NOT change in exported symbols` — `NewSpecGenerator` is NOT exported at the interface level (it returns `SpecGenerator`), so adding a parameter is safe
- The grep pattern `"dark-factory-gen-"` must be absent from all non-test files under `pkg/` after this change
- Wrap all errors with `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Existing tests must still pass; fix all compilation errors from the signature changes before `make test`
- This prompt does NOT change `pkg/processor/` or `pkg/status/` — those are prompt 2
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -rn 'dark-factory-gen-' pkg/generator/ pkg/runner/health_check.go pkg/factory/ --include='*.go' | grep -v _test.go` — must return no lines (exit code 1). NOTE: `pkg/status/status.go` and the legacy-compat probe in `pkg/runner/lifecycle.go` still contain `dark-factory-gen-`; the status.go occurrence is removed in prompt 2, and the lifecycle.go probe is intentional (see req 8b).
2. `grep -n "Project \*string" pkg/config/config.go` — one occurrence in the Config struct
3. `grep -n "ResolvedProjectOverride" pkg/config/config.go` — one occurrence (the method definition)
4. `grep -n "projectName" pkg/generator/generator.go` — at least 3 occurrences (struct field, constructor param, container name line)
5. `grep -n "projectName" pkg/runner/lifecycle.go` — occurrences in `resumeOrResetGenerating` and `resumeOrResetGeneratingEntry`
6. `grep -n "projectName" pkg/runner/health_check.go` — occurrences in `checkGeneratingSpecs` and its call in `runHealthCheckLoop`
7. `grep -n "project.Resolve(cfg.ProjectName)" pkg/factory/factory.go` — must return no lines (all replaced)
</verification>
