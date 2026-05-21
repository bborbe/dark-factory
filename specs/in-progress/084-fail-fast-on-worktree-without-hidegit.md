---
status: prompted
approved: "2026-05-21T20:47:27Z"
generating: "2026-05-21T21:19:49Z"
prompted: "2026-05-21T21:22:44Z"
branch: dark-factory/fail-fast-on-worktree-without-hidegit
---

## Summary

- Dark-factory currently crashes deep inside the container with `fatal: not a git repository: <parent>/.git/worktrees/<name>` when started from a git worktree, and even setting `hideGit=true` only partly helps: the spec generator's docker executor hardcodes `hideGit=false` at `pkg/factory/factory.go:654`, ignoring the resolved config.
- This spec restores end-to-end worktree support by (a) adding a startup precondition that refuses to launch from a worktree CWD when `hideGit=false`, and (b) plumbing the resolved `hideGit` value into the spec generator's executor — both halves are needed for the canonical pre-created-worktree workflow to actually work.
- Without the second fix, the new startup check would only move the failure earlier: the operator would set `hideGit=true`, satisfy the check, then watch the very next spec-generation container die with the same `fatal: not a git repository` error because line 654 still passes `false`.
- The host's `.git` is a file pointer to the parent repo's `worktrees/` directory, which is not mounted into the container. `hideGit=true` masks the pointer so the container sees a non-git workspace; the spec generator's executor must honor that masking the same way the prompt executor at `pkg/factory/factory.go:891` already does.
- Auto-enabling `hideGit` when a worktree is detected was considered and rejected in favour of fail-fast pedagogy: warnings scroll past, and explicit config matches dark-factory's existing design.

## Problem

Running `dark-factory daemon` or `dark-factory run` from inside a git worktree (a checkout where `.git` is a regular file pointing at `<parent>/.git/worktrees/<name>`) without `hideGit=true` is guaranteed to fail at container startup, because the container mounts the worktree's CWD but cannot follow the host-side worktree pointer back into the parent repo's git metadata. The resulting error surfaces inside the container, late, and is not self-explanatory to operators who do not already know the worktree topology. Documentation in the runbook has not closed the gap — Benjamin has hit the trap repeatedly. A startup-time precondition is needed.

## Goal

End-to-end worktree support: (1) starting dark-factory inside a git worktree without `hideGit=true` terminates immediately, before any container is launched, with a single error message that names the cause and points at the runbook and the in-repo troubleshooting doc; (2) when `hideGit=true` IS set from a worktree, both prompt execution and spec generation containers run successfully — the spec generator's executor receives the resolved `hideGit` value instead of the current hardcoded `false`. All other startup paths (regular repo, non-git directory) behave exactly as today.

## Non-goals

- Auto-enabling `hideGit` when a worktree is detected. Rejected: warning logs scroll past, and explicit config matches dark-factory's existing design.
- Mounting the parent repo's `.git/worktrees/` directory into the container. Out of scope for this spec.
- Addressing other operational footguns that arise from worktree usage (e.g. `autoRelease=true` from a feature branch). Separate spec if needed.
- Changing the config schema or adding new fields. The check and the plumbing both use the existing `hideGit` field.
- Refactoring the factory beyond the one-line `hideGit` pass-through at `pkg/factory/factory.go:654`. The fix mirrors the existing pattern at line 891 — no broader cleanup.

## Assumptions

- `os.Stat` on a `.git` entry in the operator-controlled CWD is cheap and safe; the path is not user-controlled at runtime.
- A `.git` symlink that resolves to a directory is treated as a regular repo (the resolved target's type is what matters). Symlinks to files would resolve to the worktree-pointer case and are handled accordingly.
- A git submodule's `.git` is also a regular file (containing `gitdir: ../.git/modules/<name>`). Submodules have the same in-container metadata-missing problem as worktrees, so this spec intentionally treats submodule CWDs the same way (refuse without `hideGit=true`). The operator workflow for running dark-factory inside a submodule is identical to a worktree.
- The factory plumbing fix at `pkg/factory/factory.go:654` mirrors the prompt-executor expression at `pkg/factory/factory.go:891` — there is no semantic difference between "the executor needs `.git` masking" for spec generation vs. prompt execution when running from a worktree.

## Desired Behavior

1. On `daemon` and `run` startup, dark-factory inspects the current working directory for the worktree signature: a path entry named `.git` exists and is a regular file (not a directory). Symlinks resolve to their target's type before evaluation.
2. If the worktree signature is present AND the effective `hideGit` value is `false`, dark-factory refuses to start and exits non-zero with an error that (a) names the detected condition, (b) names the remediation (`--set hideGit=true` or `.dark-factory.yaml` entry), and (c) references both the `PR via Pre-Created Worktree` runbook and the in-repo `docs/troubleshooting.md`.
3. If the worktree signature is present AND `hideGit=true`, startup proceeds unchanged AND spec-generation containers receive the same `hideGit=true` value so they do not crash with `fatal: not a git repository`.
4. If the worktree signature is absent (regular repo where `.git` is a directory, or no `.git` at all), startup proceeds unchanged regardless of `hideGit`.
5. The check runs early enough that no container is launched and no other side-effecting startup step (lock acquisition is acceptable; container spawn is not) occurs first when the check fails.
6. The check is deterministic — same CWD + same config produces the same outcome — and does not depend on running any `git` subprocess.
7. `pkg/factory/factory.go:654` (the spec generator's `executor.NewDockerExecutor` construction) passes the resolved `hideGit` value instead of hardcoded `false`. The expression mirrors `pkg/factory/factory.go:891` (`workflow == config.WorkflowWorktree || hideGit` or the equivalent resolved-config accessor at this site). The misleading comment `// hideGit — spec generators never need .git masking` is removed.

## Constraints

- The `hideGit` config field, its YAML key, its CLI override path, and its default value (`false`) do not change.
- The existing `.git/index.lock` precondition check in the runner (`pkg/runner/runner.go`) continues to function unchanged.
- All existing startup behavior in non-worktree directories is preserved: no new log lines, no new failure modes, no new latency.
- The check must not shell out to `git` — operators may run dark-factory in environments where the `git` binary on `PATH` differs from the worktree's git, and the check should rely only on filesystem inspection.
- One-shot CLI commands that do not start a container (e.g. `spec approve`, `prompt list`) are not affected — the check is gated to startup paths that would spawn a container.
- The factory pass-through fix MUST mirror the existing expression at `pkg/factory/factory.go:891` (`workflow == config.WorkflowWorktree || hideGit` or its functional equivalent on `cfg`). No new accessor, no new field, no behavioral divergence between the spec-generator's executor and the prompt-executor.

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery |
|---------|-----------|-------------------|----------|
| Worktree CWD, `hideGit=false` | Operator sees stderr `worktree CWD detected; hideGit=true required` plus runbook + troubleshooting-doc references; exit code non-zero | Refuse to start before any container spawn | Operator re-runs with `--set hideGit=true` or edits `.dark-factory.yaml` |
| Worktree CWD, `hideGit=true` | Normal startup logs; both prompt-execution AND spec-generation containers launch and stay alive | Proceed unchanged; spec-gen receives the resolved `hideGit=true` and does NOT crash with `fatal: not a git repository` | N/A |
| Submodule CWD (`.git` is a file pointing to `../.git/modules/<name>`), `hideGit=false` | Same detection as worktree case (treated identically by the check) | Refuse to start; same actionable error | Same as worktree: set `hideGit=true` |
| Submodule CWD, `hideGit=true` | Same as worktree+`hideGit=true` | Proceed unchanged | N/A |
| Regular repo (`.git/` is a directory) | Normal startup logs | Proceed unchanged | N/A |
| Non-git directory (no `.git` entry) | Normal startup logs | Proceed unchanged | N/A |
| `.git` is a symlink to a directory | Resolved target is a directory → treated as regular repo | Proceed unchanged | N/A |
| `.git` file is unreadable (permissions) | Startup error names the stat failure | Refuse to start with a stat error distinct from the worktree-detection error | Operator fixes filesystem permissions |
| Race: `.git` transitions file → directory between the check and container spawn | Not detected (single-shot check at startup) | Behavior follows the check's observation; on next restart the check re-runs | Operator restarts the daemon |
| Spec generator runs from a worktree with the OLD hardcoded `hideGit=false` at `factory.go:654` (regression risk) | Spec-gen container crashes with `fatal: not a git repository: <parent>/.git/worktrees/<name>` in `pkg/generator/...` stack trace; spec is marked failed | New regression test catches this and CI fails | Restore the pass-through expression at line 654; the test must remain green |

## Security / Abuse Cases

Not applicable. The check reads one filesystem entry (`.git`) in the operator-controlled CWD. No network input, no HTTP surface, no untrusted data crosses a trust boundary.

## Acceptance Criteria

**Fail-fast startup check:**

- [ ] In a directory where `.git` is a regular file (worktree) and the effective config has `hideGit=false`, running `dark-factory daemon` exits non-zero — evidence: exit code is non-zero AND stderr matches `grep -E 'worktree|hideGit'` returning ≥1 line AND stderr contains the substring `PR via Pre-Created Worktree` AND stderr contains the substring `docs/troubleshooting.md` (or the equivalent in-repo doc reference).
- [ ] In the same worktree directory, running `dark-factory daemon --set hideGit=true` proceeds past the worktree gate — evidence: stderr contains a log line that occurs only after the worktree check passes (e.g. `effective config` or `acquired lock`, whichever appears in the runtime order after the gate) within 10 seconds of startup; the worktree-gate error line `worktree|hideGit.*required` does NOT appear.
- [ ] In a regular repository where `.git` is a directory, running `dark-factory daemon` proceeds past the worktree gate regardless of `hideGit` — evidence: `grep -E 'worktree|hideGit.*required'` on stderr returns 0 lines AND a post-gate log line appears within 10 seconds.
- [ ] In a directory with no `.git` entry, running `dark-factory daemon` proceeds past the worktree gate regardless of `hideGit` — evidence: `grep -E 'worktree|hideGit.*required'` on stderr returns 0 lines.
- [ ] In a git submodule CWD (`.git` is a regular file with `gitdir:` content pointing into a parent's `.git/modules/`), `hideGit=false` triggers the same refusal as a worktree CWD — evidence: same exit code + grep shape as the worktree case.
- [ ] The same gating applies to `dark-factory run` — evidence: repeating the worktree+`hideGit=false` invocation with the `run` subcommand exits non-zero with the same error substring.
- [ ] No container is launched when the worktree check fails — evidence: `docker ps --filter name=dark-factory --format '{{.Names}}'` returns 0 matching containers within 5 seconds of a failed startup attempt (negative evidence). Note: this AC assumes the docker executor; if the executor pluggability ever extends to a non-docker backend, the evidence weakens to "no executor process spawned".
- [ ] When the worktree-gate fails, an unreadable `.git` file produces a stat-error message distinct from the worktree-detection message — evidence: triggering the stat path (e.g. `.git` with mode `0000`) yields stderr matching `grep -E 'stat|permission'` returning ≥1 line AND stderr does NOT match `grep -E 'worktree.*hideGit.*required'` (the two error shapes are observably different).

**Troubleshooting doc content:**

- [ ] `docs/troubleshooting.md` contains a section explaining the worktree-from-host + container `.git`-mount mismatch and the `hideGit=true` remediation — evidence: `grep -ni 'hideGit' docs/troubleshooting.md` returns ≥1 line AND `grep -ni 'worktree' docs/troubleshooting.md` returns ≥3 lines (the existing 2 + at least 1 new) AND the doc explicitly names the `fatal: not a git repository` failure signature.

**Spec-generator `hideGit` plumbing:**

- [ ] `pkg/factory/factory.go:654` no longer passes the literal `false` to the spec generator's `executor.NewDockerExecutor` — evidence: `grep -n 'false, // hideGit' pkg/factory/factory.go` returns 0 lines AND `grep -n 'hideGit' pkg/factory/factory.go` shows the spec-generator construction site uses the same resolved expression as line 891 (or its functional equivalent on `cfg`).
- [ ] The misleading comment `spec generators never need .git masking` no longer exists — evidence: `grep -n 'spec generators never need' pkg/factory/factory.go` returns 0 lines.
- [ ] With `hideGit=true` in a worktree, the spec generator's container does NOT crash with `fatal: not a git repository` — evidence: integration test in `pkg/generator/` (or a black-box test in `pkg/factory/`) creates a worktree-shaped temp directory (`.git` as a regular file), drives the spec generator with `hideGit=true`, and asserts the executor never observes the `fatal: not a git repository` failure pattern.

**Discipline:**

- [ ] Unit test exercises the detection helper across five CWD shapes: worktree (`.git` is a regular file), submodule (`.git` is a regular file with `gitdir:` content), regular repo (`.git` is a directory), non-git (no `.git`), and stat-error (`.git` exists but is unreadable) — evidence: `go test ./<package>/ -run <TestName>` exits 0 AND test names appear in `go test -v` output for each of the five cases.
- [ ] Unit or integration test exercises the startup gating across the four combinations: worktree+`hideGit=false` returns the gating error, worktree+`hideGit=true` does not, submodule+`hideGit=false` returns the gating error, regular repo does not — evidence: `go test ./pkg/runner/ -run <TestName>` exits 0 AND the test asserts both an error sentinel for failure cases and a nil error for pass cases.
- [ ] `make precommit` exits 0 — evidence: exit code 0.

## Verification

```
make precommit
```

Manual verification, exercised once before approving the spec as complete:

```
# Setup: a worktree of any repo
cd <repo>
git worktree add /tmp/df-worktree-check -b throwaway HEAD
cd /tmp/df-worktree-check

# (1) Fail-fast: expect non-zero exit; message naming worktree + hideGit + runbook + docs/troubleshooting.md
dark-factory daemon ; echo "exit=$?"

# (2) Pass-through: expect proceeds past the worktree gate (terminate with Ctrl-C once a post-gate log line appears).
# To exercise the spec-generator path end-to-end, drop an approved spec into specs/in-progress/
# and confirm the spec-gen container starts successfully (does NOT crash with `fatal: not a git repository`).
dark-factory daemon --set hideGit=true
```

## Do-Nothing Option

The status quo: dark-factory crashes inside the container with a low-context `fatal: not a git repository` error from BOTH the prompt-execution path AND the spec-generation path. Setting `hideGit=true` today only papers over the prompt-execution case; spec generation still hits the same crash because `factory.go:654` hardcodes `false` regardless of config. Documentation exists but has not prevented repeated occurrences — Benjamin has hit the trap during spec 033 work as well. The cost of doing nothing is repeated operator confusion, partial workarounds that leave half the daemon broken, and continued reliance on tribal knowledge to recognise the failure signature. The fix is small (one filesystem stat + one conditional + a one-line pass-through at line 654) and bounded; the do-nothing option is not acceptable.
