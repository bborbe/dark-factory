---
status: committing
summary: Added PushBranch to Releaser interface, implemented push at workflow boundary in direct executor and recovery path, threaded autoRelease into NewRecoverer, regenerated mocks, and added 4-row matrix tests for both direct executor and recoverer
container: dark-factory-356-autorelease-push-move-prompt-commit
dark-factory-version: v0.137.0-1-g310a15c6
created: "2026-04-30T19:39:12Z"
queued: "2026-04-30T19:42:47Z"
started: "2026-04-30T19:42:49Z"
---

# autoRelease: push every commit, not just the release

<summary>
- `autoRelease=true` is documented as "push everything after each prompt completes", but the current implementation only pushes when a CHANGELOG.md exists (the release path).
- Two orthogonal concerns are conflated: pushing (controlled by `autoRelease`) and tagging/version-bumping (controlled by CHANGELOG presence). Push lives only inside `CommitAndRelease`, the tag-producing path.
- Result: in two scenarios the daemon commits locally and never pushes, even with `autoRelease=true`:
  1. Project **with** CHANGELOG: the Phase 4 "move prompt to completed" commit is committed but never pushed (the release commit + tag are pushed correctly).
  2. Project **without** CHANGELOG: Phase 1's `CommitOnly` work commit AND Phase 4's prompt-move commit both stay local.
- Empirically observed: after the `bborbe/run` migration prompt completed under `autoRelease=true`, the release commit `release v1.9.23` and tag `v1.9.23` were pushed, but commit `3ad2fa2 move prompt to completed` remained local until manually pushed.
- The recovery path (`pkg/committingrecoverer/recoverer.go`) has the same gap and must be fixed consistently.
</summary>

<objective>
After this change, `autoRelease=true` produces a remote-equivalent local state for every successful prompt completion: every commit produced by the workflow is pushed to the remote, regardless of whether the project has a CHANGELOG. `autoRelease=false` continues to produce local-only commits (no push). Tagging behavior is unchanged: tags are produced only when CHANGELOG exists, and if produced they are pushed.
</objective>

<context>
**Mental model — two orthogonal concerns:**

| `autoRelease` | CHANGELOG.md | Expected behavior |
|---------------|--------------|-------------------|
| false         | *            | local commits only, no push, no tag |
| true          | absent       | commit + push (no tag) |
| true          | present      | commit + push + tag + push tag |

The bug today: push only happens on the `autoRelease=true` + CHANGELOG path, because push is implemented inside `CommitAndRelease`. Push needs to be lifted out of the commit primitives and gated on `autoRelease` at the workflow boundary.

**Relevant code:**

- `pkg/git/git.go:92-99` — `Releaser` interface (`GetNextVersion`, `CommitAndRelease`, `CommitCompletedFile`, `CommitOnly`, `HasChangelog`, `MoveFile`).
- `pkg/git/git.go:159-203` — `CommitAndRelease` does commit → tag → `gitPush` → `gitPushTag`. **Push lives here only.**
- `pkg/git/git.go:208-234` — `CommitCompletedFile` does add → status check → commit. **No push.**
- `pkg/git/git.go:130-144` — `CommitOnly` does add-all → commit. **No push.**
- `pkg/git/git.go:445-455` — `gitPush(ctx) error` package-private helper.
- `pkg/processor/workflow_helpers.go:114-152` — `handleDirectWorkflow` decides between `CommitOnly` (no changelog OR `autoRelease=false`) and `CommitAndRelease` (changelog AND `autoRelease=true`).
- `pkg/processor/workflow_executor_direct.go:62-103` — `directWorkflowExecutor.completeCommit` runs Phase 1 (`handleDirectWorkflow`) and Phase 4 (`Releaser.CommitCompletedFile`).
- `pkg/processor/workflow_helpers.go` — `WorkflowDeps` already exposes `AutoRelease` and `Releaser`. No new plumbing into the direct executor needed.
- `pkg/committingrecoverer/recoverer.go:43-62` — `Recoverer` does **not** currently know about `autoRelease`. Needs to be threaded in via `NewRecoverer`.
- `pkg/committingrecoverer/recoverer.go:104-130` — recovery commits via `git.CommitAll` (Phase 1 equivalent) and `releaser.CommitCompletedFile` (Phase 4 equivalent). Same push gap.
- `pkg/config/config.go` — `AutoRelease bool` config field already exists.

**Design choice — push at the workflow boundary, not inside the commit primitive:**

- Single source of truth: "autoRelease=true ⇒ branch is pushed at end of workflow", independent of changelog and which commit primitive ran.
- `CommitOnly`, `CommitCompletedFile`, `CommitAndRelease` stay focused on "make a commit". Tagging stays inside `CommitAndRelease` (a tag is a release artifact, tightly coupled to the version bump).
- One push call covers both the work commit and the prompt-move commit (`git push` is idempotent — pushing again with no new commits exits zero with "Everything up-to-date").
- `CommitAndRelease`'s existing push stays. After the fix, the workflow-level push runs after Phase 4 and is a no-op if Phase 1 already pushed via `CommitAndRelease`. This keeps tag-push atomic with the release commit.

**Existing test infrastructure:**

- `pkg/git/git_test.go` — table-driven tests for releaser methods.
- `pkg/processor/processor_internal_test.go` — `handleDirectWorkflow` Describe; `stubReleaser` (line 241) is the minimal stub.
- `pkg/processor/processor_automerge_test.go` — autoRelease=true vs false coverage at the processor level.
- `mocks/releaser.go` — counterfeiter-generated; will need regeneration after interface change.
</context>

<requirements>

## 1. Add `PushBranch` to the `Releaser` interface

`pkg/git/git.go`:

```go
type Releaser interface {
    GetNextVersion(ctx context.Context, bump VersionBump) (string, error)
    CommitAndRelease(ctx context.Context, bump VersionBump) error
    CommitCompletedFile(ctx context.Context, path string) error
    CommitOnly(ctx context.Context, message string) error
    HasChangelog(ctx context.Context) bool
    MoveFile(ctx context.Context, oldPath string, newPath string) error
    PushBranch(ctx context.Context) error
}
```

Add the implementation on `releaser`:

```go
// PushBranch pushes the current branch's commits to the remote.
// Idempotent: pushing with no new commits exits zero ("Everything up-to-date").
func (r *releaser) PushBranch(ctx context.Context) error {
    return gitPush(ctx)
}
```

Regenerate `mocks/releaser.go` (counterfeiter). Follow whatever `make generate` / existing convention does in this repo.

## 2. Push at the workflow boundary in the direct executor

`pkg/processor/workflow_executor_direct.go`, in `completeCommit`, **after** Phase 4's `CommitWithRetry`:

```go
// Phase 5: push the branch when autoRelease is enabled.
// Single push covers both Phase 1's work commit (when CommitOnly was used)
// and Phase 4's prompt-move commit. Idempotent with CommitAndRelease's
// internal push (changelog path).
if e.deps.AutoRelease {
    if err := git.CommitWithRetry(gitCtx, git.DefaultCommitBackoff, func(retryCtx context.Context) error {
        return e.deps.Releaser.PushBranch(retryCtx)
    }); err != nil {
        return errors.Wrap(ctx, err, "push branch")
    }
}
```

Do **not** modify Phases 1–4. Push gating lives only at this boundary.

## 3. Apply the same push to the recovery path

### 3a. Thread `autoRelease` into `Recoverer`

`pkg/committingrecoverer/recoverer.go`:

- Add `autoRelease bool` field to `recoverer` struct.
- Add `autoRelease bool` parameter to `NewRecoverer` (place after `completedDir` to match field order; update doc comment).

### 3b. Update the call site of `NewRecoverer`

Find the construction site with:

```bash
grep -rn "committingrecoverer.NewRecoverer\|NewRecoverer(" pkg/factory/ pkg/processor/ main.go
```

Pass `cfg.AutoRelease` as the new argument.

### 3c. Push at the end of `Recover`

In `pkg/committingrecoverer/recoverer.go` `Recover`, **after** the Phase-4-equivalent `CommitWithRetry` block (the `releaser.CommitCompletedFile` call ~line 126), add:

```go
if r.autoRelease {
    if err := git.CommitWithRetry(gitCtx, git.DefaultCommitBackoff, func(retryCtx context.Context) error {
        return r.releaser.PushBranch(retryCtx)
    }); err != nil {
        return errors.Wrap(ctx, err, "push branch during recovery")
    }
}
```

## 4. Tests — `pkg/git/git_test.go`

Add a unit test for `releaser.PushBranch`:

- Calls `gitPush` (verify by stubbing `exec.Command` if the existing test pattern does so, OR by running against a temp repo with a local bare remote — match whichever pattern existing `gitPush`-related tests use).
- Returns wrapped error on failure.

## 5. Tests — direct executor

Find the existing direct-executor test file with:

```bash
grep -rln "directWorkflowExecutor\|completeCommit" pkg/processor/*_test.go
```

Add table-driven cases covering all 4 rows of the matrix:

| autoRelease | CHANGELOG | Expected `PushBranch` calls | Expected `CommitAndRelease` calls | Expected `CommitOnly` calls |
|---|---|---|---|---|
| false | absent | 0 | 0 | 1 |
| false | present | 0 | 0 | 1 |
| true | absent | 1 | 0 | 1 |
| true | present | 1 | 1 | 0 |

Use the counterfeiter `mocks.Releaser` (or extend `stubReleaser` in `processor_internal_test.go:241` to count `PushBranch` calls).

## 6. Tests — `pkg/committingrecoverer/recoverer_test.go`

Add table-driven cases for the recovery path covering the same 4-row matrix.

## 7. No changes to `CommitAndRelease`

`CommitAndRelease`'s internal `gitPush` + `gitPushTag` remain. After the fix, the workflow-level push runs after `CommitAndRelease`'s push and is a no-op (idempotent). This keeps the tag push atomic with the release commit and minimizes churn in well-tested code.

## 8. CHANGELOG entry

Add to `CHANGELOG.md` under `## Unreleased`:

```
- fix: autoRelease now pushes the branch on every prompt completion, not only on the release path. Previously, the post-release "move prompt to completed" commit and the no-CHANGELOG work commit stayed local.
```

</requirements>

<verification>

`make precommit` must exit 0.

Spot checks:

```bash
grep -n "PushBranch" pkg/git/git.go                                    # interface + impl
grep -n "PushBranch" pkg/processor/workflow_executor_direct.go         # workflow-level push gate
grep -n "PushBranch" pkg/committingrecoverer/recoverer.go              # recovery-level push gate
grep -n "autoRelease bool" pkg/committingrecoverer/recoverer.go        # threaded into struct
grep -rn "committingrecoverer.NewRecoverer" pkg/ main.go               # call site updated
grep -n "PushBranch" mocks/releaser.go                                 # counterfeiter regenerated
```

End-to-end behavior (manual smoke):

| Scenario | Expected after `dark-factory run` completes |
|---|---|
| autoRelease=false, CHANGELOG present | `git rev-list @{u}..HEAD --count` > 0 (commits stayed local) |
| autoRelease=false, no CHANGELOG | `git rev-list @{u}..HEAD --count` > 0 |
| autoRelease=true, CHANGELOG present | `git rev-list @{u}..HEAD --count` == 0; tag pushed |
| autoRelease=true, no CHANGELOG | `git rev-list @{u}..HEAD --count` == 0; no tag |

</verification>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT change `go.mod` / `go.sum` / `vendor/`.
- Do NOT change `CommitAndRelease`'s push behavior. Its tag-push must stay atomic with the release commit.
- Do NOT add push logic inside `CommitOnly` or `CommitCompletedFile`. Push lives at the workflow boundary only.
- Do NOT change the documented contract for `autoRelease=false`: it must continue to produce local-only commits with no push, regardless of CHANGELOG.
- Use `errors.Wrap` / `errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf`.
- Reuse the existing `gitPush` helper via the new `PushBranch` method; don't duplicate the subprocess call.
- Don't add retry logic inside `PushBranch` itself — `CommitWithRetry` already wraps the call site, matching the existing release flow's retry shape.
- Preserve the `len(strings.TrimSpace(string(output))) == 0` early-return in `CommitCompletedFile` (no commit when nothing to commit).
- The branch/PR workflows are out of scope. They already push the feature branch via `Brancher.Push`; the merge happens on the remote.
- Counterfeiter mocks must be regenerated; do not hand-edit `mocks/releaser.go`.
</constraints>
