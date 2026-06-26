---
status: approved
spec: [100-centralize-subprocess-runner]
created: "2026-06-26T07:30:00Z"
queued: "2026-06-26T07:57:18Z"
branch: dark-factory/centralize-subprocess-runner
---

<summary>

- Routes every subprocess spawn in the git package and the Bitbucket git-provider through the single bounded subprocess runner, so all git/gh operations get warn-on-slow telemetry and a bounded timeout.
- Preserves the existing behavior where a long git error message is captured and truncated to 8 KiB with a `(truncated)` marker.
- Preserves exit-code propagation: callers that inspect the underlying process exit code keep working.
- Adds a new runner capability for the few commands that need a custom environment variable (the GitHub CLI token) without changing any existing runner method.
- Adds integration tests proving every git method spawns through the injected runner, that stderr truncation still works, and that exit codes still surface.
- After this prompt the git package contains zero raw subprocess-spawning calls.

</summary>

<objective>
Migrate ALL `exec.Command(Context)?` call sites in `pkg/git/*.go` (git.go, cloner.go, brancher.go, collaborator_fetcher.go, worktreer.go, root.go, pr_creator.go, pr_merger.go) and `pkg/gitprovider/bitbucket/remote.go` to spawn through `pkg/subproc.Runner`. Preserve the 8 KiB `truncateStderr` behavior and `*exec.ExitError` exit-code propagation end-to-end. Add one new (additive) Runner method for the gh-CLI sites that need a custom `GH_TOKEN` env. After this prompt, `grep -nE 'exec\.Command(Context)?\(' pkg/git/*.go` returns 0 matches.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-composition.md` — inject the runner; small interfaces.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 / Gomega, counterfeiter fakes, external `_test` packages, coverage ≥80%.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors`, `errors.As`, never `fmt.Errorf`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — public interface + private struct + `New*`, counterfeiter annotation.

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/100-centralize-subprocess-runner.md` — Desired Behavior 1, 4, 5; Acceptance Criteria 6, 7, 10; Constraints (truncateStderr, exit-code, interface frozen); Failure Modes rows 3, 4.

Read these source files before editing (ALL of them — full reads, not skims):
- `/workspace/pkg/subproc/subproc.go` — the runner. Frozen interface: `RunWithWarnAndTimeout(ctx, op, name, args...)` and `RunWithWarnAndTimeoutDir(ctx, op, dir, name, args...)`, both returning `([]byte, error)`. Internally uses `cmd.Output()` (stdout only). On child failure returns `errors.Wrapf(ctx, cmdErr, "%s failed", op)` where `cmdErr` is the `*exec.ExitError` from `cmd.Output()`. **Critical fact:** `cmd.Output()` populates `(*exec.ExitError).Stderr` with the child's stderr ONLY because the runner does not set `cmd.Stderr`. So stderr is recoverable from the wrapped `*exec.ExitError` via `errors.As`.
- `/workspace/pkg/git/stderr.go` — `TruncateStderr(s string) string` (exported, 8 KiB cap + ` (truncated)` suffix) and the unexported alias `truncateStderr`. `maxStderrBytes = 8192`. DO NOT change the cap or suffix.
- `/workspace/pkg/git/stderr_test.go` — existing truncation tests via `git.TruncateStderrForTest` (exported through export_test.go). Keep these passing.
- `/workspace/pkg/git/export_test.go` — how internals are exposed to the external `git_test` package.
- `/workspace/pkg/git/git.go` — every spawn site: `HasDirtyFiles` (54), `CommitCompletedFile` (244, 256, 270), `MoveFile` (287), `gitAddAll` (311), `stageAllAndCheck` (334), `getNextVersion` (352), `gitCommit` (517), `gitTag` (539), `gitPush` (556), `gitPushTag` (579). Note these are package-level functions with NO injected runner today.
- `/workspace/pkg/git/cloner.go` — `cloner` struct (currently empty). Spawn sites: `gitClone` (65), `setRealRemote` (85, 93), `checkoutBranch` (107, 114), `checkoutTrack` (132), `checkoutNew` (152). Some use `git -C <dir>`; some capture stderr; `fetchCmd`/`verifyCmd` ignore output and only check `.Run() == nil`.
- `/workspace/pkg/git/brancher.go` — `brancher` struct. ~18 spawn sites (lines 97-523). Mix of `.Run()` (output to a `combined` builder), `.Output()`, and one site (line 381) that sets `cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")`. The `DefaultBranch` site (233) is a `gh` call.
- `/workspace/pkg/git/collaborator_fetcher.go` — `ghRepoNameFetcher.Fetch` (94) and `ghCollaboratorLister.List` (128). Both `gh` calls; both set `cmd.Env = append(os.Environ(), "GH_TOKEN="+token)` when `ghToken != ""`.
- `/workspace/pkg/git/worktreer.go` — `worktreer` struct. Sites: `Add` (48, 60, 69), `Remove` (92). Captures stderr.
- `/workspace/pkg/git/root.go` — `ResolveGitRoot` (19), package-level function.
- `/workspace/pkg/git/pr_creator.go` — `prCreator` struct with an injected `commandOutputFn CommandOutputFn` (`func(*exec.Cmd) ([]byte, error)`) test seam. `FindOpenPR` (59) and `Create` (88) are `gh` calls that set `GH_TOKEN` env and run via `p.commandOutputFn(cmd)`.
- `/workspace/pkg/git/pr_merger.go` — `prMerger` struct. `checkPRStatus` (113) and `mergePR` (137) are `gh` calls that set `GH_TOKEN` env.
- `/workspace/pkg/gitprovider/bitbucket/remote.go` — `ParseRemoteFromGit` (70), `git remote get-url`, captures stderr, uses `git.TruncateStderr`.
- `/workspace/mocks/subproc-runner.go` — the `SubprocRunner` counterfeiter fake. After you add a new interface method (requirement 1), this regenerates in prompt 5; for THIS prompt's tests you may hand-use the existing fake methods and the new method's generated stub will appear after `make generate` (run `make generate` locally in this prompt — see requirement 7).
- `/workspace/pkg/factory/factory.go` — construction sites: `git.NewReleaser()` (209), `git.NewCloner()` (780), `git.NewWorktreer()` (781), `git.NewBrancher(...)` (246, 297), `git.NewPRCreator(ghToken)` (244), `git.NewPRMerger(...)` (245), `bitbucket.ParseRemoteFromGit` (262), `bitbucket.NewCollaboratorFetcher` (276). Constructors that gain a runner param (see requirement 2) must be updated at every site.

Decision already made (do NOT re-deliberate): stderr is recovered from the wrapped `*exec.ExitError.Stderr` via `errors.As`, NOT by adding stderr capture to the existing runner methods. The `truncateStderr` helper stays a `pkg/git` package-private helper (it is already there and exported as `TruncateStderr`); do NOT move it into `pkg/subproc`.
</context>

<requirements>

## 1. Add ONE additive Runner method for env-carrying commands (gh-CLI sites)

The gh-CLI sites set `GH_TOKEN` via `cmd.Env`. The frozen Runner methods take no env. Add a NEW method to the `Runner` interface (additive — does not rename/remove/change any existing method, which the spec permits).

1.1. In `/workspace/pkg/subproc/subproc.go`, extend the `Runner` interface with:
```go
// RunWithWarnAndTimeoutEnv is identical to RunWithWarnAndTimeoutDir but also
// appends extraEnv (each "KEY=VALUE") to the child's environment on top of
// os.Environ(). Pass dir="" to inherit the current working directory and
// extraEnv=nil for no extra env.
RunWithWarnAndTimeoutEnv(
    ctx context.Context,
    op string,
    dir string,
    extraEnv []string,
    name string,
    args ...string,
) ([]byte, error)
```

1.2. Implement it on `*runner` by extending the internal helper. Refactor `runInternal` to also accept `extraEnv []string` and set, inside the command goroutine:
```go
cmd := exec.CommandContext(ctx, name, args...)
if dir != "" {
    cmd.Dir = dir
}
if len(extraEnv) > 0 {
    cmd.Env = append(os.Environ(), extraEnv...)
}
output, cmdErr = cmd.Output()
```
Keep `RunWithWarnAndTimeout` and `RunWithWarnAndTimeoutDir` calling `runInternal` with `extraEnv=nil`. Add `"os"` to the imports. The `#nosec G204` comment on the `exec.CommandContext` line stays (trusted internal call sites). The single legitimate `exec.CommandContext` inside `pkg/subproc/` is exempt from the execcheck gate (gate skips `pkg/subproc/`).

1.3. Do NOT regenerate mocks yet by hand-editing — requirement 7 runs `make generate`. Tests in this prompt that need the new method's fake field (`RunWithWarnAndTimeoutEnvStub`) depend on that regeneration; run `make generate` BEFORE writing those tests.

1.4. Add a unit test in `pkg/subproc` (Ginkgo) for `RunWithWarnAndTimeoutEnv`: run `sh -c 'echo $FOO'` with `extraEnv=[]string{"FOO=bar"}` and assert output is `bar\n`. This is the boundary test (level 1) for the new method.

## 2. Introduce a package-private `subproc.Runner` into each `pkg/git` type and the package-level functions

The `pkg/git` package-level functions (git.go, root.go) have no struct to hold a runner. Decision (do NOT re-deliberate): add an unexported package-level default runner var and a small internal indirection so package-level functions and the struct types share one path.

2.1. **Avoid the test-only package-level mutable state anti-pattern.** Do NOT introduce `var defaultRunner = subproc.NewRunner()` + `SetDefaultRunnerForTest`. Instead, collect the 11 package-level free functions (`HasDirtyFiles`, `CommitCompletedFile`, `MoveFile`, `gitAddAll`, `stageAllAndCheck`, `getNextVersion`, `gitCommit`, `gitTag`, `gitPush`, `gitPushTag`, `ResolveGitRoot`) onto a struct that holds the runner:

```go
// /workspace/pkg/git/helpers.go
package git

import "github.com/bborbe/dark-factory/pkg/subproc"

// Helpers groups the git CLI free functions onto a runner-bearing struct so
// production wiring injects a runner once and tests inject a fake. Replaces
// the prior package-level free-function set; same behavior, testable.
type Helpers struct {
	runner subproc.Runner
}

// NewHelpers wires a Helpers with the default production runner.
func NewHelpers() *Helpers { return &Helpers{runner: subproc.NewRunner()} }

// NewHelpersWithRunner is the test seam.
func NewHelpersWithRunner(r subproc.Runner) *Helpers { return &Helpers{runner: r} }
```

Convert the 11 free functions into methods on `*Helpers` (`(*Helpers).HasDirtyFiles(ctx)`, etc.). Update the in-tree callers in `pkg/factory/factory.go`, `pkg/processor`, `pkg/committingrecoverer`, `pkg/promptresumer` and any other package importing these — pass a `*Helpers` instance constructed once at the factory level alongside the existing `releaser`, `brancher`, etc.

For tests, inject a fake via `NewHelpersWithRunner(fakeRunner)` — no package-level setter, no `export_test.go` hook. Follows the `go-composition.md` constructor-injection pattern used by every other struct in `pkg/git/`.

2.2. The struct types that already exist gain a `runner subproc.Runner` field, defaulted in their `New*` constructor via `subproc.NewRunner()`, with an additional unexported `New*WithRunner` (or option) used by tests:
   - `cloner`: add `runner` field; `NewCloner()` sets `runner: subproc.NewRunner()`. Add unexported `newClonerWithRunner(r subproc.Runner) *cloner`.
   - `brancher`: add `runner` field; `NewBrancher(opts...)` sets `b.runner = subproc.NewRunner()` after applying opts (so default is non-nil). Add a `BrancherOption` `withRunner(r subproc.Runner)` (unexported) OR an unexported `newBrancherWithRunner`. Brancher already uses the functional-options pattern — prefer an unexported option to stay consistent.
   - `worktreer`: add `runner` field; `NewWorktreer()` sets it. Add unexported `newWorktreerWithRunner`.
   - `prCreator`: it already has the `commandOutputFn` seam. REPLACE the `exec.Cmd`-based flow with the runner: add a `runner subproc.Runner` field, set in both `NewPRCreator` and `NewPRCreatorWithCommandOutput` (keep that constructor's signature for backward compat but have it also default the runner). The gh calls need `GH_TOKEN` env → use `RunWithWarnAndTimeoutEnv`. The `commandOutputFn` seam becomes unused for spawning; KEEP the exported `CommandOutputFn` type and `NewPRCreatorWithCommandOutput` constructor to avoid breaking callers, but the spawn now goes through the runner. If a test depended on `commandOutputFn`, migrate it to inject a fake runner instead.
   - `prMerger`: add `runner` field; set in `NewPRMerger`. Add unexported `newPRMergerWithRunner`. gh calls need `GH_TOKEN` env → `RunWithWarnAndTimeoutEnv`.
   - `collaboratorFetcher` / `ghRepoNameFetcher` / `ghCollaboratorLister`: add `runner` field to the two gh-backed types; set in `NewGHRepoNameFetcher` / `NewGHCollaboratorLister`. gh calls need `GH_TOKEN` env → `RunWithWarnAndTimeoutEnv`.

## 3. Rewrite every spawn site to use the runner, preserving stderr truncation and exit codes

For EACH `exec.Command(Context)?` call across the files listed in `<context>`:

3.1. Plain git commands that capture stdout (`HasDirtyFiles`, `getNextVersion`, `stageAllAndCheck`'s status, `ResolveGitRoot`, brancher `.Output()` sites, etc.): replace with `runner.RunWithWarnAndTimeout(ctx, "<op>", "git", <args...>)` (or `...Dir` when the original set `cmd.Dir`, or used `git -C <dir>`). Op label = the command string (e.g. `"git status --porcelain"`).

3.2. Git commands that used `cmd.Run()` and discarded combined output (commit/add/tag/push/mv): replace with `runner.RunWithWarnAndTimeout(ctx, "<op>", "git", <args...>)` and ignore the returned `[]byte`. The previous `slog.Debug("git output", ...)` lines may be dropped or kept reading the returned stdout bytes — keep the debug log reading the returned `[]byte` where the original logged it.

3.3. `git -C <dir> ...` sites in cloner.go/brancher.go: prefer `RunWithWarnAndTimeoutDir(ctx, op, dir, "git", <args-without -C>)`. Equivalent and cleaner. (Either form is acceptable; pick `...Dir` for consistency.)

3.4. The brancher line-381 site that sets `cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")`: use `RunWithWarnAndTimeoutEnv(ctx, op, dir, []string{"LC_ALL=C", "LANG=C"}, "git", <args...>)`.

3.5. gh-CLI sites (collaborator_fetcher.go, pr_creator.go, pr_merger.go, brancher.go `DefaultBranch`): use `RunWithWarnAndTimeoutEnv` with `extraEnv := []string{}` and, when `ghToken != ""`, `extraEnv = []string{"GH_TOKEN=" + ghToken}`. Preserve all argument validation that precedes the spawn (`repoNameRegexp`, `ValidatePRTitle`, `ValidatePRURL`) verbatim — those are the input-validation trust boundary (spec Security).

3.6. **Stderr truncation preservation (spec AC 7, Constraint).** The previous code captured stderr into a `strings.Builder` and passed `truncateStderr(builder.String())` into the error message. The runner does NOT expose stderr directly; on failure it returns a wrapped `*exec.ExitError` whose `.Stderr` field holds the child stderr. Add a package-private helper in `pkg/git`:
```go
// stderrFromErr extracts and truncates the child stderr captured in a wrapped
// *exec.ExitError (populated by cmd.Output() inside subproc.Runner). Returns ""
// when err carries no *exec.ExitError.
func stderrFromErr(err error) string {
    var exitErr *exec.ExitError
    if errors.As(err, &exitErr) {
        return TruncateStderr(string(exitErr.Stderr))
    }
    return ""
}
```
(Import `"os/exec"` for the `*exec.ExitError` TYPE only — referencing the type is NOT a spawn and the execcheck gate only matches `exec.Command(Context)?\(`, so this import does not trip the gate. Confirm `grep -nE 'exec\.Command(Context)?\(' pkg/git/*.go` is still 0 after this.) Use `stderrFromErr(err)` in the error-wrapping of each migrated site that previously embedded `truncateStderr(...)`, e.g.:
```go
output, err := runner.RunWithWarnAndTimeout(ctx, "git status --porcelain", "git", "status", "--porcelain")
if err != nil {
    return false, errors.Wrapf(ctx, err, "git status: %s", stderrFromErr(err))
}
```

3.7. **Exit-code preservation (spec AC, Constraint).** Because the runner wraps the original `*exec.ExitError` and `bborbe/errors.As` walks the `Cause()` chain, a caller doing `errors.As(returnedErr, &exitErr)` and reading `exitErr.ExitCode()` continues to work. Do NOT replace the underlying error — only wrap it. Verify with the AC-9-style test (requirement 6.3).

3.8. After migration, remove the now-unused `"os/exec"` import from any file that no longer references `*exec.Cmd` or `*exec.ExitError`. Files keeping `*exec.ExitError` (those using `stderrFromErr` defined once in one file) keep the import only where the type is referenced — `stderrFromErr` should live in ONE file (e.g. `runner.go` or `stderr.go`) so only that file imports `os/exec`. Remove `CommandOutputFn`/`defaultCommandOutput` `exec.Cmd` usage from pr_creator.go if the seam is fully replaced (but keep the exported type alias if any caller imports it — grep `CommandOutputFn` across the repo first; if only pr_creator.go and its test use it, you may remove it and update the test).

## 4. Migrate `pkg/gitprovider/bitbucket/remote.go`

4.1. `ParseRemoteFromGit` takes `(ctx, remoteName)` and currently spawns `git remote get-url <remoteName>` directly. Inject a runner: change the function to accept a `subproc.Runner` OR (simpler, matching the package-level pattern) add a package-private `defaultRunner = subproc.NewRunner()` in the bitbucket package and use it. Decision: add a `runner` parameter is cleaner for testability, BUT `ParseRemoteFromGit` is called from factory.go (line 262) — adding a param means updating that caller. Choose the param approach:
```go
func ParseRemoteFromGit(ctx context.Context, runner subproc.Runner, remoteName string) (*RemoteCoords, error)
```
Use `runner.RunWithWarnAndTimeout(ctx, "git remote get-url", "git", "remote", "get-url", remoteName)` and on error wrap with `git.TruncateStderr` applied to stderr extracted from the error. Since `stderrFromErr` is package-private in `pkg/git`, replicate a tiny local helper in the bitbucket package OR (cleaner) export `git.StderrFromError(err error) string` from `pkg/git` and use it here. Decision: export `git.StderrFromError(err error) string` (a thin exported wrapper over the private `stderrFromErr`) so both packages share it; use it in remote.go.
4.2. Update the factory.go call at line 262 to pass `subproc.NewRunner()`.
4.3. Update `pkg/gitprovider/bitbucket/remote_test.go` for the new signature (inject a `mocks.SubprocRunner` fake).

## 5. Update all factory.go construction sites

Update every constructor call whose signature changed: `bitbucket.ParseRemoteFromGit` (line 262, add `subproc.NewRunner()`). The struct constructors `git.NewReleaser`, `git.NewCloner`, `git.NewWorktreer`, `git.NewBrancher`, `git.NewPRCreator`, `git.NewPRMerger`, `git.NewGHRepoNameFetcher`, `git.NewGHCollaboratorLister` keep their PUBLIC signatures (they default the runner internally per requirement 2.2) — so factory.go does NOT change for those. Confirm by grepping that no public `New*` signature in pkg/git changed. Add `"github.com/bborbe/dark-factory/pkg/subproc"` import to factory.go if missing.

## 6. Tests (spec AC 6, 7, 10)

6.1. **RunnerInjected (AC 6).** In `pkg/git` external tests, add a spec whose `It` description contains `RunnerInjected`. For each public method that spawns (pick representative coverage: `cloner.Clone`, `brancher.CreateAndSwitch`, `worktreer.Add`, `releaser.CommitOnly` path, a gh method), construct the type with an injected `mocks.SubprocRunner` fake, invoke the method, and assert the fake's call count for `RunWithWarnAndTimeout*` is `>= 1` (the AC says "exactly once per spawn" — assert the total spawn-method call count equals the number of git commands that method issues on the success path). Configure the fake's `*Returns`/`*Stub` to return benign output so the method reaches its spawn points. `go test -v ./pkg/git/... -run RunnerInjected` must contain `PASS`.

6.2. **TruncateStderr (AC 7).** Add a spec whose `It` description contains `TruncateStderr`. Drive a migrated code path with an injected fake runner whose stub returns an error wrapping an `*exec.ExitError` carrying a 16 KiB `Stderr`. Construct such an error in the test: since you cannot easily fabricate a real `*exec.ExitError`, instead run a REAL failing git command through the real runner that produces >8 KiB stderr is impractical; therefore test `stderrFromErr` directly AND via a fake: have the fake return `someWrappedExitErr` where `someWrappedExitErr` is produced by running `exec.CommandContext(ctx,"git","-c","invalid.option=x","status").Output()` in the TEST (test files are exempt from the gate) and capturing the returned error — but that stderr is short. For the 16 KiB requirement, fabricate via a helper that runs `sh -c 'head -c 16384 /dev/zero | tr "\\0" "X" >&2; exit 1'` through `cmd.Output()` in the test to get a real `*exec.ExitError` with 16 KiB `Stderr`, then pass that error to `git.StderrFromError` and assert the result has suffix ` (truncated)` and `len <= 8192 + len(" (truncated)")`. `go test -v ./pkg/git/... -run TruncateStderr` must contain `PASS`. (The existing `stderr_test.go` `TruncateStderrForTest` cases stay.)

6.3. **ExitCodePropagated (AC, exit-code).** Add a spec whose `It` description contains `ExitCodePropagated`. Run `git -c invalid.option=x status` through a MIGRATED public path (or through `defaultRunner` directly via a package-level function in a real temp git dir) and assert the returned error satisfies `errors.As(err, &exitErr)` for `var exitErr *exec.ExitError` and that `exitErr.ExitCode()` equals the value captured as a constant in the test (run the same command pre-migration once to learn the code — typically `129` for `git`'s bad-usage or `128`; capture whatever the real git emits and assert equality to that captured constant). Use `github.com/bborbe/errors` `As`. `go test -v ./pkg/git/... -run ExitCodePropagated` must contain `PASS`.

6.4. Update ALL existing `pkg/git` tests (git_test.go, cloner_test.go, brancher_test.go, collaborator_fetcher_test.go, pr_creator_test.go, pr_merger_test.go, worktreer_test.go, root_test.go, git_internal_test.go) that break due to the runner injection. Where a test exercised a real git repo end-to-end, it can keep using the default runner (real). Where it used `commandOutputFn`, migrate to a fake runner.

## 7. Regenerate mocks

Run `make generate` so `mocks/subproc-runner.go` gains `RunWithWarnAndTimeoutEnv` fake methods. This is required before the tests referencing `RunWithWarnAndTimeoutEnvStub`/`...Returns` will compile. After `make generate`, `git status --porcelain mocks/` should show only the expected `subproc-runner.go` change (spec AC 8 is fully verified in prompt 5, but keep the tree consistent here).

## 8. CHANGELOG

Append to `## Unreleased` in `/workspace/CHANGELOG.md`:
```
- fix: route all pkg/git and pkg/gitprovider/bitbucket subprocess spawns through pkg/subproc.Runner (warn-on-slow + bounded timeout); 8 KiB stderr truncation and *exec.ExitError exit-code propagation preserved; adds additive RunWithWarnAndTimeoutEnv for GH_TOKEN-carrying gh calls (spec 100 prompt 3)
```

</requirements>

<constraints>

- Do NOT rename, remove, or change the signature of any EXISTING `subproc.Runner` method — only ADD `RunWithWarnAndTimeoutEnv` (spec Constraint — interface frozen, new methods allowed).
- Keep the PUBLIC signatures of `git.NewReleaser`, `git.NewCloner`, `git.NewBrancher`, `git.NewWorktreer`, `git.NewPRCreator`, `git.NewPRMerger`, `git.NewGHRepoNameFetcher`, `git.NewGHCollaboratorLister` unchanged — default the runner internally. `bitbucket.ParseRemoteFromGit` DOES gain a `subproc.Runner` param (update its one factory caller).
- `truncateStderr` semantics — 8 KiB cap (`maxStderrBytes = 8192`) with literal ` (truncated)` suffix — preserved end-to-end (spec Constraint, AC 7). Do NOT change the cap or suffix. Keep the helper in `pkg/git` (do NOT move to `pkg/subproc`).
- Exit-code semantics preserved: callers using `errors.As` to unwrap `*exec.ExitError` and read `ExitCode()` keep working. Wrap, never replace, the underlying error (spec Constraint).
- Preserve ALL input validation that precedes a spawn (`repoNameRegexp`, `ValidatePRTitle`, `ValidatePRURL`, semver tag validation) verbatim — these are trust boundaries (spec Security).
- After this prompt: `grep -nE 'exec\.Command(Context)?\(' pkg/git/*.go` returns 0 matches and `grep -nE 'exec\.Command(Context)?\(' pkg/gitprovider/bitbucket/*.go` returns 0 matches (AC 6, Desired Behavior 5). Referencing the `*exec.ExitError` TYPE or `*exec.Cmd` type is fine — only the `exec.Command(`/`exec.CommandContext(` CALL is forbidden.
- Wrap errors with `github.com/bborbe/errors`; never `fmt.Errorf`; never `context.Background()` in `pkg/` non-test code (spec / go-error-wrapping-guide).
- Coverage ≥80% on changed `pkg/git` and `pkg/gitprovider/bitbucket` code; error paths (stderr truncation, exit-code, runner failure) tested (spec AC 6, 7).
- Do NOT touch `pkg/executor` — that is prompt 4.
- Do NOT flip the execcheck gate to strict — that is prompt 5 (gate is in warn mode from prompt 1). But after this prompt the `pkg/git` and `pkg/gitprovider` offenders are gone, which prompt 5 relies on.
- No new Go dependencies (spec Constraint).
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# Regenerate mocks for the new interface method
make generate

# AC 6 / Desired Behavior 5 — zero raw spawns remain in git + gitprovider
grep -nE 'exec\.Command(Context)?\(' pkg/git/*.go            # expected: 0 matches
grep -nE 'exec\.Command(Context)?\(' pkg/gitprovider/bitbucket/*.go   # expected: 0 matches

# Compile + unit tests
go test -mod=mod ./pkg/git/... ./pkg/gitprovider/... ./pkg/subproc/... ./pkg/factory/...

# AC 6 — runner injection
go test -v -mod=mod ./pkg/git/... -run RunnerInjected        # output contains PASS

# AC 7 — stderr truncation preserved
go test -v -mod=mod ./pkg/git/... -run TruncateStderr        # output contains PASS

# exit-code preserved
go test -v -mod=mod ./pkg/git/... -run ExitCodePropagated    # output contains PASS

# new env method boundary test
go test -v -mod=mod ./pkg/subproc/... -run Env               # output contains PASS

# Coverage
go test -coverprofile=/tmp/cover.out -mod=mod ./pkg/git/... ./pkg/gitprovider/... && go tool cover -func=/tmp/cover.out | tail -1

# execcheck still warn (must stay green); git offenders now gone from its output
bash scripts/hotpath-execcheck.sh warn 2>/tmp/exec_off.txt; echo "exit=$?"
grep -E '^pkg/git/|^pkg/gitprovider/' /tmp/exec_off.txt && echo "BUG: git site still flagged" || echo "git fully migrated"

# Full precommit (execcheck warn — green)
make precommit                                               # expected: exit 0
```

</verification>
