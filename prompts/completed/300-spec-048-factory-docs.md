---
status: completed
spec: [048-hide-git]
summary: Wired hideGit in CreateProcessor (worktree always true, other workflows use cfg.HideGit), updated docs/workflows.md with masking docs, and added 5 executor hideGit test cases.
container: dark-factory-300-spec-048-factory-docs
dark-factory-version: v0.111.2
created: "2026-04-16T19:30:00Z"
queued: "2026-04-16T19:14:05Z"
started: "2026-04-16T19:26:14Z"
completed: "2026-04-16T19:40:56Z"
branch: dark-factory/hide-git
---

<summary>
- The factory now computes `hideGit` as `cfg.Workflow == WorkflowWorktree || cfg.HideGit` and passes it to `createDockerExecutor`, making worktree mode always mask `.git` without any user configuration
- `direct`, `branch`, and `clone` workflows honor `cfg.HideGit` as an opt-in; when `hideGit: false` (the default), the docker command is byte-for-byte identical to before this spec
- `CreateSpecGenerator` continues to pass `hideGit=false` — spec generation never needs `.git` masking
- `docs/workflows.md` documents the worktree auto-mask behavior and the `hideGit` opt-in for other workflows; the container semantics table remains accurate
- Executor tests cover all five acceptance criteria: worktree-with-directory-.git, worktree-with-file-.git, opt-in-enabled-with-directory-.git, opt-in-disabled-default, missing-.git
- All existing tests continue to pass; `make precommit` exits 0
</summary>

<objective>
Wire the factory so that `workflow: worktree` always sets `hideGit=true` (correctness requirement) and that `cfg.HideGit` propagates for other workflows. Add the `docs/workflows.md` updates and comprehensive executor tests to meet all acceptance criteria from spec 048.
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, Counterfeiter).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `docs/configuration.md` for documentation conventions.
Read `docs/workflows.md` for the current state (this file is updated in this prompt).

**Precondition:** Prompt 1 (spec-048-config-executor) has already been executed. The following are in place:
- `pkg/config/config.go` has `HideGit bool` field with yaml tag `hideGit`
- `pkg/executor/executor.go` has `hideGit bool` in `dockerExecutor`, `NewDockerExecutor` accepts `hideGit bool` as last parameter, `buildDockerCommand` has the mount logic
- `pkg/executor/export_test.go` helpers accept `hideGit bool` as last parameter
- `pkg/factory/factory.go` has `createDockerExecutor` accepting `hideGit bool`, currently all call sites pass `false`

Files to read before editing:
- `pkg/factory/factory.go` — `createDockerExecutor` call site (~line 605 inside `CreateProcessor`) to update `false` → computed value
- `pkg/config/workflow.go` — verify `WorkflowWorktree` constant value
- `pkg/executor/executor_test.go` — find the `Describe("buildDockerCommand", ...)` block and understand the helper closure used by tests (around line 370) to add new `It` cases
- `docs/workflows.md` — current documentation to update
</context>

<requirements>

## 1. Wire `hideGit` in `pkg/factory/factory.go`

**Ground truth** (verified before writing this prompt):
- `CreateProcessor` (line 561) signature includes `workflow config.Workflow` but NOT `cfg config.Config`.
- The call to `createDockerExecutor` lives inside `CreateProcessor` at line 605.
- `CreateProcessor` has exactly two callers in this file: line 303 (`CreateRunner`) and line 396 (`CreateOneShotRunner`). Both have `cfg config.Config` in scope.

**Changes — do exactly this, no judgment calls:**

### 1a. Add `hideGit bool` parameter to `CreateProcessor`

In `pkg/factory/factory.go`, `CreateProcessor`'s signature currently ends with:

```go
gitLockChecker processor.GitLockChecker, maxPromptDuration time.Duration, autoRetryLimit int,
) processor.Processor {
```

Append a new trailing parameter `hideGit bool`:

```go
gitLockChecker processor.GitLockChecker, maxPromptDuration time.Duration, autoRetryLimit int,
hideGit bool,
) processor.Processor {
```

### 1b. Use it inside `CreateProcessor`

Inside `CreateProcessor`, the call to `createDockerExecutor` at ~line 605 currently passes no `hideGit` value (the private function's last arg is `currentDateTimeGetter`). After prompt 1, `createDockerExecutor` already accepts a trailing `hideGit bool` parameter; every existing call site passes `false`. Replace that `false` here with:

```go
workflow == config.WorkflowWorktree || hideGit,
```

So the call becomes (approximate shape — preserve the other args verbatim):

```go
createDockerExecutor(
    containerImage, projectName, model, netrcFile,
    gitconfigFile, env, extraMounts, claudeDir, maxPromptDuration,
    currentDateTimeGetter,
    workflow == config.WorkflowWorktree || hideGit,
),
```

### 1c. Update all `CreateProcessor` call sites

Three locations call `CreateProcessor`:
- `pkg/factory/factory.go:303` (inside `CreateRunner`) — append `cfg.HideGit` as new trailing argument; `cfg` is in scope
- `pkg/factory/factory.go:396` (inside `CreateOneShotRunner`) — append `cfg.HideGit` as new trailing argument; `cfg` is in scope
- `pkg/factory/factory_test.go:60` (inside `Describe("CreateProcessor", ...)` — append `false` as new trailing argument; the test only checks non-nil and does not exercise hideGit behavior

Run `grep -rn "CreateProcessor(" pkg/factory/ | grep -v "^[^:]*:[^:]*:func"` to confirm exactly 3 call sites and update each.

### 1d. `CreateSpecGenerator` stays unchanged

`CreateSpecGenerator` (line 448) calls `createDockerExecutor` directly with `false` for `hideGit`. Do NOT change it — spec generators never need `.git` masking.

### 1e. Do not change `createDockerExecutor`

After prompt 1, `createDockerExecutor`'s signature already accepts `hideGit bool` as its last parameter and threads it into `NewDockerExecutor`. No signature change is needed in this prompt.

## 2. Update `docs/workflows.md`

### 2a. Worktree section

In the `### worktree` section (around line 39–48), add a new bullet after the existing "Container mounts the worktree at /workspace" bullet:

```
- **`.git` is always masked** inside the container: the worktree's `.git` pointer file is covered by a Docker volume overlay, preventing the dangling-pointer error (`fatal: not a git repository`) that previously crashed container startup
```

### 2b. Container semantics table

The container semantics table (around line 88–93) currently has this row for `worktree`:

```
| worktree | worktree files | `.git` pointer file (target not mounted) | NO — prompts must avoid git |
```

Update the `.git inside container` column to reflect the masking:

```
| worktree | worktree files | `.git` masked (anonymous volume or `/dev/null` bind — see hideGit) | NO — prompts must avoid git |
```

### 2c. Add `hideGit` opt-in documentation

Add a new subsection `## `.git` masking (`hideGit`)` at the end of the file (after the `## Implementation notes` section), with the following content:

```markdown
## `.git` masking (`hideGit`)

By default, `workflow: worktree` always masks the worktree's `.git` pointer file inside the container. The mask prevents `git` from following a dangling gitdir pointer, so the container no longer crashes with `fatal: not a git repository`. For other workflows (`direct`, `branch`, `clone`), the container sees the host's real `.git` directory by default.

To opt in to the same masking for non-worktree workflows, set `hideGit: true` in `.dark-factory.yaml`:

```yaml
workflow: direct
hideGit: true
```

**Mount shape** — the mask is chosen by inspecting the project root's `.git` entry at container launch time:

| `.git` on host | Docker flag added |
|----------------|-------------------|
| Directory (normal repo, clone) | `-v /workspace/.git` — anonymous volume hides host directory contents |
| File (worktree pointer, submodule) | `-v /dev/null:/workspace/.git` — bind hides the pointer |
| Missing | no flag — nothing to hide |

`hideGit: true` is strictly additive isolation — it reduces what the container can see, never expands it. The host's `.git/config` (which may contain tokens) becomes invisible to the container.
```

## 3. Add executor tests in `pkg/executor/executor_test.go`

Add a new `Describe("buildDockerCommand hideGit", ...)` block at the same nesting level as the existing `Describe("buildDockerCommand", ...)` block (line 370) — inside the same outer `var Describe`/function they share. Do NOT modify any existing `It` blocks.

The new tests use `BuildDockerCommandForTest` with a real temp directory as `projectRoot` so that `os.Stat` can observe actual filesystem state.

### Helper setup

**Ground truth** (verified):
- The outer `Describe` declares `ctx context.Context` in a `var (...)` block at line 30 and assigns it in `BeforeEach` at line 38.
- There is NO `home` variable in scope — the existing `buildDockerCommand` tests pass a literal string like `"/home/user"` as the `home` argument.
- After prompt 1, `executor.BuildDockerCommandForTest` accepts `hideGit bool` as its trailing parameter.

At the top of the new `Describe("buildDockerCommand hideGit", ...)` block, define a closure that constructs a command with a specific `hideGit` value and a specific `projectRoot`:

```go
buildCmd := func(projectRoot string, hideGit bool) *exec.Cmd {
    return executor.BuildDockerCommandForTest(
        ctx,
        config.Defaults().ContainerImage,
        "test-project",
        "claude-sonnet-4-6",
        "",    // netrcFile
        "",    // gitconfigFile
        nil,   // env
        nil,   // extraMounts
        "test-container",
        "/tmp/prompt.md",
        projectRoot,
        "/home/user/.claude",
        "test-prompt",
        "/home/user",
        hideGit,
    )
}
```

`ctx` is captured from the outer `var` block (same pattern as existing `Describe("buildDockerCommand", ...)`). Pass `"/home/user"` as a literal for the `home` argument — matches the existing test convention at lines 410 and 424.

### Test cases

**3a. hideGit=false, no .git present → no masking flags**

```go
It("does not add any masking flag when hideGit is false", func() {
    dir := GinkgoT().TempDir()
    cmd := buildCmd(dir, false)
    Expect(cmd.Args).NotTo(ContainElement("/workspace/.git"))
    Expect(cmd.Args).NotTo(ContainElement("/dev/null:/workspace/.git"))
})
```

**3b. hideGit=true, .git missing → no masking flags**

```go
It("does not add masking flag when hideGit is true but .git is missing", func() {
    dir := GinkgoT().TempDir()
    // No .git created — dir is empty
    cmd := buildCmd(dir, true)
    Expect(cmd.Args).NotTo(ContainElement("/workspace/.git"))
    Expect(cmd.Args).NotTo(ContainElement("/dev/null:/workspace/.git"))
})
```

**3c. hideGit=true, .git is a directory → anonymous volume**

```go
It("adds anonymous volume when hideGit is true and .git is a directory", func() {
    dir := GinkgoT().TempDir()
    Expect(os.Mkdir(filepath.Join(dir, ".git"), 0750)).To(Succeed())
    cmd := buildCmd(dir, true)
    Expect(cmd.Args).To(ContainElement("/workspace/.git"))
    Expect(cmd.Args).NotTo(ContainElement("/dev/null:/workspace/.git"))
})
```

**3d. hideGit=true, .git is a file → /dev/null bind**

```go
It("adds /dev/null bind when hideGit is true and .git is a file", func() {
    dir := GinkgoT().TempDir()
    Expect(os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../.git/worktrees/foo"), 0600)).To(Succeed())
    cmd := buildCmd(dir, true)
    Expect(cmd.Args).To(ContainElement("/dev/null:/workspace/.git"))
    Expect(cmd.Args).NotTo(ContainElement("/workspace/.git")) // anonymous volume form must NOT appear
})
```

NOTE: The anonymous volume form (`/workspace/.git`) is a substring of `/dev/null:/workspace/.git`. Use `ContainElement` (exact match on a single Args element), not `ContainSubstring`, to avoid false positives. The assertion `Expect(cmd.Args).NotTo(ContainElement("/workspace/.git"))` checks that no arg is EXACTLY `/workspace/.git`, which is correct — `/dev/null:/workspace/.git` is a different element.

**3e. hideGit=false, .git is a directory → same args as without any hideGit logic**

```go
It("produces byte-identical args when hideGit is false regardless of .git presence", func() {
    dirWithGit := GinkgoT().TempDir()
    Expect(os.Mkdir(filepath.Join(dirWithGit, ".git"), 0750)).To(Succeed())
    dirWithoutGit := GinkgoT().TempDir()

    cmdWith := buildCmd(dirWithGit, false)
    cmdWithout := buildCmd(dirWithoutGit, false)

    Expect(cmdWith.Args).To(Equal(cmdWithout.Args))
})
```

This test locks in the "no impact when disabled" requirement from spec 048 Desired Behavior 4.

### Required imports

The new tests use `os` and `path/filepath`. Check the existing import block in `executor_test.go` — add them if not already present. Do NOT remove any existing imports.

## 4. Write `## Unreleased` CHANGELOG entry

Check if `CHANGELOG.md` has an `## Unreleased` section. If not, add one immediately after the first `# Changelog` or `# CHANGELOG` heading. Append:

```
- feat: mask /workspace/.git inside containers for worktree workflow and via hideGit config opt-in
```

If `## Unreleased` already exists, append the bullet to it.

## 5. Run `make precommit`

Run `make precommit` in `/workspace`. It must exit 0. If it fails, fix the failing target, then re-run only that target before retrying `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- `CreateProcessor` gets a new trailing `hideGit bool` parameter; both call sites (`CreateRunner`, `CreateOneShotRunner`) pass `cfg.HideGit`; the logic `workflow == config.WorkflowWorktree || hideGit` lives inside `CreateProcessor` at the `createDockerExecutor` call
- `CreateSpecGenerator` must continue to pass `false` for `hideGit` — spec generators never need `.git` masking
- For the `docs/workflows.md` update: preserve all existing section headings, tables, and bullets — only ADD content, never remove lines that are still accurate
- Wrap all non-nil errors with `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors` (no new error paths expected)
- Do not touch `go.mod` / `go.sum` / `vendor/`
- The new test `It` blocks must be in `package executor_test` (external test package) — they call `executor.BuildDockerCommandForTest`, not internal methods
- Use `GinkgoT().TempDir()` for temp directories in tests — it cleans up automatically
- Use `os.Mkdir` with mode `0750` for `.git` directories in tests (matches go-security-linting.md rules)
- Use `os.WriteFile` with mode `0600` for `.git` file in tests
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "WorkflowWorktree\|HideGit" pkg/factory/factory.go` — at least one occurrence showing the computed `hideGit` expression
2. `grep -n "hideGit" docs/workflows.md` — at least one occurrence (the opt-in documentation)
3. `go test -coverprofile=/tmp/cover.out ./pkg/executor/... && go tool cover -func=/tmp/cover.out | grep -E "buildDockerCommand"` — coverage for `buildDockerCommand` includes the new branch
4. Run the new test cases: `go test -run "hideGit" ./pkg/executor/...` — all five pass
5. `grep -n "worktree.*masked\|masked\|anonymous volume" docs/workflows.md` — container semantics table updated
</verification>
