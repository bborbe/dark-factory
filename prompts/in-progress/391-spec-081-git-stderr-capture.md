---
status: approved
spec: [081-bug-git-wrapper-swallows-stderr]
created: "2026-05-16T12:10:00Z"
queued: "2026-05-16T13:03:39Z"
branch: dark-factory/bug-git-wrapper-swallows-stderr
---

<summary>
- Every git shell-out in `pkg/git/` that previously swallowed stderr now captures it into a buffer
- When a git wrapper fails with non-zero exit, the returned error string contains git's captured stderr verbatim, appended after the existing wrapper label
- Operators can read the exact git error message ("Your local changes would be overwritten by merge") directly from `.dark-factory.log` without SSHing into the worktree
- A truncation guard caps captured stderr at 8 KiB to bound log-line size; oversized output gets a `(truncated)` marker
- Successful git commands log their captured combined output at DEBUG level so `--log-level=debug` shows what git said
- `dark-factory prompt show <id>` now displays the `lastFailReason` field so the operator sees the full error without reading the raw log file
- The prompt-show text renderer is extracted to a testable pure function accepting an `io.Writer`
- A new `docs/troubleshooting.md` file contains a "Reading prompt-failure errors" section with a before/after dirty-tree example
- Six new Ginkgo tests lock down the contract: stderr-in-error, dirty-tree verbatim text, renderer output, DEBUG-log-on-success, truncation, and no regression
</summary>

<objective>
Capture stderr from every `pkg/git/` shell-out and include it verbatim in the error returned on failure, so operators can diagnose git failures (dirty tree, auth, network, index lock, conflicts) directly from `.dark-factory.log` and `dark-factory prompt show` without manual worktree reproduction. This is a reporting bug — daemon control flow is unchanged.
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors wrapping, Ginkgo/Gomega, Counterfeiter).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-logging-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` for which test types to write for each code change.

Files to read in full before editing:
- `pkg/git/brancher.go` — all methods; identify which use `cmd.Run()` vs `cmd.Output()` and which already capture stderr
- `pkg/git/git.go` — helper functions `gitCommit`, `gitTag`, `gitPush`, `gitPushTag`, `gitAddAll`, `CommitCompletedFile`, `MoveFile`
- `pkg/git/cloner.go` — already has stderr capture; read to understand the established pattern
- `pkg/git/worktreer.go` — already has stderr capture; same
- `pkg/git/brancher_test.go` — understand existing BeforeEach (real git repo in temp dir, os.Chdir); tests go in the same file
- `pkg/git/git_suite_test.go` — suite bootstrap
- `pkg/cmd/prompt_show.go` — `PromptShowOutput`, `Run` method to be refactored
- `pkg/cmd/prompt_show_test.go` — existing tests; understand what is already tested
- `pkg/prompt/prompt.go` — `Frontmatter.LastFailReason` field and `SetLastFailReason` method
</context>

<requirements>

## 1. Add `truncateStderr` helper to `pkg/git/`

Create a new file `pkg/git/stderr.go` with:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import "strings"

// maxStderrBytes is the maximum number of bytes captured from git stderr before
// truncation. 8 KiB is enough to surface the actionable first lines of any git
// error (conflict lists, auth failures, refname warnings) while keeping log
// lines bounded. Operators can always re-run the failed command manually if
// they need the full output.
const maxStderrBytes = 8192

// truncateStderr returns s unchanged if len(s) <= maxStderrBytes.
// If s exceeds the limit, the first maxStderrBytes bytes are returned followed
// by the literal string " (truncated)".
func truncateStderr(s string) string {
    if len(s) <= maxStderrBytes {
        return strings.TrimRight(s, "\n")
    }
    return strings.TrimRight(s[:maxStderrBytes], "\n") + " (truncated)"
}
```

This helper is used in every wrapper below to format the stderr string before including it in the error message.

## 2. Update `pkg/git/brancher.go` — capture stderr at every `cmd.Run()` shell-out

**Audit step (required before editing):** Run `grep -n 'exec\.Command' pkg/git/brancher.go` to list every call site. Confirm each one is covered below. Do NOT edit `FetchBranch` or `MergeToDefault` (verified to already capture stderr via `var stderr strings.Builder` + `cmd.Stderr = &stderr`). `DiscardUncommittedInPaths` (around line 307) still uses bare `cmd.Run()` — apply the combined-output pattern to it as well (label: `"discard uncommitted changes in paths: %s"`).

For each method below, apply the pattern:
1. Declare `var combined strings.Builder` before the command.
2. For `cmd.Run()` cases (no important stdout): assign **both** `cmd.Stdout = &combined` and `cmd.Stderr = &combined` (combined output — simplest for commands that don't need stdout returned).
3. Replace `cmd.Run()` with `combined.Reset(); err := cmd.Run()` — wait, `cmd.Run()` is still fine; the pipe is set up before Run.
4. On error: wrap with `errors.Wrapf(ctx, err, "%s: %s", existingLabel, truncateStderr(combined.String()))`.
5. On success: if `combined.String() != ""`, log `slog.Debug("git output", "op", "<method-name>", "output", combined.String())`.

**Note on the error format:** the existing error label (e.g., `"create and switch to branch"`) must remain as the primary text to preserve log greppability. Stderr is appended after `: `. Example result: `"create and switch to branch: error: pathspec 'feat/x' does not match any file(s) known to git"`.

### 2a. `CreateAndSwitch` — `git checkout -b <name>`

Current:
```go
cmd := exec.CommandContext(ctx, "git", "checkout", "-b", name)
if err := cmd.Run(); err != nil {
    return errors.Wrap(ctx, err, "create and switch to branch")
}
```

Replace with:
```go
var combined strings.Builder
cmd := exec.CommandContext(ctx, "git", "checkout", "-b", name)
cmd.Stdout = &combined
cmd.Stderr = &combined
if err := cmd.Run(); err != nil {
    return errors.Wrapf(ctx, err, "create and switch to branch: %s", truncateStderr(combined.String()))
}
if s := combined.String(); s != "" {
    slog.Debug("git output", "op", "create-and-switch", "output", s)
}
```

### 2b. `Push` — `git push -u origin <name>`

Same pattern. Label: `"push branch to remote: %s"`.

### 2c. `Switch` — `git checkout <name>`

Same pattern. Label: `"switch to branch: %s"`.

### 2d. `Fetch` — `git fetch origin`

Same pattern. Label: `"fetch from origin: %s"`.

### 2e. `FetchAndVerifyBranch` — two commands

The first command is `git fetch origin` (currently `cmd.Run()`): apply the same pattern, label `"fetch from origin: %s"`.

The second command is `git rev-parse --verify origin/<branch>` (currently checks presence): this command intentionally returns a custom "branch not found" error rather than wrapping the git error. Keep this behavior. No stderr change needed for the second command (rev-parse --verify exits silently when the ref is missing; stderr is empty).

### 2f. `Pull` — `git pull`

Same pattern. Label: `"pull current branch: %s"`.

### 2g. `MergeOriginDefault` — `git merge origin/<defaultBranch>`

**This is the triggering incident.** Apply the pattern. Label: `"merge origin/%s: %s"` (include the branch name). Example:
```go
var combined strings.Builder
cmd := exec.CommandContext(ctx, "git", "merge", "origin/"+defaultBranch)
cmd.Stdout = &combined
cmd.Stderr = &combined
if err := cmd.Run(); err != nil {
    return errors.Wrapf(ctx, err, "merge origin/%s: %s", defaultBranch, truncateStderr(combined.String()))
}
if s := combined.String(); s != "" {
    slog.Debug("git output", "op", "merge-origin-default", "output", s)
}
```

## 3. Update `pkg/git/git.go` — capture stderr at every `cmd.Run()` shell-out

**Audit step:** Run `grep -n 'exec\.Command' pkg/git/git.go` before editing.

### 3a. `gitAddAll` — `git add -A`

Apply the combined-output pattern. Label: `"git add all: %s"`.

### 3b. `gitCommit` — `git commit -m <msg>`

Apply the combined-output pattern. Label: `"create commit: %s"`.

### 3c. `gitTag` — `git tag <tag>`

Apply the combined-output pattern. Label: `"create tag: %s"`.

### 3d. `gitPush` — `git push`

Apply the combined-output pattern. Label: `"push to remote: %s"`.

### 3e. `gitPushTag` — `git push origin <tag>`

Apply the combined-output pattern. Label: `"push tag to remote: %s"`.

### 3f. `CommitCompletedFile` — two commands

The `git add <path>` call: apply the combined-output pattern, label `"git add: %s"`.
The `git commit -m <msg>` call: apply the combined-output pattern, label `"git commit: %s"`.

### 3g. `MoveFile` — `git mv <old> <new>`

This command falls back to `os.Rename` on error (the error is intentionally swallowed). Instead of changing the error message, log the stderr at DEBUG before falling back:

```go
var combined strings.Builder
cmd := exec.CommandContext(ctx, "git", "mv", oldPath, newPath)
cmd.Stdout = &combined
cmd.Stderr = &combined
if err := cmd.Run(); err != nil {
    if s := combined.String(); s != "" {
        slog.Debug("git mv failed, falling back to os.Rename", "stderr", s)
    }
    return fallbackRename(ctx, oldPath, newPath)
}
```

## 4. Verify no `cmd.Run()` git call sites were missed; handle `.Output()` sites separately

After editing, run:
```
grep -rnE 'exec\.Command(Context)?\([^,]+,\s*"git"' pkg/git/
```

For **every** file and line returned, classify the call site:

**(a) `.Run()` sites** — MUST capture stderr via `cmd.Stdout = &combined; cmd.Stderr = &combined`. These are the focus of requirements 2-3. If any `.Run()` site is unmatched, apply the same combined-output pattern.

**(b) `.Output()` sites** (audited: ~11 — `brancher.go:136, 196, 216, 247, 256, 414`; `git.go:54, 231, 289, 305`) — these read stdout and intentionally discard stderr because they parse output of read-only commands (`status --porcelain`, `rev-parse`, `tag --list`, `symbolic-ref`, `rev-list --count`). For each, set `cmd.Stderr = &stderrBuf` (single-purpose buffer, NOT combined with stdout) and on `.Output()` error wrap with `errors.Wrapf(ctx, err, "<label>: %s", truncateStderr(stderrBuf.String()))`. Example pattern:

```go
var stderrBuf strings.Builder
cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
cmd.Stderr = &stderrBuf
output, err := cmd.Output()
if err != nil {
    return errors.Wrapf(ctx, err, "check working tree status: %s", truncateStderr(stderrBuf.String()))
}
```

Apply this pattern to every `.Output()` site. The label must describe the operation (not "git output"). Keep the existing return type and stdout-parsing logic unchanged.

**(c) Already-compliant sites** — `cloner.go`, `worktreer.go`, `brancher.go::FetchBranch`, `brancher.go::MergeToDefault`. Do not modify.

If after these changes any unmatched call sites remain, apply the relevant pattern before continuing.

## 5. Update `pkg/cmd/prompt_show.go` — display `LastFailReason` and extract renderer

### 5a. Add `LastFailReason` to `PromptShowOutput`

```go
type PromptShowOutput struct {
    File           string   `json:"file"`
    Status         string   `json:"status"`
    Specs          []string `json:"specs,omitempty"`
    Summary        string   `json:"summary,omitempty"`
    LastFailReason string   `json:"lastFailReason,omitempty"`
    Created        string   `json:"created,omitempty"`
    Queued         string   `json:"queued,omitempty"`
    Started        string   `json:"started,omitempty"`
    Completed      string   `json:"completed,omitempty"`
    LogPath        string   `json:"log_path,omitempty"`
}
```

### 5b. Populate `LastFailReason` from frontmatter

In the `Run` method, after building `out`, add:
```go
out.LastFailReason = pf.Frontmatter.LastFailReason
```

### 5c. Extract `RenderPromptShow` as a pure, exported function

Extract the text-mode rendering (the block of `fmt.Printf` calls) into:

```go
// RenderPromptShow writes the human-readable text representation of out to w.
func RenderPromptShow(w io.Writer, out PromptShowOutput) {
    fmt.Fprintf(w, "File:    %s\n", out.File)
    fmt.Fprintf(w, "Status:  %s\n", out.Status)
    if len(out.Specs) > 0 {
        fmt.Fprintf(w, "Spec:    %s\n", strings.Join(out.Specs, ", "))
    }
    if out.Summary != "" {
        fmt.Fprintf(w, "Summary: %s\n", out.Summary)
    }
    if out.LastFailReason != "" {
        fmt.Fprintf(w, "Error:   %s\n", out.LastFailReason)
    }
    if out.Created != "" {
        fmt.Fprintf(w, "Created:   %s\n", out.Created)
    }
    if out.Queued != "" {
        fmt.Fprintf(w, "Queued:    %s\n", out.Queued)
    }
    if out.Started != "" {
        fmt.Fprintf(w, "Started:   %s\n", out.Started)
    }
    if out.Completed != "" {
        fmt.Fprintf(w, "Completed: %s\n", out.Completed)
    }
    if out.LogPath != "" {
        fmt.Fprintf(w, "Log:     %s\n", out.LogPath)
    }
}
```

Update the `Run` method to call `RenderPromptShow(os.Stdout, out)` instead of the inline `fmt.Printf` calls. The `io` package import is already in the standard library — add it to the import block if not present.

## 6. Add tests to `pkg/git/brancher_test.go` (or a new file `pkg/git/stderr_test.go`)

Add a new `Describe("stderr capture")` block. These tests use a **fake git binary** injected via PATH override. They do NOT require an actual git repository, so they can run independently of the BeforeEach that calls `os.Chdir`. Place them inside the outer `Describe("Brancher", ...)` or in a separate top-level `Describe` — whichever keeps the existing BeforeEach/AfterEach from interfering.

**Preferred structure:** use a nested `Describe` inside the existing outer Describe but with its own `BeforeEach` that sets up the fake binary without requiring a real git repo. Since these tests intercept the "git" binary, they must NOT run in the temp git repo dir — use a separate temp dir or no dir change.

### Helper: create a fake git binary

Write a helper function in the test file (unexported, only used in tests):

```go
// makeFakeGitBinary writes a shell script "git" to dir and returns a cleanup func.
// When called, the script: prints stderrOutput to stderr, prints stdoutOutput to stdout,
// then exits with exitCode.
func makeFakeGitBinary(t GinkgoTInterface, dir string, exitCode int, stderrOutput, stdoutOutput string) {
    script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' %q >&2\nprintf '%%s' %q\nexit %d\n",
        stderrOutput, stdoutOutput, exitCode)
    gitPath := filepath.Join(dir, "git")
    Expect(os.WriteFile(gitPath, []byte(script), 0700)).To(Succeed())
}
```

In the tests, use `GinkgoT().TempDir()` for the fake binary dir, and store/restore PATH with `DeferCleanup`.

### Test 6a: Error contains git stderr (generic)

Setup:
- Fake git: exits 2, stderr = `"INJECTED_STDERR_MARKER\nsecond line"`, stdout = `""`
- `b := git.NewBrancher(git.WithDefaultBranch("master"))`
- PATH = `fakeBinDir:$PATH` (DeferCleanup restores)

Action: `err := b.MergeOriginDefault(ctx)` (using `WithDefaultBranch` bypasses the `gh` call)

Assert:
- `err` is not nil
- `err.Error()` contains `"INJECTED_STDERR_MARKER"`
- `err.Error()` contains `"second line"`

### Test 6b: Dirty-tree triggering-incident case

Setup:
- Fake git: exits 2, stderr = `"error: Your local changes to the following files would be overwritten by merge:\n\tprompts/spec-031.md\nPlease commit your changes or stash them before you merge.\nAborting"`, stdout = `""`
- Same PATH setup

Action: `err := b.MergeOriginDefault(ctx)`

Assert:
- `err.Error()` contains `"Your local changes to the following files would be overwritten by merge"`
- `err.Error()` contains `"Please commit your changes or stash them before you merge"`
- `err.Error()` contains `"Aborting"`

### Test 6c: Successful command logs at DEBUG

Setup:
- Fake git: exits 0, stdout = `"Already up to date.\n"`, stderr = `""`
- Set up a `bytes.Buffer` slog handler at DEBUG level; set as the default slog logger; restore in DeferCleanup:
  ```go
  var logBuf bytes.Buffer
  testHandler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
  testLogger := slog.New(testHandler)
  oldDefault := slog.Default()
  slog.SetDefault(testLogger)
  DeferCleanup(func() { slog.SetDefault(oldDefault) })
  ```

Action: `err := b.Pull(ctx)` (Pull uses `git pull` which would be intercepted by the fake binary)

Assert:
- `err` is nil
- `logBuf.String()` contains `"Already up to date."`

### Test 6d: Truncation guard

Setup:
- Generate a 64 KiB stderr string: `bigStderr := strings.Repeat("X", 64*1024)`
- Fake git: exits 2, stderr = `bigStderr`, stdout = `""`

Action: `err := b.MergeOriginDefault(ctx)`

Assert:
- `err` is not nil
- `err.Error()` contains `"(truncated)"`
- `len(err.Error()) < 64*1024` (error is bounded)

## 7. Add test for `truncateStderr` in `pkg/git/stderr_test.go`

Create `pkg/git/stderr_test.go` in `package git_test`. Write a Ginkgo `Describe("truncateStderr")` block with cases:
- Input shorter than 8192 bytes → returned unchanged (modulo trailing newline stripping)
- Input exactly 8192 bytes → returned unchanged
- Input 8193 bytes → result ends with `" (truncated)"` and length is bounded

Use the exported wrapper `git.TruncateStderrForTest(s string) string` — add this to `pkg/git/export_test.go`:

```go
// TruncateStderrForTest exposes truncateStderr for external tests.
func TruncateStderrForTest(s string) string {
    return truncateStderr(s)
}
```

## 8. Add test for `RenderPromptShow` in `pkg/cmd/prompt_show_test.go`

Add a new `Describe("RenderPromptShow")` block (can be in the existing file or a new `prompt_show_render_test.go` in `package cmd_test`):

```go
Describe("RenderPromptShow", func() {
    It("renders LastFailReason verbatim including multi-line git stderr", func() {
        var buf bytes.Buffer
        out := cmd.PromptShowOutput{
            File:   "042-foo.md",
            Status: "failed",
            LastFailReason: "merge origin/master: exit status 2\nYour local changes to the following files would be overwritten by merge:\n\tprompts/spec-031.md\nPlease commit your changes or stash them before you merge.\nAborting: INJECTED_STDERR_MARKER\nsecond line",
        }
        cmd.RenderPromptShow(&buf, out)
        output := buf.String()
        Expect(output).To(ContainSubstring("INJECTED_STDERR_MARKER"))
        Expect(output).To(ContainSubstring("second line"))
        Expect(output).To(ContainSubstring("would be overwritten by merge"))
    })
})
```

## 9. Write `docs/troubleshooting.md`

Create the file. It need not be exhaustive — the spec requires exactly one section:

```markdown
# Troubleshooting

## Reading prompt-failure errors

When a prompt fails, dark-factory records the error in the prompt file's `lastFailReason`
field and in `.dark-factory.log`.

### Before this fix

The daemon log showed only the exit code and a Go stack trace:

```
time=2026-05-16T13:06:19Z level=ERROR msg="prompt failed" file=120-fix.md error="exit status 2
merge origin/master
github.com/bborbe/errors.Wrap
   ...pkg/git/brancher.go:343
..."
```

The actual reason (`Your local changes would be overwritten by merge`) was absent. The operator
had to SSH into the worktree and re-run `git merge origin/master` manually.

### After this fix

The daemon log contains git's stderr verbatim:

```
time=2026-05-16T13:06:19Z level=ERROR msg="prompt failed" file=120-fix.md error="merge origin/master: exit status 2: error: Your local changes to the following files would be overwritten by merge:\n\tprompts/spec-031.md\nPlease commit your changes or stash them before you merge.\nAborting"
```

The `dark-factory prompt show <id>` output also shows the full error under the `Error:` field.

**Resolution for dirty-tree failures:** commit or stash the listed files in the project
worktree, then run `dark-factory prompt retry` to re-queue the failed prompt.
```


## 10. Add a CHANGELOG entry

Add under `## Unreleased` in `CHANGELOG.md`:

```
- fix: git wrappers in pkg/git/ now capture stderr and include it verbatim in errors, so dirty-tree, auth, and network failures are diagnosable from the daemon log without manual worktree reproduction
```

## 11. Run `make test` iteratively

After each group of changes (e.g., after finishing `brancher.go`, after finishing `git.go`, after finishing `prompt_show.go`), run `make test` to catch compile errors early. Do not wait until all files are done before first test run.

</requirements>

<constraints>
- Public signatures of `pkg/git/` wrapper functions (`MergeOriginDefault`, `Checkout`, `Rebase`, `Pull`, `Fetch`, `Push`, and peers) MUST NOT change. Callers continue to receive `error`.
- Error-message format change is additive only: the existing prefix text (e.g. `merge origin/master: exit status 2`) is preserved; git stderr appears appended. Operators grepping logs for `"merge origin/master"` still match.
- No new external dependencies. Use stdlib `bytes` / `strings` for capture.
- Do NOT modify `cloner.go` or `worktreer.go` — they already capture stderr correctly.
- Existing unit tests in `pkg/git/*_test.go` must continue to pass without modification.
- Wrap all errors with `errors.Wrapf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- Do NOT commit — dark-factory handles git.
- Do not touch `go.mod` / `go.sum` / `vendor/`.
- The truncation limit (8192 bytes) must be documented in a comment at its definition site in `pkg/git/stderr.go` (requirement 1 includes this comment).
- The fake git binary test approach uses PATH override, not mocking of `exec.Command` — do not add an injectable `exec.Command` seam to production code.
- `truncateStderr` must also strip trailing newlines from the stderr string (use `strings.TrimRight(s, "\n")`) so error messages don't end with a bare newline character.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -rnE 'exec\.Command(Context)?\([^,]+,\s*"git"' pkg/git/` — for every hit, confirm stderr capture is present in the surrounding lines (either combined Stdout+Stderr or explicit Stderr assignment), EXCEPT in `cloner.go` and `worktreer.go` which are already compliant.
2. `grep -n 'LastFailReason\|Error:' pkg/cmd/prompt_show.go` — shows the new field in `PromptShowOutput` and the `fmt.Fprintf(w, "Error:` line in `RenderPromptShow`.
3. `grep -n 'truncateStderr\|maxStderrBytes' pkg/git/stderr.go` — both are present.
4. `grep -n 'TruncateStderrForTest' pkg/git/export_test.go` — exported wrapper is present.
5. `grep -n 'RenderPromptShow' pkg/cmd/prompt_show.go` — function exists and is called from `Run`.
6. `grep -n 'Reading prompt-failure errors' docs/troubleshooting.md` — section header present.
7. `grep -n 'would be overwritten by merge\|exit status 2' docs/troubleshooting.md` — both literal strings present in the section body.
8. `grep -F 'fix: git wrappers in pkg/git/ now capture stderr' CHANGELOG.md` — exact new entry present (matching just `## Unreleased` is insufficient because the section may already contain other entries).
</verification>
