---
status: failed
spec: [092-fix-prompt-complete-autorelease]
container: dark-factory-fix-autorelease-exec-441-fix-prompt-complete-autorelease
dark-factory-version: v0.174.4
created: "2026-06-03T00:00:00Z"
queued: "2026-06-03T06:07:20Z"
started: "2026-06-03T06:08:06Z"
completed: "2026-06-03T06:21:02Z"
branch: dark-factory/fix-prompt-complete-autorelease
lastFailReason: 'validate completion report: completion report status: failed'
---

<summary>
- The one-shot CLI `dark-factory prompt complete <id>` currently calls the releaser unconditionally — every `prompt complete` on a feature branch rewrites `## Unreleased` to `## vX.Y.Z`, tags, and pushes
- Operator's `--set autoRelease=false` is silently ignored on the direct CLI path (the daemon path honours it, but the CLI does not)
- After this prompt: `prompt complete` honours `cfg.AutoRelease` end-to-end and adds a branch-context safety default — on any non-`master` branch, completion commits but does NOT release, unless the operator passes `--release`
- `--release` is the explicit opt-in flag for the rare case where the operator genuinely wants to release from a feature branch; it overrides both the branch default and `autoRelease=false`
- One-line INFO log fires when the branch default kicks in, naming the current branch and pointing the operator at `--release`
- `master` + `autoRelease=true` (today's default in this repo) is unchanged — CHANGELOG rewrite, tag, push still happen
- Docs: `docs/configuration.md` `autoRelease semantics` paragraph is updated to describe the CLI's behaviour; `docs/running.md` `prompt complete` section documents `--release`; `CHANGELOG.md ## Unreleased` gets a `fix:` bullet
- A new scenario `scenarios/023-prompt-complete-autorelease.md` exercises the three cases (master+autoRelease, feature branch default, feature branch with `--release`) end-to-end against a fresh binary
- Existing scenarios 001 and 016 must continue to pass without modification (master+`autoRelease=true` behaviour preserved)
- All test code uses the existing counterfeiter mocks (`mocks.Releaser`, `mocks.Brancher`) and the Ginkgo v2 style already in `pkg/cmd/prompt_complete_test.go`

</summary>

<objective>
Make `dark-factory prompt complete <id>` honour `cfg.AutoRelease` and add a branch-context safety default (non-`master` defaults to commit-only unless `--release` is passed). The releaser library and daemon executor path are NOT touched. The new behaviour is end-to-end tested by extending `pkg/cmd/prompt_complete_test.go` and by a new scenario `scenarios/023-prompt-complete-autorelease.md`.

</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.

Read the spec end-to-end: `/workspace/specs/in-progress/092-fix-prompt-complete-autorelease.md` — the Goal, Desired Behavior, Constraints, Failure Modes, and Acceptance Criteria sections are the source of truth. Do not re-derive the spec's decisions.

Files to READ end-to-end before writing code:
- `/workspace/pkg/cmd/prompt_complete.go` — the file being modified; the struct (`promptCompleteCommand`), constructor (`NewPromptCompleteCommand`), and `Run`/`completeDirectWorkflow`/`completePRWorkflow` methods. Note that `brancher git.Brancher` is already a field on the struct.
- `/workspace/pkg/cmd/prompt_complete_test.go` — the existing test file. Mirror its Ginkgo v2 + Gomega `Context(...)`/`It(...)` style. Note the `BeforeEach` setup pattern (creates `tempDir`, `queueDir`, `completedDir`, initializes `mocks.CmdPromptManager`, `mocks.Releaser`, `mocks.Brancher`, `mocks.PRCreator`). Note the `makeCmd(pr bool) cmd.PromptCompleteCommand` helper at line 63.
- `/workspace/pkg/factory/factory.go` lines 1231–1254 — the single caller of `NewPromptCompleteCommand`. The factory function `CreatePromptCompleteCommand` (no `cmd` subcommand) is the only construction site for the CLI's prompt-complete command. `cfg.AutoRelease` is already plumbed into the daemon executor at line 434 — do NOT touch that line; it is out of scope. `cfg.PR` is plumbed via the constructor at line 1250 — mirror that pattern for `cfg.AutoRelease`.
- `/workspace/pkg/factory/factory.go` lines 213–226 — `providerDeps` struct holds `prCreator git.PRCreator`, `prMerger git.PRMerger`, `brancher git.Brancher`. The `brancher` field is what `CreatePromptCompleteCommand` will need to use for branch detection (already wired at line 1251).
- `/workspace/pkg/git/brancher.go` lines 161–178 — the `CurrentBranch` method already does `git rev-parse --abbrev-ref HEAD` and returns `(string, error)`. It is part of the `git.Brancher` interface (declared at line 25). Do NOT add a new method; just call this one.
- `/workspace/pkg/git/git.go` lines 86–97 — the `git.Releaser` interface. Do NOT add an `autoRelease` parameter to `NewReleaser` (spec constraint: "The releaser library `pkg/git/git.go` `NewReleaser` does NOT need to grow an `autoRelease` parameter"). The gating decision lives in the command layer.
- `/workspace/main.go` lines 352–358 — the `case "complete":` block inside `runPromptCommand`. It calls `validateOneArg(ctx, args, printPromptHelp)` (which rejects args starting with `-`) and then `factory.CreatePromptCompleteCommand(ctx, cfg, currentDateTimeGetter).Run(ctx, args)`. This is where `--release` flag plumbing lives.
- `/workspace/main.go` lines 318–319 — `printPromptHelp` is the function that prints the `prompt` subcommand help. Update it to mention `--release`.
- `/workspace/main.go` lines 1129–1145 — the current `printPromptHelp` body. Add the `--release` flag to the `complete` line.
- `/workspace/scenarios/016-spec-063-direct-autorelease-regression.md` — the format to mirror for `scenarios/023-prompt-complete-autorelease.md`. Status is `draft` on creation; flip to `active` after first green walk.
- `/workspace/scenarios/helper/lib.sh` lines 41–91 — `build_binary` and `setup_sandbox_copy` helpers used by existing scenarios. `scenario_run_command` (line 127) runs a raw `$BIN` subcommand (no implicit "run"); that's what `prompt complete` needs.
- `/workspace/docs/configuration.md` line 37 — the `autoRelease semantics` paragraph to update.
- `/workspace/docs/running.md` — find the `prompt complete` section (the spec says "in the `prompt complete` section"; verify the actual section heading with `grep -n '^##' docs/running.md` before writing).
- `/workspace/CHANGELOG.md` — add a `## Unreleased` section at the top (or append if present) with a `fix:` bullet.

</context>

<requirements>

## 1. `pkg/cmd/prompt_complete.go` — gate the release path

1.1. Add two new struct fields to `promptCompleteCommand` (placed near the existing `pr` field for locality):
```go
autoRelease  bool
forceRelease bool
```

1.2. Extend `NewPromptCompleteCommand` signature. Append `autoRelease bool, forceRelease bool` AFTER the existing `prCreator git.PRCreator` parameter (mirrors the `cfg.PR` plumbing in `pkg/factory/factory.go:1250`). The full new signature is:
```go
func NewPromptCompleteCommand(
    queueDir string,
    completedDir string,
    promptManager PromptManager,
    releaser git.Releaser,
    pr bool,
    brancher git.Brancher,
    prCreator git.PRCreator,
    autoRelease bool,
    forceRelease bool,
) PromptCompleteCommand
```
The struct literal inside must set both new fields.

1.3. In `Run`, after the `releaser.CommitCompletedFile` call (around line 102) and BEFORE the `if !c.pr { ... } else { ... }` dispatch (around line 106), insert a branch-detection + decision block:
```go
gitCtx := context.WithoutCancel(ctx)

// (existing MoveToCompleted and CommitCompletedFile calls remain unchanged)

// NEW BLOCK — detect branch and decide release vs commit-only.
branch, err := c.brancher.CurrentBranch(gitCtx)
if err != nil {
    return errors.Wrap(ctx, err, "detect current branch")
}
shouldRelease := c.autoRelease && (c.forceRelease || branch == "master")
if !shouldRelease {
    reason := "autoRelease=false"
    if c.autoRelease && !c.forceRelease && branch != "master" {
        reason = fmt.Sprintf("branch is %q, pass --release to force release", branch)
    }
    slog.Info("prompt complete: commit-only", "reason", reason, "branch", branch)
    if err := c.releaser.CommitOnly(gitCtx, title); err != nil {
        return errors.Wrap(ctx, err, "commit")
    }
    fmt.Printf("completed: %s\n", filepath.Base(path))
    return nil
}
```
Use `fmt` and `log/slog` — both are already imported.

1.4. The downstream `if !c.pr { ... } else { ... }` dispatch (around lines 106–114) must now branch on `shouldRelease` for the `!c.pr` (direct) path. **Canonical shape (only) — caller decides, callee never sees the flag:**

Keep the existing `if !c.pr { ... } else { ... }` dispatch as-is and pass `shouldRelease` as a NEW parameter to `completeDirectWorkflow`:
```go
func (c *promptCompleteCommand) completeDirectWorkflow(
    gitCtx, ctx context.Context,
    title string,
    shouldRelease bool,
) error
```
At the top of `completeDirectWorkflow`'s body, early-return on commit-only:
```go
if !shouldRelease {
    if err := c.releaser.CommitOnly(gitCtx, title); err != nil {
        return errors.Wrap(ctx, err, "commit")
    }
    slog.Info("committed changes")
    return nil
}
```
The existing `HasChangelog` / `CommitAndRelease` path below this early-return is reached only when `shouldRelease=true`. Update the call site to pass the precomputed `shouldRelease`. Do NOT re-detect the branch inside `completeDirectWorkflow` — the caller already has it.

1.5. The `completePRWorkflow` is UNCHANGED. PR workflow is not affected by the spec.

1.6. Verify: when `autoRelease=false` AND `branch=master` AND `forceRelease=false` (the master+no-release case from AC line 82), the `Run` method takes the early-return branch-default path (line 1.3 above), calls `releaser.CommitOnly(gitCtx, title)`, returns nil, and never reaches `completeDirectWorkflow`. The test will assert `releaser.CommitAndReleaseCallCount() == 0`.

## 2. `pkg/factory/factory.go` — wire `cfg.AutoRelease`

2.1. Update `CreatePromptCompleteCommand` (lines 1232–1254) to pass `cfg.AutoRelease` as the new `autoRelease` parameter and `false` as the new `forceRelease` parameter. The factory does not have access to a per-invocation `--release` flag — that flag is parsed in `main.go` and threaded into the constructor at a higher layer. The factory always passes `false`; the CLI's `--release` parsing in `main.go` (handled in step 3) overrides this when present.
```go
return cmd.NewPromptCompleteCommand(
    cfg.Prompts.InProgressDir,
    cfg.Prompts.CompletedDir,
    promptManager,
    releaser,
    cfg.PR,
    deps.brancher,
    deps.prCreator,
    cfg.AutoRelease,    // NEW
    false,              // NEW — forceRelease is parsed in main.go
)
```

2.2. Do NOT touch line 434 (the daemon executor's `cfg.AutoRelease` plumbing). The spec constraint is explicit: "The daemon executor path (`pkg/factory/factory.go` around line 434) must NOT be touched by this spec."

## 3. `main.go` — parse `--release` and thread it through

3.1. Add a helper that extracts `--release` from args (presence flag, like `--auto-approve-prompts` at `main.go:684`):
```go
// extractForceRelease removes --release from args and reports whether it was set.
// The flag is a presence flag: its appearance means true. No value argument is consumed.
func extractForceRelease(args []string) (bool, []string) {
    for i, arg := range args {
        if arg != "--release" {
            continue
        }
        remaining := make([]string, 0, len(args)-1)
        remaining = append(remaining, args[:i]...)
        remaining = append(remaining, args[i+1:]...)
        return true, remaining
    }
    return false, args
}
```

3.2. The factory signature `CreatePromptCompleteCommand` currently does not accept a `forceRelease` argument. **Canonical shape (only) — factory takes `forceRelease` as a NEW parameter; argv parsing stays in `main.go`:**

Change `CreatePromptCompleteCommand` to:
```go
func CreatePromptCompleteCommand(
    ctx context.Context,
    cfg config.Config,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    forceRelease bool,
) cmd.PromptCompleteCommand
```
…and pass `forceRelease` as the new argument to `cmd.NewPromptCompleteCommand`. Update the call site at `main.go:356` to:
```go
forceRelease, remaining := extractForceRelease(args)
if err := validateOneArg(ctx, remaining, printPromptHelp); err != nil {
    return err
}
return factory.CreatePromptCompleteCommand(ctx, cfg, currentDateTimeGetter, forceRelease).Run(ctx, remaining)
```
Do NOT move `extractForceRelease` into the factory — the factory must stay agnostic of argv ordering.

3.3. The `--release` flag must be accepted by `validateOneArg` (or the helper you add must run BEFORE `validateOneArg`). Order matters: `extractForceRelease` strips `--release` first, then `validateOneArg` only sees the positional `<id>`. If you forget to strip, `validateOneArg` returns `unknown flag: --release` (see `main.go:507`).

3.4. Update `printPromptHelp` (around `main.go:1129–1145`) to mention `--release` on the `complete` line. The current text is:
```
"  complete <id>   Complete a prompt (triggers commit/push)\n"+
```
Change to:
```
"  complete <id> [--release]   Complete a prompt (commits locally; on master+autoRelease: tag+push; --release forces release on any branch)\n"+
```
Keep the line under 100 chars to satisfy the `golines` linter. If too long, split across two `fmt.Fprintf` calls (mirroring the existing `printRunHelp` style at `main.go:1044–1065`).

## 4. `pkg/cmd/prompt_complete_test.go` — test all four AC cases

4.1. Update the `makeCmd` helper at line 63 to accept the two new parameters and thread them. The existing tests that do NOT care about branch context can keep passing `false, false`. New signature:
```go
makeCmd := func(pr, autoRelease, forceRelease bool) cmd.PromptCompleteCommand {
    return cmd.NewPromptCompleteCommand(
        queueDir,
        completedDir,
        promptManager,
        releaser,
        pr,
        brancher,
        prCreator,
        autoRelease,
        forceRelease,
    )
}
```
Update every existing call to `makeCmd(false)` and `makeCmd(true)` (six call sites — search with `grep -n 'makeCmd(' pkg/cmd/prompt_complete_test.go`) to pass `false, false` for the new parameters. Behavior must be unchanged for these tests — `releaser.CommitOnlyCallCount()` assertions still pass because the existing test contexts (no CHANGELOG) hit the `CommitOnly` branch of `completeDirectWorkflow`.

4.2. In the `BeforeEach` block (around line 34), default `brancher.CurrentBranchStub` to return `"master"` so the existing tests keep passing without modification:
```go
brancher.CurrentBranchStub = func(ctx context.Context) (string, error) {
    return "master", nil
}
```

4.3. Add FOUR new `Context(...)` blocks (one per AC case) at the end of the existing `Describe("PromptCompleteCommand", ...)` block, BEFORE the closing brace. Each one uses a non-master branch name by overriding `brancher.CurrentBranchStub` in its own `BeforeEach` (or inline in the `It` body, mirroring the existing fixture pattern at line 49).

The four cases (verbatim from the spec AC lines 80–83):

**Case A — feature branch + `autoRelease=true` + no `--release` → commit-only (AC line 80):**
```go
Context("feature branch + autoRelease=true + no --release", func() {
    BeforeEach(func() {
        brancher.CurrentBranchStub = func(ctx context.Context) (string, error) {
            return "feature-x", nil
        }
    })
    It("calls CommitOnly and does NOT call CommitAndRelease", func() {
        testFile := filepath.Join(queueDir, "080-test.md")
        err := os.WriteFile(
            testFile,
            []byte("---\nstatus: pending_verification\n---\n# Test\n"),
            0600,
        )
        Expect(err).NotTo(HaveOccurred())

        promptManager.MoveToCompletedReturns(nil)
        releaser.CommitCompletedFileReturns(nil)
        releaser.HasChangelogReturns(false)
        releaser.CommitOnlyReturns(nil)

        err = makeCmd(false, true, false).Run(ctx, []string{"080-test.md"})
        Expect(err).NotTo(HaveOccurred())

        Expect(releaser.CommitOnlyCallCount()).To(Equal(1))
        Expect(releaser.CommitAndReleaseCallCount()).To(Equal(0))
    })
})
```

**Case B — feature branch + `autoRelease=true` + `--release` → release fires (AC line 81):**
```go
Context("feature branch + autoRelease=true + --release flag", func() {
    BeforeEach(func() {
        brancher.CurrentBranchStub = func(ctx context.Context) (string, error) {
            return "feature-x", nil
        }
    })
    It("calls CommitAndRelease", func() {
        testFile := filepath.Join(queueDir, "080-test.md")
        err := os.WriteFile(
            testFile,
            []byte("---\nstatus: pending_verification\n---\n# Test\n"),
            0600,
        )
        Expect(err).NotTo(HaveOccurred())

        // Create a CHANGELOG.md in tempDir and change to it
        changelogContent := "# Changelog\n\n## Unreleased\n\n- feat: add something new\n\n## v1.0.0\n\n- fix: old fix\n"
        origDir, err := os.Getwd()
        Expect(err).NotTo(HaveOccurred())
        err = os.WriteFile(
            filepath.Join(tempDir, "CHANGELOG.md"),
            []byte(changelogContent),
            0600,
        )
        Expect(err).NotTo(HaveOccurred())
        err = os.Chdir(tempDir)
        Expect(err).NotTo(HaveOccurred())
        defer func() { _ = os.Chdir(origDir) }()

        promptManager.MoveToCompletedReturns(nil)
        releaser.CommitCompletedFileReturns(nil)
        releaser.HasChangelogReturns(true)
        releaser.CommitAndReleaseReturns(nil)

        err = makeCmd(false, true, true).Run(ctx, []string{"080-test.md"})
        Expect(err).NotTo(HaveOccurred())

        Expect(releaser.CommitAndReleaseCallCount()).To(Equal(1))
        Expect(releaser.CommitOnlyCallCount()).To(Equal(0))
    })
})
```

**Case C — `master` + `autoRelease=false` + no `--release` → commit-only (AC line 82):**
```go
Context("master + autoRelease=false + no --release", func() {
    BeforeEach(func() {
        brancher.CurrentBranchStub = func(ctx context.Context) (string, error) {
            return "master", nil
        }
    })
    It("calls CommitOnly and does NOT call CommitAndRelease", func() {
        testFile := filepath.Join(queueDir, "080-test.md")
        err := os.WriteFile(
            testFile,
            []byte("---\nstatus: pending_verification\n---\n# Test\n"),
            0600,
        )
        Expect(err).NotTo(HaveOccurred())

        promptManager.MoveToCompletedReturns(nil)
        releaser.CommitCompletedFileReturns(nil)
        releaser.HasChangelogReturns(true)
        releaser.CommitOnlyReturns(nil)

        err = makeCmd(false, false, false).Run(ctx, []string{"080-test.md"})
        Expect(err).NotTo(HaveOccurred())

        Expect(releaser.CommitOnlyCallCount()).To(Equal(1))
        Expect(releaser.CommitAndReleaseCallCount()).To(Equal(0))
    })
})
```

**Case D — `master` + `autoRelease=true` + no `--release` → release fires (regression check, AC line 83):**
```go
Context("master + autoRelease=true + no --release", func() {
    BeforeEach(func() {
        brancher.CurrentBranchStub = func(ctx context.Context) (string, error) {
            return "master", nil
        }
    })
    It("calls CommitAndRelease (regression: master+autoRelease=true unchanged)", func() {
        testFile := filepath.Join(queueDir, "080-test.md")
        err := os.WriteFile(
            testFile,
            []byte("---\nstatus: pending_verification\n---\n# Test\n"),
            0600,
        )
        Expect(err).NotTo(HaveOccurred())

        // Create a CHANGELOG.md in tempDir and change to it
        changelogContent := "# Changelog\n\n## Unreleased\n\n- feat: add something new\n\n## v1.0.0\n\n- fix: old fix\n"
        origDir, err := os.Getwd()
        Expect(err).NotTo(HaveOccurred())
        err = os.WriteFile(
            filepath.Join(tempDir, "CHANGELOG.md"),
            []byte(changelogContent),
            0600,
        )
        Expect(err).NotTo(HaveOccurred())
        err = os.Chdir(tempDir)
        Expect(err).NotTo(HaveOccurred())
        defer func() { _ = os.Chdir(origDir) }()

        promptManager.MoveToCompletedReturns(nil)
        releaser.CommitCompletedFileReturns(nil)
        releaser.HasChangelogReturns(true)
        releaser.CommitAndReleaseReturns(nil)

        err = makeCmd(false, true, false).Run(ctx, []string{"080-test.md"})
        Expect(err).NotTo(HaveOccurred())

        Expect(releaser.CommitAndReleaseCallCount()).To(Equal(1))
        _, bump := releaser.CommitAndReleaseArgsForCall(0)
        Expect(bump).To(Equal(git.MinorBump))
        Expect(releaser.CommitOnlyCallCount()).To(Equal(0))
    })
})
```

4.4. The `mocks.Brancher` is already wired in the existing test's `BeforeEach` (line 53). Do NOT add a new mock — the existing one is sufficient. `brancher.CurrentBranchStub` is the counterfeiter-generated override hook (visible in `/workspace/mocks/brancher.go:38-40`).

4.5. **DELETE the existing `pending_verification + CHANGELOG` test block at `pkg/cmd/prompt_complete_test.go:188–225`.** New Case D (added in step 4.4) is the canonical `master + autoRelease=true + CHANGELOG → CommitAndRelease` regression check; keeping the old block alongside Case D duplicates the assertion. Locate it via `grep -n 'HasChangelogReturns(true)' pkg/cmd/prompt_complete_test.go` — there is exactly one such block in the non-PR section; remove its enclosing `Context(...)` / `It(...)` wholesale.

For every remaining call to `makeCmd(false)` / `makeCmd(true)` (use `grep -n 'makeCmd(' pkg/cmd/prompt_complete_test.go` to enumerate), rewrite as `makeCmd(false, false, false)` / `makeCmd(true, false, false)`. These remaining call sites hit early-return paths (`no args`, `prompt not found`, `approved`, `failed`, `executing`, `in_review`, `pending_verification + no CHANGELOG`) and never reach the gate, so the `autoRelease=false` default is harmless.

PR-workflow tests (`makeCmd(true, ...)`): unchanged behaviour — the gate is only on the direct path (step 1.5).

4.6. After all updates, run `cd /workspace && go test ./pkg/cmd/...` and confirm green. The new tests must all pass. The existing tests must all still pass.

## 5. `docs/configuration.md` — update the `autoRelease semantics` paragraph

5.1. At line 37, the current paragraph reads:
```
`autoRelease` semantics: When `false` (default), commits stay local (no push, no tag). When `true` and `CHANGELOG.md` exists, commits are pushed AND `## Unreleased` is bumped to `## vX.Y.Z` with a tag pushed. When `true` without `CHANGELOG.md`, commits are pushed but no tag is created. Works in all workflows.
```

Replace with:
```
`autoRelease` semantics: When `false` (default), commits stay local (no push, no tag). When `true` and `CHANGELOG.md` exists, commits are pushed AND `## Unreleased` is bumped to `## vX.Y.Z` with a tag pushed. When `true` without `CHANGELOG.md`, commits are pushed but no tag is created. Works in all workflows.

`dark-factory prompt complete <id>` honours `autoRelease` and adds a branch-context safety default: on any non-`master` branch, completion commits but does NOT release, regardless of `autoRelease`, unless the operator passes `--release` explicitly. The flag overrides both the branch default and `autoRelease=false`. See [running.md § prompt complete --release](running.md#prompt-complete---release) for the operator-facing description.
```

Keep the existing paragraph intact; APPEND the new sentence after it. Do NOT delete or rephrase the original text.

## 6. `docs/running.md` — document `--release`

6.1. Find the `prompt complete` section. The current `running.md` does NOT have a dedicated `## prompt complete` section — find the right insertion point by searching for `prompt complete` in the file (line ~24 and the CLI reference at the bottom). Insert a new section immediately after the `## Workflow: Direct` section (line ~178), titled:

```
## prompt complete --release

`dark-factory prompt complete <id>` completes the prompt, commits the file move, and then either releases or commits-only based on the current state.

| State | `autoRelease` | Branch | Result |
|-------|---------------|--------|--------|
| `--release` passed | any | any | Full release: CHANGELOG rewrite, tag, push |
| not set | `false` (or `--set autoRelease=false`) | any | Commit-only — no push, no tag, no CHANGELOG rewrite |
| not set | `true` | `master` | Full release (today's behaviour) |
| not set | `true` | non-master | Commit-only with INFO log: `branch is "<name>", pass --release to force release` |

The branch-context default prevents accidental version bumps on multi-prompt feature branches — the operator only releases once, on `master`, after merge. To release from a non-`master` branch, pass `--release` explicitly.
```

If you cannot find a clean insertion point, add it just before `## Versioning` (line ~192). The exact heading text is the spec's discretion — match the existing section style (`## Section Name` with a blank line before and after).

6.2. Verify with `grep -n -- '--release' docs/running.md` — expect ≥1.

## 7. `CHANGELOG.md` — add `## Unreleased` bullet

7.1. Read `/workspace/CHANGELOG.md` top 5 lines to determine if `## Unreleased` already exists. If it does, APPEND a bullet. If it does NOT exist, add a new `## Unreleased` section at the top (between line 6 and the first versioned section at line 11). The bullet:

```
- fix(prompt): `dark-factory prompt complete <id>` now honours `cfg.AutoRelease` and the new `--release` flag. By default, completion on a non-master branch is commit-only (no CHANGELOG rewrite, no tag, no push) regardless of `autoRelease`; pass `--release` to force release on any branch. The daemon executor path is unchanged.
```

Prefix is `fix:` (the spec calls it "a fix to a regression"; the user-visible behaviour change is incidental).

7.2. Verify with `awk '/^## /{p=0} /^## Unreleased/{p=1} p' /workspace/CHANGELOG.md | grep -c -- '--release'` — expect ≥1.

## 8. `scenarios/023-prompt-complete-autorelease.md` — new end-to-end scenario

8.1. Create the file. Use scenario 016 as the structural template (`/workspace/scenarios/016-spec-063-direct-autorelease-regression.md`). Status `draft`. Frontmatter:
```yaml
---
status: draft
---
```

8.2. Body. The scenario exercises the three cases the spec's AC line 87 requires. Use the helper `setup_sandbox_copy` (from `/workspace/scenarios/helper/lib.sh`) for the shared setup, then `git checkout -b feature-x` for the non-master cases, then `scenario_run_command` (line 127) to call `$BIN prompt complete <id>` directly. After the first green walk, flip the frontmatter to `status: active`.

Structure (mirror scenario 016's Setup/Action/Expected/Cleanup sectioning):

```markdown
# Scenario 023: prompt complete honours autoRelease + branch context + --release

Validates that the fix from spec 092 prompt 1 makes `dark-factory prompt complete <id>` honour `autoRelease` end-to-end and add a branch-context safety default (non-master defaults to commit-only unless `--release` is passed).

## Setup

- [ ] Build the freshly-modified binary: `go build -C /workspace -o /tmp/new-dark-factory .`
- [ ] Create sandbox (master branch): `WORK_DIR=$(mktemp -d) && cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/sandbox" && cd "$WORK_DIR/sandbox"`
- [ ] `.dark-factory.yaml` set to `workflow: direct` + `autoRelease: true` + `validationCommand: ""` + `validationPrompt: ""`
- [ ] Sandbox repo has a `CHANGELOG.md` with at least one prior version entry
- [ ] One trivial prompt approved in `prompts/in-progress/` (e.g. updates a README line)

## Case A — master + autoRelease=true → release fires (regression check)

This case asserts that today's master+`autoRelease=true` behaviour is preserved. If this case fails, scenarios 001 and 016 will also fail.

### Action

- [ ] Confirm current branch is `master`: `git rev-parse --abbrev-ref HEAD` returns `master`
- [ ] Capture state: `BEFORE_TAG=$(git describe --tags --abbrev=0)` and `BEFORE_HEAD=$(git rev-parse HEAD)`
- [ ] Run: `/tmp/new-dark-factory prompt complete <id>`

### Expected

- [ ] Exit code 0
- [ ] `git rev-parse HEAD` is different from `BEFORE_HEAD` — a new commit landed
- [ ] `git describe --tags --abbrev=0` is different from `BEFORE_TAG` — a new tag was created
- [ ] CHANGELOG.md no longer contains `## Unreleased` (it was rewritten to `## vX.Y.Z`)

## Case B — feature branch + autoRelease=true + no --release → no release (the new safety default)

This case is the core fix. Without it, every multi-prompt feature branch cuts an orphan tag.

### Action

- [ ] Create a feature branch: `git checkout -b dark-factory/test-no-release`
- [ ] Approve one more prompt: `/tmp/new-dark-factory prompt approve <id2>`
- [ ] Wait for daemon completion, or run directly: `/tmp/new-dark-factory prompt complete <id2>`
- [ ] Capture state: `BEFORE_TAG=$(git describe --tags --abbrev=0)` and `BEFORE_HEAD=$(git rev-parse HEAD)`

### Expected

- [ ] Exit code 0
- [ ] INFO log line on stderr contains: `branch is "dark-factory/test-no-release", pass --release to force release`
- [ ] `git rev-parse HEAD` is different from `BEFORE_HEAD` — a new commit landed
- [ ] `git describe --tags --abbrev=0` is EQUAL to `BEFORE_TAG` — NO new tag was created
- [ ] CHANGELOG.md STILL contains `## Unreleased` (it was NOT rewritten)

## Case C — feature branch + --release → release fires (explicit opt-in)

This case confirms `--release` overrides the branch default.

### Action

- [ ] Stay on the feature branch: `git rev-parse --abbrev-ref HEAD` returns `dark-factory/test-no-release`
- [ ] Approve one more prompt: `/tmp/new-dark-factory prompt approve <id3>`
- [ ] Run with the flag: `/tmp/new-dark-factory prompt complete <id3> --release`
- [ ] Capture state: `BEFORE_TAG=$(git describe --tags --abbrev=0)` and `BEFORE_HEAD=$(git rev-parse HEAD)`

### Expected

- [ ] Exit code 0
- [ ] `git rev-parse HEAD` is different from `BEFORE_HEAD` — a new commit landed
- [ ] `git describe --tags --abbrev=0` is different from `BEFORE_TAG` — a new tag was created (override worked)
- [ ] CHANGELOG.md no longer contains `## Unreleased`

## Cleanup

```bash
cd ~
rm -rf "$WORK_DIR"
```
```

8.3. Verify the scenario file with `grep -c 'master\|feature\|--release' /workspace/scenarios/023-prompt-complete-autorelease.md` — expect ≥3.

## 9. Verification

9.1. Run from the repo root:
```bash
cd /workspace && make test
```
All tests must pass.

9.2. Run the four spec-AC grep checks from `/workspace/specs/in-progress/092-fix-prompt-complete-autorelease.md` lines 76–87:
```bash
cd /workspace
# AC: --help output contains --release
go build -o /tmp/new-dark-factory . && /tmp/new-dark-factory prompt complete --help 2>&1 | grep -c -- '--release'
# AC: prompt_complete.go reads autoRelease and --release
grep -n 'autoRelease\|--release\|forceRelease' pkg/cmd/prompt_complete.go
# AC: factory passes cfg.AutoRelease
grep -n -A2 'CreatePromptCompleteCommand' pkg/factory/factory.go | grep -c 'AutoRelease'
# AC: branch-context default test exists
grep -n 'TestPromptComplete.*NonMaster\|TestPromptComplete.*Branch\|TestCompleteOnNonMasterBranch\|feature branch + autoRelease' pkg/cmd/prompt_complete_test.go
# AC: --release override test exists
grep -n 'release_flag\|--release flag' pkg/cmd/prompt_complete_test.go
# AC: master + autoRelease=false test exists
grep -n 'master + autoRelease=false' pkg/cmd/prompt_complete_test.go
# AC: master + autoRelease=true regression test exists
grep -n 'master + autoRelease=true\|regression' pkg/cmd/prompt_complete_test.go
# AC: configuration.md updated
grep -n -A2 'autoRelease semantics' docs/configuration.md
# AC: running.md documents --release
grep -n -- '--release' docs/running.md
# AC: CHANGELOG.md has bullet
awk '/^## /{p=0} /^## Unreleased/{p=1} p' CHANGELOG.md | grep -c -- '--release'
# AC: scenario 023 exists with three cases
grep -c 'master\|feature\|--release' scenarios/023-prompt-complete-autorelease.md
```
All checks must return non-zero counts (the exact `≥N` thresholds are in the spec).

9.3. Run `cd /workspace && make precommit` — must exit 0. If it fails, fix the issue and re-run ONLY the failing target (e.g. `make lint`, `make gosec`, `make errcheck`).

9.4. Build the binary so the operator's scenario walks (scenarios 001/016/023, run outside this container) can use it:
```bash
cd /workspace && go build -o /tmp/new-dark-factory .
```
Do NOT walk the scenarios in this container — scenario walks require operator-driven git/branch state and live in the spec-verification ladder, not in container execution.

</requirements>

<constraints>

- Use the existing counterfeiter mocks: `mocks.Releaser` (`/workspace/mocks/releaser.go`) and `mocks.Brancher` (`/workspace/mocks/brancher.go`). Do NOT generate a new mock.
- Use Ginkgo v2 + Gomega for all new tests. Use the existing `Context(...)`/`It(...)` style; do NOT introduce `DescribeTable` for the four AC cases (the existing file uses bare `It` blocks).
- All new tests in `pkg/cmd` go in `pkg/cmd/prompt_complete_test.go` (extend, do NOT create a new file). External test package `package cmd_test`.
- Do NOT modify the `git.Releaser` interface or the `releaser` struct. The spec constraint forbids it: "The releaser library `pkg/git/git.go` `NewReleaser` does NOT need to grow an `autoRelease` parameter."
- Do NOT touch `pkg/factory/factory.go` line 434 (the daemon executor's `cfg.AutoRelease` wiring). The spec constraint is explicit.
- Do NOT touch `pkg/factory/factory.go` lines 580–600 (the daemon-side `CreateOneShotRunner` / `CreateRunner` path). Same reason.
- The literal string `"master"` is hardcoded. If the spec author later needs `main` as the default branch, that's a follow-up spec — not this one.
- Branch detection uses `git.Brancher.CurrentBranch`, which already runs `git rev-parse --abbrev-ref HEAD`. Do NOT add a new brancher method.
- Do NOT commit — dark-factory handles git.
- Do NOT add new dependencies to `go.mod`.
- Do NOT change the daemon's behaviour or the executor's autoRelease handling.
- The `autoRelease` source is `cfg.AutoRelease` (already plumbed by `applyGlobalOverrides` at `main.go:567–569` and `applySetOverrides` at `main.go:843–849`). Do NOT add a new config field or new `--set` key.

</constraints>

<verification>
Run from `/workspace`:

```
cd /workspace && make precommit
```

must exit 0.

Scenario walks (001, 016, 023) are operator-driven and live in the spec-verification ladder, not container execution. This prompt is responsible for *creating* `scenarios/023-prompt-complete-autorelease.md` (step 8) so the operator can walk it later; the prompt does NOT walk it. Build the binary at `/tmp/new-dark-factory` (step 9.4) so the operator's walk has a binary to invoke.

Spot checks (run after `make precommit` is green):
```bash
cd /workspace
grep -c 'autoRelease\|--release\|forceRelease' pkg/cmd/prompt_complete.go                                # expect >= 5
grep -c 'feature branch + autoRelease' pkg/cmd/prompt_complete_test.go                                  # expect >= 1
grep -c 'master + autoRelease' pkg/cmd/prompt_complete_test.go                                          # expect >= 2
grep -c 'cfg.AutoRelease\|AutoRelease,' pkg/factory/factory.go                                           # expect >= 3 (one is daemon, one is prompt complete)
grep -c -- '--release' main.go                                                                          # expect >= 2 (extractForceRelease, printPromptHelp)
grep -c 'autoRelease semantics' docs/configuration.md                                                    # expect >= 1
grep -c -- '--release' docs/running.md                                                                  # expect >= 1
awk '/^## /{p=0} /^## Unreleased/{p=1} p' CHANGELOG.md | grep -c -- '--release'                          # expect >= 1
test -f scenarios/023-prompt-complete-autorelease.md && echo present || echo MISSING                     # expect present
grep -c 'master\|feature\|--release' scenarios/023-prompt-complete-autorelease.md                        # expect >= 3
```

If any spot check fails, fix the gap before declaring done.

</verification>
