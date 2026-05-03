---
status: approved
spec: [066-bug-branch-workflow-rejects-its-own-uncommitted-prompt-file-write]
created: "2026-05-03T20:10:00Z"
queued: "2026-05-03T20:10:05Z"
branch: dark-factory/bug-branch-workflow-rejects-its-own-uncommitted-prompt-file-write
---

<summary>
- `Brancher` gains a new `IsCleanIgnoring(ctx, ignorePrefixes []string) ([]string, error)` method that runs `git status --porcelain` and returns only the dirty paths that do NOT match any ignore prefix — empty slice means clean for the caller's purposes
- `branchWorkflowExecutor.setupInPlaceBranch` uses `IsCleanIgnoring` instead of `IsClean`, passing the dark-factory prompt directory prefixes, so its own bookkeeping writes no longer abort Setup
- A new `IgnorePathPrefixes []string` field on `WorkflowDeps` carries the prompt dirs from the factory to the executor without exposing config to the processor package
- `CreateWorkflowExecutor` and `CreateProcessor` gain the prompt dir prefix slice; callers (`CreateRunner`, `CreateOneShotRunner`) pass the four `Prompts.*` config fields
- Error message when user-side dirt is detected names the specific dirty path(s), not a generic "tree not clean" message
- `dark-factory prompt retry` → `dark-factory daemon` in a `workflow: branch` project advances past Setup on the first cycle with no manual `git commit` in between
- User-side dirt (e.g. `pkg/foo/bar.go` modified but uncommitted) still causes Setup to fail with a specific error listing the offending path
- All existing `IsClean` callers are unaffected; the original method stays in the interface unchanged
- `docs/workflows.md` branch-row documents that `IsCleanIgnoring` filters dark-factory's own prompt directories
- All existing tests updated to use `IsCleanIgnoring` stubs; new unit tests cover filtering logic and negative-control path
</summary>

<objective>
Fix the infinite-retry loop in `workflow: branch` projects where `dark-factory prompt retry` (or `requeue`) writes the prompt frontmatter to disk, leaving the working tree dirty, and the next daemon cycle's `setupInPlaceBranch` rejects the branch checkout because the tree isn't clean. The fix (Option C from the spec) teaches `Brancher` to ignore changes inside dark-factory's own state directories — those writes are daemon bookkeeping, not user work. User-side dirt outside those directories still aborts Setup with a clear, actionable error.
</objective>

<context>
Read `CLAUDE.md` for project conventions (error wrapping, Ginkgo/Gomega, Counterfeiter).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read in full before editing:
- `pkg/git/brancher.go` — `Brancher` interface, `IsClean` implementation, struct definition
- `pkg/processor/workflow_executor.go` — `WorkflowDeps` struct, `WorkflowExecutor` interface
- `pkg/processor/workflow_executor_branch.go` — `setupInPlaceBranch` at line 51 (the fix site)
- `pkg/factory/factory.go` — `CreateWorkflowExecutor` (~line 712) and `CreateProcessor` (~line 757) signatures; `CreateRunner` (~line 391) and `CreateOneShotRunner` (~line 510) call sites
- `pkg/factory/factory_test.go` — `CreateProcessor` call at line 72 (needs new param)
- `pkg/processor/processor_internal_test.go` — `stubBrancher` struct at line 390; test cases 11o (line 1184), 11ag (line 1524), 11ah (line 1542)
- `pkg/processor/processor_branchswitch_test.go` — all `brancher.IsCleanReturns` and `brancher.IsCleanCallCount` lines
- `pkg/git/brancher_test.go` — existing `IsClean` test at line 317 (add `IsCleanIgnoring` tests nearby)
- `docs/workflows.md` — `branch` workflow section at line 32
</context>

<requirements>

## 1. Add `IsCleanIgnoring` to `pkg/git/brancher.go`

### 1a. Interface — add the new method

In the `Brancher` interface, add `IsCleanIgnoring` immediately after `IsClean`:

```go
// IsClean returns true if the working tree has no uncommitted changes.
IsClean(ctx context.Context) (bool, error)
// IsCleanIgnoring returns the dirty paths that are NOT covered by any of
// the given ignorePrefixes. An empty slice means the tree is effectively
// clean for the caller's purposes. The prefix comparison is
// strings.HasPrefix(path, prefix) on the path as reported by
// git status --porcelain (relative, no leading slash).
// A non-nil error means the git command itself failed.
IsCleanIgnoring(ctx context.Context, ignorePrefixes []string) ([]string, error)
```

**FREEZE `IsClean`** — signature, return type, and all callers are unchanged.

### 1b. Implementation — add to `brancher` struct

Add the `IsCleanIgnoring` method to the `brancher` struct. The implementation runs `git status --porcelain`, splits on newlines, extracts the file path from each line (format: `XY<SPACE>path`), and skips lines whose path has a prefix that appears in `ignorePrefixes`. Any remaining dirty path is included in the return slice. An empty `ignorePrefixes` makes the behavior identical to `IsClean`:

Use `git status --porcelain -z` so that filenames are NUL-separated and never split on embedded whitespace. Each entry is `XY<SPACE><path>` followed by a NUL byte; renamed entries occupy two NUL-separated entries (`R  new` then the old path), but for prefix matching, only the path that appears in the working tree right now matters — the leading entry. The implementation:

```go
// IsCleanIgnoring returns dirty paths not covered by any ignorePrefixes.
func (b *brancher) IsCleanIgnoring(ctx context.Context, ignorePrefixes []string) ([]string, error) {
    cmd := exec.CommandContext(ctx, "git", "status", "--porcelain", "-z")
    output, err := cmd.Output()
    if err != nil {
        return nil, errors.Wrap(ctx, err, "check working tree status")
    }
    var dirty []string
    // -z output: each entry is "XY <path>\0". Renamed entries (status R or C)
    // append a second NUL-separated record holding the old path; we ignore the
    // old-path follow-up records by only examining entries with a status prefix.
    skipNext := false
    for _, entry := range strings.Split(strings.TrimRight(string(output), "\x00"), "\x00") {
        if skipNext {
            skipNext = false
            continue
        }
        if len(entry) < 4 {
            continue
        }
        statusXY := entry[:2]
        path := entry[3:]
        if statusXY[0] == 'R' || statusXY[0] == 'C' {
            // Next record is the old path — skip it for prefix matching.
            skipNext = true
        }
        ignored := false
        for _, prefix := range ignorePrefixes {
            if prefix != "" && strings.HasPrefix(path, prefix) {
                ignored = true
                break
            }
        }
        if !ignored {
            dirty = append(dirty, path)
        }
    }
    return dirty, nil
}
```

**Why `-z`:** without it, paths containing spaces or quote characters are quoted by git (`"path with spaces"`), and renames produce a single line with ` -> ` separator that breaks naive `line[3:]` parsing. NUL-separated output sidesteps both.

### 1c. Regenerate Counterfeiter mock

After writing `brancher.go`, run:
```bash
cd /workspace && go generate ./pkg/git/...
```

This regenerates `mocks/brancher.go` with `FakeIsCleanIgnoring`, `IsCleanIgnoringCallCount`, `IsCleanIgnoringReturns`, and `IsCleanIgnoringReturnsOnCall` — matching the counterfeiter pattern for the existing `IsClean` stub.

Verify the new methods exist:
```bash
grep -c "IsCleanIgnoring" mocks/brancher.go
```
Expected: ≥4 (stub, call count, returns, calls).

## 2. Add `IgnorePathPrefixes` to `WorkflowDeps` in `pkg/processor/workflow_executor.go`

Add the new field to `WorkflowDeps` immediately after `AutoRelease`:

```go
type WorkflowDeps struct {
    ProjectName   project.Name
    PromptManager PromptManager
    AutoCompleter spec.AutoCompleter
    Releaser      git.Releaser
    VersionGetter version.Getter
    Brancher      git.Brancher
    PRCreator     git.PRCreator
    Cloner        git.Cloner
    Worktreer     git.Worktreer
    PRMerger      git.PRMerger
    PR            bool
    AutoMerge     bool
    AutoReview    bool
    AutoRelease   bool
    // IgnorePathPrefixes lists directory prefixes (relative, no leading slash)
    // that branchWorkflowExecutor should treat as dark-factory bookkeeping and
    // exclude from the working-tree cleanliness check before branch switching.
    // Typically set to the four prompts.* config directories.
    // Nil or empty means no filtering (identical to the previous IsClean behavior).
    IgnorePathPrefixes []string
}
```

**FREEZE all other fields** — position, type, name unchanged.

## 3. Update `setupInPlaceBranch` in `pkg/processor/workflow_executor_branch.go`

Replace the `IsClean` call with `IsCleanIgnoring`. The full updated method is:

```go
func (e *branchWorkflowExecutor) setupInPlaceBranch(ctx context.Context, branch string) error {
    dirtyPaths, err := e.deps.Brancher.IsCleanIgnoring(ctx, e.deps.IgnorePathPrefixes)
    if err != nil {
        return errors.Wrap(ctx, err, "check working tree")
    }
    if len(dirtyPaths) > 0 {
        return errors.Errorf(
            ctx,
            "working tree is not clean; cannot switch to branch %q; uncommitted changes: %s",
            branch,
            strings.Join(dirtyPaths, ", "),
        )
    }
    defaultBranch, err := e.deps.Brancher.DefaultBranch(ctx)
    if err != nil {
        return errors.Wrap(ctx, err, "get default branch")
    }
    e.inPlaceDefaultBranch = defaultBranch
    e.inPlaceBranch = branch

    if err := e.deps.Brancher.FetchAndVerifyBranch(ctx, branch); err == nil {
        if err := e.deps.Brancher.Switch(ctx, branch); err != nil {
            return errors.Wrap(ctx, err, "switch to existing branch")
        }
    } else {
        if err := e.deps.Brancher.CreateAndSwitch(ctx, branch); err != nil {
            return errors.Wrap(ctx, err, "create and switch to branch")
        }
    }
    slog.Info("switched to branch for in-place execution", "branch", branch)
    return nil
}
```

Add `"strings"` to the import block if not already present.

**The rest of `workflow_executor_branch.go` is unchanged.**

## 4. Update `pkg/factory/factory.go`

### 4a. Add `promptDirPrefixes []string` to `CreateWorkflowExecutor`

Add the parameter as the LAST argument (after `autoCompleter spec.AutoCompleter`) and wire it into `WorkflowDeps`:

```go
func CreateWorkflowExecutor(
    workflow config.Workflow,
    pr bool,
    brancher git.Brancher,
    prCreator git.PRCreator,
    prMerger git.PRMerger,
    autoMerge bool,
    autoRelease bool,
    autoReview bool,
    projectName project.Name,
    promptManager *prompt.Manager,
    releaser git.Releaser,
    autoCompleter spec.AutoCompleter,
    promptDirPrefixes []string,
) processor.WorkflowExecutor {
    deps := processor.WorkflowDeps{
        ProjectName:        projectName,
        PromptManager:      promptManager,
        AutoCompleter:      autoCompleter,
        Releaser:           releaser,
        Brancher:           brancher,
        PRCreator:          prCreator,
        Cloner:             git.NewCloner(),
        Worktreer:          git.NewWorktreer(),
        PRMerger:           prMerger,
        PR:                 pr,
        AutoMerge:          autoMerge,
        AutoReview:         autoReview,
        AutoRelease:        autoRelease,
        IgnorePathPrefixes: promptDirPrefixes,
    }
    // ... switch unchanged ...
}
```

### 4b. Add `promptDirPrefixes []string` to `CreateProcessor`

Add the parameter as the LAST argument before `preflightChecker preflight.Checker` (i.e., immediately after `autoRetryLimit int`), and forward it to `CreateWorkflowExecutor`.

**Current last few params** (confirm by reading the file):
```
maxPromptDuration time.Duration, autoRetryLimit int,
hideGit bool,
preflightChecker preflight.Checker,
```

Insert `promptDirPrefixes []string` between `hideGit bool` and `preflightChecker preflight.Checker`:

```go
func CreateProcessor(
    // ... all existing params unchanged, then at the end: ...
    hideGit bool,
    promptDirPrefixes []string,
    preflightChecker preflight.Checker,
    queueInterval time.Duration,
    sweepInterval time.Duration,
    onIdle processor.NothingToDoCallback,
) processor.Processor {
```

Update the `CreateWorkflowExecutor` call inside `CreateProcessor` to pass `promptDirPrefixes` as the final argument:

```go
workflowExecutor := CreateWorkflowExecutor(
    workflow, pr, brancher, prCreator, prMerger,
    autoMerge, autoRelease, autoReview,
    projectName, promptManager, releaser, autoCompleter,
    promptDirPrefixes,
)
```

**FREEZE everything else in `CreateProcessor`.**

### 4c. Update `CreateRunner` call (~line 391)

Find the existing `CreateProcessor(...)` call in `CreateRunner`. After `cfg.HideGit` and before `preflightChecker`, insert the prompt dir prefixes slice:

```go
proc := CreateProcessor(
    inProgressDir, completedDir, cfg.Prompts.LogDir, projectName,
    promptManager, releaser, versionGetter, wakeup,
    cfg.ContainerImage, cfg.Model, cfg.NetrcFile, cfg.GitconfigFile,
    cfg.Workflow, cfg.PR,
    deps.brancher, deps.prCreator, deps.prMerger,
    cfg.AutoMerge, cfg.AutoRelease, cfg.AutoReview,
    cfg.ValidationCommand, cfg.ValidationPrompt, cfg.TestCommand,
    cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir, cfg.Specs.RejectedDir,
    cfg.VerificationGate, cfg.Env, cfg.ExtraMounts, currentDateTimeGetter, n,
    cfg.ResolvedClaudeDir(),
    createContainerCounter(),
    EffectiveMaxContainers(cfg.MaxContainers, globalCfg.MaxContainers),
    cfg.AdditionalInstructions,
    cl,
    containerChecker,
    cfg.DirtyFileThreshold, dirtyFileChecker, gitLockChecker,
    cfg.ParsedMaxPromptDuration(), cfg.AutoRetryLimit,
    cfg.HideGit,
    []string{cfg.Prompts.InboxDir, cfg.Prompts.InProgressDir, cfg.Prompts.CompletedDir, cfg.Prompts.LogDir}, // promptDirPrefixes
    preflightChecker,
    cfg.ParsedQueueInterval(),
    cfg.ParsedSweepInterval(),
    func(_ context.Context, _ context.CancelFunc) {
        slog.Info("nothing to do, waiting for changes")
        // ... rest unchanged ...
    },
)
```

### 4d. Update `CreateOneShotRunner` call (~line 510)

Apply the same insertion — the exact same 4-element slice — in the inline `CreateProcessor(...)` call inside `CreateOneShotRunner`. Find `cfg.HideGit` in that call and insert the slice immediately after it, before the `preflightChecker` argument.

Run `grep -n "HideGit\|preflightChecker\|promptDirPrefixes" pkg/factory/factory.go` to locate both call sites before editing. Do NOT miss either one.

## 5. Update `pkg/factory/factory_test.go`

In the `CreateProcessor` call at ~line 72, add `nil` as the new `promptDirPrefixes` argument immediately after `false` (the `hideGit` argument, currently near the end):

```go
// ... existing args ...
0,      // autoRetryLimit
false,  // hideGit
nil,    // promptDirPrefixes — nil means no ignore filtering in tests
nil,    // preflightChecker
// ... rest unchanged ...
```

Find the exact position by running:
```bash
grep -n "false.*hideGit\|false,\s*$\|preflightChecker" pkg/factory/factory_test.go
```

## 6. Update `pkg/processor/processor_internal_test.go` — `stubBrancher`

### 6a. Add `IsCleanIgnoring` method to `stubBrancher`

The `stubBrancher` struct (line ~390) implements the `git.Brancher` interface. After adding `IsCleanIgnoring` to the interface, the stub must implement it. Add the method immediately after `IsClean`:

```go
func (s *stubBrancher) IsCleanIgnoring(_ context.Context, _ []string) ([]string, error) {
    if s.isCleanErr != nil {
        return nil, s.isCleanErr
    }
    if !s.isClean {
        // Return a synthetic dirty path so callers can construct an error message.
        return []string{"test/dirty-file.go"}, nil
    }
    return nil, nil
}
```

This delegates to the same `isClean`/`isCleanErr` fields so all existing tests that set `stubBr.isClean = true/false` work without changing their setup.

### 6b. Keep `stubBrancher.IsClean` unchanged

The original `IsClean` method stays in the interface and `stubBrancher` must still implement it. Do NOT remove it.

### 6c. Update test label comment at line 389

Update the comment to include `IsCleanIgnoring`:
```go
// stubBrancher tracks Push/CreateAndSwitch/FetchAndVerify/Switch/IsClean/IsCleanIgnoring/DefaultBranch/MergeToDefault calls.
```

## 7. Update `pkg/processor/processor_branchswitch_test.go` — mock Brancher stubs

The test file uses the Counterfeiter `mocks.Brancher`. Since `setupInPlaceBranch` now calls `IsCleanIgnoring` instead of `IsClean`, every test that previously set up `brancher.IsCleanReturns(...)` must switch to `brancher.IsCleanIgnoringReturns(...)`, and every `brancher.IsCleanCallCount()` must become `brancher.IsCleanIgnoringCallCount()`.

**Return-value mapping:**
- Old `brancher.IsCleanReturns(true, nil)` → New `brancher.IsCleanIgnoringReturns(nil, nil)` (nil = empty = clean)
- Old `brancher.IsCleanReturns(false, nil)` → New `brancher.IsCleanIgnoringReturns([]string{"pkg/dirty.go"}, nil)` (non-empty = dirty)
- Count assertions: `brancher.IsCleanCallCount()` → `brancher.IsCleanIgnoringCallCount()`

Run this before editing to get exact line numbers:
```bash
grep -n "IsCleanReturns\|IsCleanCallCount\|IsClean" pkg/processor/processor_branchswitch_test.go
```

Expected occurrences based on the current file (update all of them):
- `brancher.IsCleanReturns(true, nil)` at ~lines 223, 271, 356, 445, 550, 589 → `brancher.IsCleanIgnoringReturns(nil, nil)`
- `brancher.IsCleanReturns(false, nil)` at ~line 313 → `brancher.IsCleanIgnoringReturns([]string{"pkg/dirty.go"}, nil)`
- `brancher.IsCleanCallCount()` at ~lines 162, 196, 237, 288, 322, 420 → `brancher.IsCleanIgnoringCallCount()`
- Comment at ~line 320 "Wait for the IsClean call to happen" → "Wait for the IsCleanIgnoring call to happen"

After editing, run:
```bash
grep -n "IsClean[^I]" pkg/processor/processor_branchswitch_test.go
```
Expected: zero matches (no remaining calls to the old `IsClean` method in the test file).

## 8. Add unit tests for `IsCleanIgnoring` in `pkg/git/brancher_test.go`

Add a new `Describe("IsCleanIgnoring", ...)` block after the existing `Describe("IsClean", ...)` block. This test uses the real `git` binary in a temp repo (matching the existing `IsClean` test pattern — read the `BeforeEach` at line ~19 to understand the temp repo setup).

### 8a. Empty repo → returns nil (clean)

After the `BeforeEach` creates a clean repo, call `IsCleanIgnoring(ctx, nil)`. Assert `dirtyPaths` is nil and `err` is nil.

### 8b. Dirty file outside prefixes → returns the path

Create a new untracked/modified file (e.g. `os.WriteFile(filepath.Join(tempDir, "pkg/src/feature.go"), ...)`) without committing. Call `IsCleanIgnoring(ctx, []string{"prompts/in-progress"})`. Assert `len(dirtyPaths) == 1` and `dirtyPaths[0]` contains `"pkg/src/feature.go"` (or the relative path git reports).

### 8c. Dirty file matching ignore prefix → returns nil

Create `prompts/in-progress/001-test.md` without committing. Call `IsCleanIgnoring(ctx, []string{"prompts/in-progress"})`. Assert `dirtyPaths` is nil and `err` is nil.

### 8d. Mixed: one ignored, one not → returns only the non-ignored path

Create both `prompts/in-progress/001-test.md` AND `pkg/handler/foo.go` without committing. Call `IsCleanIgnoring(ctx, []string{"prompts/in-progress"})`. Assert `len(dirtyPaths) == 1` and `dirtyPaths[0]` does NOT contain `"prompts"`.

### 8e. Negative-control: empty prefix list, dirty repo → returns dirty paths

Create `pkg/handler/foo.go` without committing. Call `IsCleanIgnoring(ctx, nil)`. Assert `len(dirtyPaths) >= 1`.

**Note on git repo setup for sub-paths:** `git status --porcelain` only reports relative paths from the git root. The test's `BeforeEach` already `os.Chdir(tempDir)`, so paths will be reported as-is (e.g. `?? pkg/src/feature.go`). You may need to `os.MkdirAll(filepath.Join(tempDir, "pkg/src"), 0750)` to create sub-directories. The `??` prefix in porcelain output has length 3 (`??<space>`), so `line[3:]` gives `pkg/src/feature.go`.

## 9. Add new tests in `pkg/processor/processor_internal_test.go` for the filter behavior

Add two test cases after `11o` (dirty tree):

### 9a: `11o-ignored`: dirty prompt file is filtered → Setup proceeds

```go
Describe("11o-ignored: setupInPlaceBranch dirty prompt file is filtered", func() {
    It("advances past IsCleanIgnoring when only prompt dirs are dirty", func() {
        deps := makeDeps(true)
        // stubBrancher.IsCleanIgnoring returns nil (clean after filtering)
        // because stubBrancher.isClean is true by default in makeDeps(true)
        stubBr.isClean = true
        // Ensure branch creation path works
        stubBr.fetchAndVerifyErr = stderrors.New("branch not found remotely")
        rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
        Expect(ok).To(BeTrue())

        err := rawExec.setupInPlaceBranch(ctx, "feature/prompt-retry")
        Expect(err).NotTo(HaveOccurred())
    })
})
```

### 9b: `11o-userdir`: user-side dirty path names the file in the error

```go
Describe("11o-userdir: setupInPlaceBranch user-side dirt names the file in error", func() {
    It("error message contains the offending path", func() {
        deps := makeDeps(true)
        stubBr.isClean = false // stubBrancher returns "test/dirty-file.go"
        rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
        Expect(ok).To(BeTrue())

        err := rawExec.setupInPlaceBranch(ctx, "feature/user-dirt")
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("test/dirty-file.go"))
        Expect(err.Error()).To(ContainSubstring("feature/user-dirt"))
    })
})
```

## 10. Update `docs/workflows.md` — branch workflow section

In the `### branch` section (around line 32), add a bullet below the existing bullets describing the new filtering behavior:

```markdown
### `branch`

- dark-factory runs `git fetch origin`, then `git checkout -b <promptBranch> origin/<defaultBranch>` in the parent repo
- Container mounts the parent repo at `/workspace`, now on the new branch
- After the prompt: commit, optionally push / open PR
- Serial execution (parent repo is shared)
- **Working-tree cleanliness check:** before switching branches, dark-factory verifies the tree is clean — but ignores uncommitted changes inside the four prompt directories (`inboxDir`, `inProgressDir`, `completedDir`, `logDir`) because those are dark-factory's own bookkeeping writes, not user work. Any uncommitted change outside those directories still aborts Setup with an error naming the specific dirty file.
```

**FREEZE all other sections of `docs/workflows.md`.**

## 11. Add CHANGELOG entry

In `CHANGELOG.md`, add a bullet under `## Unreleased` (create it at the top if it doesn't exist):

```
- fix: branch workflow no longer rejects its own prompt-file frontmatter writes as working-tree dirt; IsCleanIgnoring filters dark-factory state directories before branch checkout
```

## 12. Run verification

After all changes:
```bash
cd /workspace && make test
```

Fix any compilation errors. Then:
```bash
cd /workspace && make precommit
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT change `IsClean` signature, return type, or behavior — it remains in the `Brancher` interface unchanged; only `IsCleanIgnoring` is new.
- Do NOT change the prompt-file-as-source-of-truth invariant. Prompt frontmatter stays the canonical state store.
- Do NOT regress `worktree`, `clone`, or `direct` workflows — those executors do NOT call `IsClean` or `IsCleanIgnoring`.
- The `IgnorePathPrefixes` field being nil or empty in `WorkflowDeps` must result in no filtering — identical behavior to the previous `IsClean` call (empty `dirtyPaths` returned only when tree is actually clean).
- Wrap all non-nil errors with `errors.Wrap`/`errors.Wrapf`/`errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- Error messages: lowercase, no trailing period.
- The `strings.Join(dirtyPaths, ", ")` in the error message must only appear when `len(dirtyPaths) > 0`.
- All existing tests in `processor_branchswitch_test.go` must pass after the `IsClean` → `IsCleanIgnoring` migration. Do not delete any existing test case.
- `mocks/brancher.go` must be regenerated via `go generate`, not hand-edited.
- Do not touch `go.mod` / `go.sum` / `vendor/`.
- Do not add `IsCleanIgnoring` to the `WorkflowExecutor` interface — it is an implementation detail of the `branch` executor only.
- The `"strings"` import in `workflow_executor_branch.go` must be added if not already present.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot-checks:
1. `grep -n "IsCleanIgnoring" pkg/git/brancher.go` — two occurrences (interface + implementation)
2. `grep -n "IsCleanIgnoring" mocks/brancher.go` — ≥4 occurrences (generated fake methods)
3. `grep -n "IgnorePathPrefixes" pkg/processor/workflow_executor.go` — one occurrence (struct field)
4. `grep -n "IsCleanIgnoring" pkg/processor/workflow_executor_branch.go` — one occurrence (in setupInPlaceBranch)
5. `grep -n "IsClean[^I]" pkg/processor/processor_branchswitch_test.go` — zero occurrences (all old IsClean calls replaced)
6. `grep -n "promptDirPrefixes\|IgnorePathPrefixes" pkg/factory/factory.go` — ≥3 occurrences (CreateWorkflowExecutor param, CreateProcessor param, wiring)
7. `grep -n "cfg.Prompts.InboxDir.*cfg.Prompts.InProgressDir\|promptDirPrefixes" pkg/factory/factory.go` — two occurrences (CreateRunner + CreateOneShotRunner call sites)
8. `grep -n "IsCleanIgnoring" pkg/git/brancher_test.go` — ≥5 occurrences (test cases 8a–8e)
9. `grep -n '\-\-porcelain.*-z\|"-z"' pkg/git/brancher.go` — one occurrence (NUL-separated git status output for safe filename parsing)

## Runtime replay (handed off to spec-verify, not run inside this prompt)

Spec 066 `## Verification` requires that the fix be replayed against the *built* binary in a real `workflow: branch` project. That replay happens at `dark-factory:verify-spec` time, **after** this prompt's container exits, the changes commit, the binary is rebuilt (`make install`), and a new daemon instance is started in `~/Documents/workspaces/jira-task-creator`.

This prompt's `make precommit` covers unit + integration coverage and is **necessary but not sufficient** to mark the spec `completed`. Do NOT report `success` with the implication that the bug is verified gone — only that the code change passed unit tests. The runtime replay is the spec-level gate.
</verification>
