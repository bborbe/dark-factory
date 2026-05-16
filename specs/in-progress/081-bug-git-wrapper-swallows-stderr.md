---
status: approved
approved: "2026-05-16T11:48:19Z"
branch: dark-factory/bug-git-wrapper-swallows-stderr
---

## Summary

- When a shell-out to `git` fails inside `pkg/git/` wrappers (merge, checkout, rebase, pull, etc.), the returned error contains only the exit code and a Go stack trace.
- Git's actual stderr (e.g. "Your local changes would be overwritten by merge: foo.md, please commit or stash") is captured nowhere and never surfaces in `.dark-factory.log` or `dark-factory prompt show`.
- Operators must SSH into the worktree and re-run git manually to learn why the daemon failed — a 5-minute correlation step that git would have answered in one line.
- The fix is mechanical: capture stderr into a buffer at every git shell-out and include it verbatim in the wrapped error.
- This is a reporting bug, not a control-flow bug. The daemon correctly stopped on the failed merge; it just hid the reason.

## Problem

Daemon shell-outs to `git` capture exit code but discard stderr. When git returns non-zero, the wrapper returns `errors.Wrapf(ctx, err, "<op>")` where `err` is an `*exec.ExitError` carrying only the exit code. The actionable explanation — git's stderr — is dropped on the floor. Every operator-facing surface (`.dark-factory.log`, `dark-factory prompt show <id>`, any frontmatter diagnostics) inherits this gap. The result is that any git-side failure (dirty tree, index lock, auth, network, conflicts) reaches the operator as `exit status 2 <op-name>` with no causal information.

## Goal

When any `pkg/git/` wrapper fails, the operator sees git's own stderr verbatim in the error chain — in the daemon log, in `dark-factory prompt show`, and anywhere else the error is rendered. The cause of a git failure is identifiable without leaving the dark-factory log surface.

## Reproduction

dark-factory version: latest as of 2026-05-16.

1. In a project under daemon control (e.g. `~/Documents/workspaces/maintainer`), make the worktree dirty by modifying a tracked file without committing — pick a file the next daemon `sync` will attempt to merge from origin/master.
2. Queue and approve a prompt so the daemon enters the workflow-setup step that calls `MergeOriginDefault`.
3. Wait for the daemon to fail.

Observed in `.dark-factory.log`:

```
time=2026-05-16T13:06:19.268+02:00 level=ERROR msg="prompt failed" file=120-spec-030-verdict-parser-fix.md error="exit status 2
merge origin/master
github.com/bborbe/errors.Wrap
   /Users/bborbe/Documents/workspaces/go/pkg/mod/github.com/bborbe/errors@v1.5.13/errors_wrap.go:17
github.com/bborbe/dark-factory/pkg/git.(*brancher).MergeOriginDefault
   /Users/bborbe/Documents/workspaces/dark-factory/pkg/git/brancher.go:343
github.com/bborbe/dark-factory/pkg/processor.syncWithRemoteViaDeps
   /Users/bborbe/Documents/workspaces/dark-factory/pkg/processor/workflow_helpers.go:28
... [stack trace continues, no git stderr] ...
```

Manual investigation in the worktree reveals git's actual message:

```
error: Your local changes to the following files would be overwritten by merge:
        prompts/spec-031-task-controller-terminal-phase-gate.md
        specs/in-progress/031-bug-task-controller-respawns-on-terminal-phase.md
Please commit your changes or stash them before you merge.
Aborting
```

That message is absent from every dark-factory surface.

## Expected vs Actual

| Aspect | Expected | Actual |
|---|---|---|
| Daemon log on git failure | Error contains git's stderr verbatim | Only `exit status 2` plus wrapper label and Go stack trace |
| `dark-factory prompt show <id>` | Renders full error including git stderr | Renders same exit-code-only error |
| Operator action to diagnose | Read the daemon log | SSH to worktree, re-run `git status` / repeat the failing command |
| Time to root cause for dirty-tree case | Seconds | Several minutes |

Expected behavior is consistent with general Unix shell-out convention and with `errors.Wrap` usage elsewhere in the codebase (where surrounding context is included). This is not contradicting documented behavior — it is filling a gap that documentation does not address.

## Why this is a bug

The daemon's job during workflow setup is to either succeed or to tell the operator how to proceed. A failure mode that requires the operator to leave dark-factory's reporting surface and reproduce the failure manually defeats the daemon's purpose for that class of error. Every git wrapper in `pkg/git/` shares this shape, so this is systemic, not a one-off.

## Desired Behavior

1. Every `pkg/git/` shell-out to `git` captures stderr (and stdout) into a buffer before invoking the command.
2. When a git command exits non-zero, the wrapper's returned error contains git's captured stderr verbatim, in addition to the existing wrapper label.
3. The error surfaces unchanged through the daemon log and through `dark-factory prompt show <id>` — no truncation, no reformatting that drops the stderr.
4. Successful git commands log their captured stdout/stderr at DEBUG level (not discarded) so a `--log-level=debug` run shows what git said even on success.
5. The wrapper's exported function signatures remain `error`-returning — no API change to callers.

## Constraints

- Public signatures of `pkg/git/` wrapper functions (`MergeOriginDefault`, `Checkout`, `Rebase`, `Pull`, `Fetch`, `Push`, and peers) MUST NOT change. Callers continue to receive `error`.
- Error-message format change is additive only: existing prefix text (e.g. `merge origin/master: exit status 2`) remains; git stderr appears appended. Operators grepping logs for `merge origin/master` continue to match.
- No new external dependencies. Use stdlib `bytes.Buffer` for capture.
- Existing unit tests in `pkg/git/*_test.go` must continue to pass without modification, except those that asserted on the absence of stderr in error messages (none expected; verify during implementation).

## Workaround

Operator must:
1. Note the failing wrapper from the stack trace (`MergeOriginDefault`, `Checkout`, etc.).
2. SSH/cd into the project worktree where the daemon was running.
3. Manually run the equivalent git command (`git merge origin/master`, `git checkout <branch>`, etc.) and read git's stderr directly.

This costs several minutes per failure and is the entire motivation for the fix.

## Failure Modes

| Trigger | Expected behavior | Detection | Recovery |
|---|---|---|---|
| Git stderr is very large (e.g. thousands of conflict lines) | Error message includes captured stderr but is bounded — at minimum, the first N bytes (e.g. 8 KiB) plus a `(truncated)` marker | `grep '(truncated)' .dark-factory.log` for the failed prompt | Operator reads first N bytes; remainder is reproducible by re-running git manually |
| Git stderr is binary or has non-UTF-8 bytes (rare, hooks can produce this) | Wrapper does not panic; bytes are rendered with Go's standard quoting or stripped of control characters | `grep` for the wrapper label still matches | Wrapper preserves enough text to identify the failing command |
| Git command hangs (auth prompt, network) | Existing timeout / cancellation behavior unchanged; on cancel, the captured-so-far stderr is still included in the returned error | Daemon log shows context-cancelled error with whatever stderr arrived before cancel | Operator can see how far git got before being killed |
| Git command succeeds with warnings on stderr (e.g. "warning: refname...") | Function returns nil error; warnings logged at DEBUG level | `--log-level=debug` daemon run shows the warning | None required — no failure |
| Stderr capture itself fails (buffer alloc error, impossibly rare) | Wrapper still returns the original `*exec.ExitError` so the original behavior is preserved as a fallback | Daemon log shows exit code without stderr — same as today | Operator falls back to manual reproduction; no worse than current state |

## Acceptance Criteria

- [ ] Every git shell-out in `pkg/git/` captures stderr to a buffer. Evidence: `grep -rnE 'exec\.Command.*"git"' pkg/git/` lists all call sites; for each, the same file shows either `CombinedOutput()` or an explicit `Stderr = &stderrBuf` assignment in surrounding lines. Auditor walks the list and confirms zero unmatched sites.
- [ ] When a git wrapper fails with non-zero exit, its returned error string contains git's captured stderr verbatim. Evidence: a new Ginkgo test in `pkg/git/` injects a fake git binary (via PATH override or a test seam) that exits 2 and writes the literal string `INJECTED_STDERR_MARKER\nsecond line` to stderr; assertion: `err.Error()` contains both `INJECTED_STDERR_MARKER` and `second line`.
- [ ] Triggering-incident case is locked down: a Ginkgo test sets up a fake git that emits the verbatim "Your local changes to the following files would be overwritten by merge: foo.md\nPlease commit your changes or stash them before you merge.\nAborting" stderr with exit 2, calls `MergeOriginDefault`, and asserts the returned error contains all three lines verbatim. Evidence: test exists and passes.
- [ ] `dark-factory prompt show <failed-prompt-id>` renders the full error including git stderr without truncation in the path between processor → log → CLI. Evidence: a Ginkgo test against the CLI's prompt-show render function feeds an error whose `Error()` contains a multi-line stderr (including the literal `INJECTED_STDERR_MARKER` and `second line`); assertion: the renderer's captured output contains both lines verbatim. The rendering function must be made testable in isolation if it is not already — extracting it into a pure function is part of the fix.
- [ ] On success, git's stdout/stderr is logged at DEBUG level. Evidence: a Ginkgo test invokes a wrapper against a fake git that exits 0 with stdout `Already up to date.` and asserts the test logger received a DEBUG-level entry containing that string.
- [ ] Truncation guard: a Ginkgo test feeds 64 KiB of stderr through the wrapper and asserts the returned error contains the marker `(truncated)` and is bounded under some explicit byte limit (limit value — agent decides at impl time, but must be documented in a comment at the truncation site).
- [ ] No regression: `make precommit` exits 0 from `~/Documents/workspaces/dark-factory/`. Evidence: exit code 0.
- [ ] `docs/troubleshooting.md` (created if missing) contains a section titled "Reading prompt-failure errors" with a before/after example showing the dirty-tree case. Evidence: `grep -n 'Reading prompt-failure errors' docs/troubleshooting.md` returns a match; the section between that heading and the next contains the literal strings `would be overwritten by merge` and `exit status 2`.

## Verification

After the fix:

```bash
cd ~/Documents/workspaces/dark-factory
make precommit
```

Manual end-to-end (rung-2 equivalent — required because the daemon's error-rendering pipeline crosses process boundaries):

```bash
# In a daemon-controlled project (e.g. ~/Documents/workspaces/maintainer):
cd ~/Documents/workspaces/maintainer
# Dirty the worktree against a file that origin/master will touch on next merge
echo "stray edit" >> some-tracked-file.md
# Queue + approve a trivial prompt, start the daemon, wait for failure
# Then:
grep -A30 'prompt failed' .dark-factory.log | head -40
# Expected: the grepped block contains the literal text
# "Your local changes to the following files would be overwritten by merge"
dark-factory prompt show <failed-prompt-id>
# Expected: rendered error contains the same git stderr line
```

## Rung Verification

Pure code-correctness fix in `pkg/git/` plus minor error-string change. Rung-1 (`make precommit` in `pkg/git/` and unit tests with a fake-git helper) is the primary gate. No k8s involvement — dark-factory runs on developer machines.

Rung-2 equivalent is the manual end-to-end verification above: trigger a real merge failure and confirm git's stderr appears in `.dark-factory.log` and `dark-factory prompt show`.

## Do-Nothing Option

Operators continue the SSH-and-reproduce cycle on every git failure — minutes per incident, several per week during heavy spec churn. After ~3 such incidents the fix has paid back. Doing nothing leaves a permanent diagnostic tax on every git-related daemon failure.

## Out of Scope

- Retry-with-backoff for transient git errors (network, lock contention) — separate spec; this fix is about message quality, not auto-recovery.
- Restructuring `pkg/git/` into a typed-error hierarchy (`ErrDirtyTree`, `ErrAuthFailure`, etc.) — larger refactor; verbatim stderr capture is the minimum viable fix.
- Operator-hint pattern matching ("hint: worktree is dirty; commit or stash before retry" prepended on known stderr patterns) — explicitly deferrable to a follow-up spec. Verbatim stderr alone closes ~90% of the diagnostic gap.
- Any change to the workflow-setup control flow. The daemon correctly aborts on a failed merge; only the failure-reporting path is in scope.
- Changes to error rendering outside `pkg/git/` and the immediate operator-facing surfaces (log line, `prompt show`).
