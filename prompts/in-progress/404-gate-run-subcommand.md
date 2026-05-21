---
status: approved
spec: [084-fail-fast-on-worktree-without-hidegit]
created: "2026-05-22T00:50:00Z"
queued: "2026-05-21T22:57:16Z"
branch: dark-factory/fail-fast-on-worktree-without-hidegit
---

<summary>
- `dark-factory run` (the one-shot subcommand) currently bypasses the worktree/submodule gate because `oneShotRunner.Run` does not call `checkGitSafety` — only the daemon `runner.Run` does
- Spec 084 AC6 explicitly requires: "The same gating applies to `dark-factory run`"
- Fix: extract `checkGitSafety` from a `runner` method to a package-level function `CheckGitSafety(ctx, hideGit)`, then call it from BOTH `runner.Run` (daemon path) and `oneShotRunner.Run` (one-shot path)
- Add a unit/integration test that exercises `dark-factory run` semantics from a worktree CWD with `hideGit=false` and asserts the gate fires
</summary>

<objective>
Close spec 084 AC6: make the worktree/submodule gate also apply to `dark-factory run`, not just `dark-factory daemon`. Extract `checkGitSafety` to a package-level function so both runners share one implementation, and add a test that proves the one-shot path is gated.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `specs/in-progress/084-fail-fast-on-worktree-without-hidegit.md` — AC6 (around line 88): "The same gating applies to `dark-factory run` — evidence: repeating the worktree+`hideGit=false` invocation with the `run` subcommand exits non-zero with the same error substring."
Read `pkg/runner/runner.go` lines 263-281 — current `checkGitSafety` is an unexported method on `*runner`; called from line 152 in `runner.Run`.
Read `pkg/runner/oneshot.go` lines 78-121 — `oneShotRunner` struct + `Run` method. Note the struct has fields like `locker`, `processor`, etc. and acquires the lock at line 98 but does NOT call `checkGitSafety`. The struct currently has no `hideGit` field — needs adding.
Read `pkg/runner/runner.go` to find how `runner` is constructed and where `hideGit` comes in (likely a constructor parameter or factory wiring).
Read `pkg/runner/runner_test.go` lines 1300-1394 — existing `Describe("worktree gating", ...)` block for the daemon path, as the canonical pattern for the new one-shot gating test.
</context>

<requirements>

1. **Extract `checkGitSafety` to a package-level function** in `pkg/runner/runner.go`:

   Replace the existing method (lines 263-281):
   ```go
   func (r *runner) checkGitSafety(ctx context.Context) error {
       if r.hideGit {
           return nil
       }
       // ... rest of body
   }
   ```

   With an exported package-level function:
   ```go
   // CheckGitSafety verifies git safety conditions before starting either
   // the daemon or a one-shot run:
   //  1. Refuse to start from a worktree or submodule CWD — the .git pointer
   //     points to the parent repo's worktrees/ directory, which is not mounted.
   //  2. Abort if .git/index.lock exists — all git operations will fail.
   // Skips both checks when hideGit is true (the operator has explicitly opted
   // into the .git mask, so the worktree pointer is hidden anyway).
   func CheckGitSafety(ctx context.Context, hideGit bool) error {
       if hideGit {
           return nil
       }
       if err := DetectWorktreeOrSubmodule(ctx); err != nil {
           return errors.Wrap(ctx, err, "worktree/submodule CWD detected")
       }
       if _, err := os.Stat(filepath.Join(".", ".git", "index.lock")); err == nil {
           return errors.Errorf(
               ctx,
               ".git/index.lock exists — remove it before starting the daemon (another git process may be running)",
           )
       }
       return nil
   }
   ```

   Update the call site at `pkg/runner/runner.go:152` (inside `runner.Run`) to call `CheckGitSafety(ctx, r.hideGit)` instead of `r.checkGitSafety(ctx)`.

2. **Add `hideGit` to `oneShotRunner` and call the gate** in `pkg/runner/oneshot.go`:

   a) Add `hideGit bool` field to the `oneShotRunner` struct (in the field list around line 80-92). Place it near the other config-derived booleans like `autoApprove`.

   b) In `pkg/runner/factory.go` (or wherever `oneShotRunner` is constructed — find it via `grep -rn 'oneShotRunner{' pkg/`), set `hideGit:` from the same source the daemon `runner` uses. Mirror exactly — both runners must receive the same resolved value for the same project config.

   c) In `oneShotRunner.Run` at `pkg/runner/oneshot.go:96`, add the gate call AFTER lock acquisition (after the `slog.Info("acquired lock", ...)` at line 107) and BEFORE `r.startupLogger()` at line 109:
   ```go
   if err := CheckGitSafety(ctx, r.hideGit); err != nil {
       return errors.Wrap(ctx, err, "git safety check failed")
   }
   ```

   The error must be returned BEFORE any container or processor work begins.

3. **Add an integration test** for the one-shot path in `pkg/runner/runner_test.go` (or `pkg/runner/oneshot_test.go` if that file exists — `grep -l 'oneShotRunner\|oneshot' pkg/runner/*_test.go`):

   Add a new `Describe("worktree gating for one-shot run", ...)` block that mirrors the existing daemon-path `Describe("worktree gating", ...)`. Tests:
   - `It("one-shot run refuses to start from worktree CWD when hideGit=false", ...)` — create a worktree-shaped temp dir (`.git` as regular file), construct a oneShotRunner with `hideGit=false`, call `Run()` with a short context timeout, expect non-nil error containing `worktree` and `hideGit`
   - `It("one-shot run starts successfully from worktree CWD when hideGit=true", ...)` — same setup with `hideGit=true`, expect `nil` error (or context-cancel as the runner proceeds)
   - `It("one-shot run refuses to start from submodule CWD when hideGit=false", ...)` — submodule-shaped temp dir, same gating expectation
   - `It("one-shot run starts successfully from regular repo CWD regardless of hideGit", ...)` — `.git` as directory, both hideGit values pass

   Reuse the test scaffolding from `pkg/runner/runner_test.go:1300-1394` (the daemon-path version) — same temp-dir setup, same mocked dependencies, just construct `oneShotRunner` instead of `runner`.

4. **Update existing tests that reference `(*runner).checkGitSafety`** if any. The unexported method is gone; tests that used it directly must call `CheckGitSafety(ctx, hideGit)` instead. Search via `grep -n 'checkGitSafety' pkg/runner/`.

5. **Verification:**
   - `go test ./pkg/runner/... -v` exits 0; new one-shot gating tests pass
   - `grep -n 'CheckGitSafety' pkg/runner/runner.go pkg/runner/oneshot.go` returns at least 3 lines (1 definition + 2 call sites — daemon + one-shot)
   - `grep -n 'checkGitSafety' pkg/runner/runner.go` returns 0 lines (the unexported method is gone)
   - Manual verification: `dark-factory run` from a `.git`-as-file CWD with `hideGit=false` MUST exit non-zero with stderr containing `worktree CWD detected`
   - `make precommit` exits 0

</requirements>

<constraints>
- Do NOT change `DetectWorktreeOrSubmodule`'s signature — the gate is a thin wrapper around it
- Do NOT introduce a new config field — the existing `hideGit` value already flows to both runners via the factory
- Do NOT skip the test step — AC6 specifically requires the `run` subcommand to be gated, and a missing test was how the gap shipped
- The daemon path's existing AC2/AC3 behavior must NOT regress — both runners share the same gate function now
- Do NOT commit — dark-factory handles git
- Use `errors.Wrapf(ctx, err, "message")` for error wrapping — never `fmt.Errorf`
</constraints>

<verification>
```bash
go test ./pkg/runner/... -v
grep -n 'CheckGitSafety' pkg/runner/runner.go pkg/runner/oneshot.go
grep -n 'checkGitSafety' pkg/runner/runner.go
make precommit
```
</verification>
