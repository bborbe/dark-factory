---
status: approved
spec: [080-container-naming-project-role-prefix]
created: "2026-05-16T12:00:00Z"
queued: "2026-05-16T12:15:33Z"
branch: dark-factory/container-naming-project-role-prefix
---

<summary>
- Execution containers are renamed from `<project>-<prompt-basename>` to `<project>-exec-<prompt-basename>`, adding a `-exec-` role infix that pairs visually with the `-gen-` infix from prompt 1
- `docker ps --filter name=maintainer-exec-` returns only maintainer execution containers; `docker ps --filter name=maintainer-` returns both gen and exec containers for that project
- The status checker's `docker ps --filter name=dark-factory-gen-` filter is replaced by a project-aware `<project>-gen-` filter so it correctly identifies the new generation container names
- `docs/configuration.md` documents the optional `project:` field introduced in prompt 1 with its default behavior and override semantics
- `CHANGELOG.md` includes a migration note warning external tooling that greps for `dark-factory-gen-` or `<project>-<prompt>` to update to the new schema
- All acceptance criteria for spec 080 are satisfied: `grep -rn 'dark-factory-gen-' pkg/ | grep -v _test.go` returns zero lines; unit tests for both gen and exec naming pass
</summary>

<objective>
Complete spec 080 by: (1) adding the `-exec-` role infix to execution container names in `pkg/processor/`, (2) making the status checker project-aware so it can filter for `<project>-gen-` containers in `docker ps`, and (3) documenting the `project:` config field in `docs/configuration.md` and adding a CHANGELOG migration note. Prompt 1 (spec-080-config-and-gen-naming) must be merged first — this prompt builds on its changes.
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, Counterfeiter, no fmt.Errorf).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` for which test types to write.

Files to read in full before editing:
- `pkg/processor/processor.go` — `computePromptMetadata` helper (lines ~497–506); ONLY this function changes in processor.go
- `pkg/status/status.go` — `checker` struct (lines ~83–96), `NewChecker` constructor (lines ~99–127), `populateGeneratingSpec` method (lines ~538–578)
- `pkg/factory/factory.go` — `createStatusChecker` function (lines ~678–709) and its three call sites (find with `grep -n "createStatusChecker" pkg/factory/factory.go`); also `CreateServer` function signature (lines ~968–992)
- `pkg/processor/processor_test.go` — lines 385–400 to understand how container name assertions are structured (look for "test-project-" pattern in expected container names)
- `pkg/status/status_test.go` — look for `populateGeneratingSpec` tests or `docker ps` filter assertions to understand the test pattern
- `docs/configuration.md` — find the `projectName` entry to understand where to add the new `project:` entry (search for "projectName" in the file)
- `CHANGELOG.md` — read the most recent 20 lines to understand the entry format and voice
</context>

<requirements>

## 1. Update `computePromptMetadata` in `pkg/processor/processor.go`

Find `computePromptMetadata` (around line 497). Change the container name construction to include the `-exec-` role infix:

```go
// OLD:
name := prompt.ContainerName(string(projectName) + "-" + string(base)).Sanitize()

// NEW:
name := prompt.ContainerName(string(projectName) + "-exec-" + string(base)).Sanitize()
```

**This is the only change to `pkg/processor/processor.go`.** FREEZE everything else in this file.

## 2. Add `projectName project.Name` to `status.checker` and `status.NewChecker`

### 2a. Add `projectName project.Name` field to the `checker` struct

In `pkg/status/status.go`, add `projectName project.Name` as the FIRST field in the `checker` struct (before `projectDir`):

```go
type checker struct {
    projectName           project.Name    // NEW
    projectDir            string
    queueDir              string
    // ... all existing fields unchanged ...
}
```

### 2b. Add `projectName project.Name` as the FIRST parameter of `NewChecker`

```go
func NewChecker(
    projectName project.Name,    // NEW — first parameter
    projectDir string,
    queueDir string,
    completedDir string,
    logDir string,
    lockFilePath string,
    serverPort int,
    promptMgr PromptManager,
    containerCounter executor.ContainerCounter,
    maxContainers int,
    dirtyFileThreshold int,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    subprocRunner subproc.Runner,
) Checker {
    return &checker{
        projectName:           projectName,    // NEW
        projectDir:            projectDir,
        // ... all existing fields unchanged ...
    }
}
```

### 2c. Add import for `pkg/project` in `pkg/status/status.go`

Add to the existing import block:

```go
"github.com/bborbe/dark-factory/pkg/project"
```

## 3. Update `populateGeneratingSpec` to use the project-aware filter

In `pkg/status/status.go`, replace the hardcoded `"dark-factory-gen-"` prefix with the project-aware prefix built from `s.projectName`:

```go
func (s *checker) populateGeneratingSpec(ctx context.Context, st *Status) {
    genPrefix := string(s.projectName) + "-gen-"    // CHANGED: was const "dark-factory-gen-"
    out, err := s.subprocRunner.RunWithWarnAndTimeout(
        ctx,
        "docker ps --filter name="+genPrefix,    // CHANGED: was hardcoded
        "docker",
        "ps",
        "--filter",
        "name="+genPrefix,
        "--format",
        "{{.Names}}",
    )
    if errors.Is(err, context.DeadlineExceeded) {
        st.GeneratingSpecSkipped = true
        return
    }
    if err != nil {
        slog.Debug("docker ps for spec generation failed", "err", err)
        return
    }
    output := strings.TrimSpace(string(out))
    if output == "" {
        return
    }
    // Take the first matching container line
    var containerName string
    for _, line := range strings.Split(output, "\n") {
        name := strings.TrimSpace(line)
        if strings.HasPrefix(name, genPrefix) {
            containerName = name
            break
        }
    }
    if containerName == "" {
        return
    }
    specName := strings.TrimPrefix(containerName, genPrefix)
    st.GeneratingSpec = specName
    st.GeneratingContainer = containerName
}
```

The `const genPrefix = "dark-factory-gen-"` declaration is removed and replaced with the local variable. **FREEZE all other methods in the checker.**

## 4. Update `createStatusChecker` in `pkg/factory/factory.go` to accept and pass `projectName`

### 4a. Add `projectName project.Name` parameter to `createStatusChecker`

Add as the last parameter (after `currentDateTimeGetter`):

```go
func createStatusChecker(
    ctx context.Context,
    inProgressDir, completedDir, logDir string,
    serverPort int,
    promptManager *prompt.Manager,
    projectMax int,
    dirtyFileThreshold int,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    projectName project.Name,    // NEW
) status.Checker {
```

Pass it as the first argument to `status.NewChecker`:

```go
return status.NewChecker(
    projectName,    // NEW — first argument
    projectDir,
    inProgressDir,
    completedDir,
    logDir,
    lock.FilePath("."),
    serverPort,
    promptManager,
    createContainerCounter(),
    EffectiveMaxContainers(projectMax, globalCfg.MaxContainers),
    dirtyFileThreshold,
    currentDateTimeGetter,
    subproc.NewRunner(),
)
```

### 4b. Update the three call sites of `createStatusChecker`

Run `grep -n "createStatusChecker" pkg/factory/factory.go` to find all three call sites.

**Call site inside `CreateServer`** (line ~981):
`CreateServer` does not have access to `cfg config.Config`. Add `projectName project.Name` as a new parameter to `CreateServer` and pass it through:

```go
func CreateServer(
    ctx context.Context,
    port int,
    inboxDir string,
    inProgressDir string,
    completedDir string,
    logDir string,
    promptManager *prompt.Manager,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    projectMaxContainers int,
    projectName project.Name,    // NEW — last parameter
) server.Server {
    ...
    statusChecker := createStatusChecker(
        ctx,
        inProgressDir,
        completedDir,
        logDir,
        port,
        promptManager,
        projectMaxContainers,
        0,
        currentDateTimeGetter,
        projectName,    // NEW
    )
```

Then find where `CreateServer` is called in `factory.go` (run `grep -n "CreateServer" pkg/factory/factory.go`) and pass the resolved `projectName` there as well.

**Call site with `cfg config.Config`** (~line 1030, inside a function with access to `cfg`):
```go
statusChecker := createStatusChecker(
    ctx,
    cfg.Prompts.InProgressDir,
    cfg.Prompts.CompletedDir,
    cfg.Prompts.LogDir,
    cfg.ServerPort,
    promptManager,
    cfg.MaxContainers,
    cfg.DirtyFileThreshold,
    currentDateTimeGetter,
    project.Resolve(cfg.ResolvedProjectOverride()),    // NEW — use method from prompt 1
)
```

**Call site with `cfg config.Config`** (~line 1315, inside another function with access to `cfg`):
Same pattern — append `project.Resolve(cfg.ResolvedProjectOverride())` as the last argument.

Confirm there are no remaining call sites: `grep -c "createStatusChecker" pkg/factory/factory.go` must equal the count of updated sites (function definition + all call sites).

Audit confirmed `CreateServer` has NO callers outside `pkg/factory/factory.go`. Verify before assuming changes elsewhere:

```bash
grep -rn "CreateServer" pkg/ cmd/ --include="*.go" 2>/dev/null | grep -v factory.go
```

If the grep returns lines, update those callers; otherwise no further changes.

## 4c. Update all `status.NewChecker(` call sites in `pkg/status/status_test.go`

Adding `projectName project.Name` as the FIRST parameter of `NewChecker` breaks every test call site (audited count: 13 — confirm with `grep -n 'status.NewChecker(' pkg/status/status_test.go`). For EACH call site, prepend `project.Name("test-project")` as the first argument:

```go
// OLD:
status.NewChecker(projectDir, queueDir, ...)
// NEW:
status.NewChecker(project.Name("test-project"), projectDir, queueDir, ...)
```

Add the import `"github.com/bborbe/dark-factory/pkg/project"` to `pkg/status/status_test.go` if not already present. Do not skip any call site — `make test` must compile after this step.

## 5. Run `make test` after factory changes

Verify compilation and tests pass before proceeding.

## 6. Update `docs/configuration.md`

Find the `projectName` entry in `docs/configuration.md`:
```bash
grep -n "projectName" docs/configuration.md
```

Add the new `project:` field entry immediately after the `projectName` row. Match the formatting, column alignment, and description style of surrounding rows. The entry should read:

```
| `project` | Optional override for the Docker container name prefix (`<project>-gen-<spec>`, `<project>-exec-<prompt>`). When absent, defaults to the git working tree root directory basename. Rejects empty or whitespace-only values. | — |
```

If the docs use a different table format, match it exactly.

## 7. Add CHANGELOG migration note

`CHANGELOG.md` currently has NO `## Unreleased` section — entries live under `## vX.Y.Z` headings, with `## v0.158.0` at the top (under the `# Changelog` and the "All notable changes..." preface line). Insert a NEW `## Unreleased` section immediately BEFORE `## v0.158.0`, with the migration note bullet beneath:

```markdown
## Unreleased

- feat: Container names now follow `<project>-gen-<spec>` and `<project>-exec-<prompt>` schema (was `dark-factory-gen-<spec>` and `<project>-<prompt>`). The project name defaults to the git root directory basename. External tooling that greps for `dark-factory-gen-` must be updated to grep for `-gen-` or `<project>-gen-`. The optional `project:` field in `.dark-factory.yaml` overrides the project name.

## v0.158.0
```

Use the original CHANGELOG bullet shown below for content; the block above shows the placement skeleton.

Bullet content to use:

```markdown
- feat: Container names now follow `<project>-gen-<spec>` and `<project>-exec-<prompt>` schema (was `dark-factory-gen-<spec>` and `<project>-<prompt>`). The project name defaults to the git root directory basename. External tooling that greps for `dark-factory-gen-` must be updated to grep for `-gen-` or `<project>-gen-`. The optional `project:` field in `.dark-factory.yaml` overrides the project name.
```

Match the verb style and format of existing entries.

## 8. Add unit tests

### 8a. Processor container name test

In `pkg/processor/processor_test.go`, find existing assertions for the container name (search for `"test-project-"` in the file). The existing tests expect `test-project-001-test` (or similar). After this change, the expected container name becomes `test-project-exec-001-test` (or the corresponding exec-infixed form).

Run: `grep -n "test-project-" pkg/processor/processor_test.go` to find all affected assertions and update them to include the `-exec-` infix.

Additionally, find the `computePromptMetadata` unit test if one exists (search for `Describe("computePromptMetadata"`) and update the expected container name there too.

### 8b. Status filter test

In `pkg/status/status_test.go`, find the existing test for `populateGeneratingSpec` (search for `"dark-factory-gen-"` or `GeneratingSpec`). Update any assertions that check the docker ps filter string or the hardcoded prefix to use the project-aware prefix.

If no positive-path unit test exists for `populateGeneratingSpec`, add one in the existing suite using this scaffolding:

```go
It("parses generating container name with project-aware prefix", func() {
    fakeRunner.RunWithWarnAndTimeoutReturns([]byte("maintainer-gen-031-my-spec\n"), nil)
    checker := status.NewChecker(
        project.Name("maintainer"),
        // ... rest of constructor args matching the surrounding test setup ...
    )
    st, err := checker.GetStatus(ctx)
    Expect(err).NotTo(HaveOccurred())
    Expect(st.GeneratingSpec).To(Equal("031-my-spec"))
    Expect(st.GeneratingContainer).To(Equal("maintainer-gen-031-my-spec"))
    // Verify the filter string passed to the subprocess includes the project-aware prefix
    _, _, args := capturedRunArgs(fakeRunner, 0) // use whatever helper exists; or call ArgsForCall(0) directly
    Expect(args).To(ContainElement("name=maintainer-gen-"))
})
```

Reuse the existing `fakeRunner` (instance of `mocks.SubprocRunner`) and `ctx` from the surrounding suite. Match the constructor argument order of the surrounding tests verbatim.

## 9. Final verification

Run `make precommit` in `/workspace` — must exit 0.

After precommit passes, run:
```bash
grep -rn 'dark-factory-gen-' pkg/ | grep -v _test.go
```
Expected: no output (exit code 1 from grep).

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- This prompt builds on prompt 1 (spec-080-config-and-gen-naming) — `cfg.ResolvedProjectOverride()` and the `project *string` field must already exist
- `Public Go API of pkg/generator/, pkg/processor/, pkg/executor/, pkg/runner/, and pkg/status/ MUST NOT change in exported interface signatures` — the `Checker` interface (`GetStatus`, `GetQueuedPrompts`, `GetCompletedPrompts`) is unchanged; only the `NewChecker` constructor changes
- The `Processor` interface and `Executor` interface are FROZEN — only `computePromptMetadata` (a private helper) changes in processor.go
- Wrap all errors with `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`
- Do not touch `go.mod` / `go.sum` / `vendor/`
- The `populateGeneratingSpec` docker ps filter MUST use the project-aware prefix; the hardcoded constant `"dark-factory-gen-"` must not appear in status.go after this change
- Existing processor tests that assert specific container names (e.g., `"test-project-001-test"`) MUST be updated to reflect the new `-exec-` infix (e.g., `"test-project-exec-001-test"`) — do not leave mismatched assertions
- No new runtime dependencies
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional checks:
1. `grep -rn 'dark-factory-gen-' pkg/ --include='*.go' | grep -v _test.go | grep -v 'legacy-compat: probe pre-spec-080'` — must return no lines (exit code 1). The legacy-probe in `pkg/runner/lifecycle.go` is exempt and marked with a `// legacy-compat: probe pre-spec-080 container name` comment so this grep can exclude it.
1a. `grep -c "dark-factory-gen-" pkg/status/status.go` — must be 0.
2. `grep -n '"-exec-"' pkg/processor/processor.go` — one occurrence in `computePromptMetadata`
3. `grep -n 'projectName' pkg/status/status.go` — occurrences in struct, constructor, and `populateGeneratingSpec`
4. `grep -n 'project:' docs/configuration.md` — new entry present
5. `grep -A2 "## Unreleased" CHANGELOG.md` — migration note present with both old patterns named
6. `grep -n 'createStatusChecker' pkg/factory/factory.go` — all call sites have the new `projectName` trailing argument
</verification>
