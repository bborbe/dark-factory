---
status: approved
tags:
    - dark-factory
    - spec
approved: "2026-06-01T21:36:59Z"
generating: "2026-06-01T21:43:39Z"
prompted: "2026-06-01T22:30:09Z"
branch: dark-factory/doctor-command
---

## Summary

- Today the dark-factory daemon silently mutates spec numbers, prompt-spec links, and on-disk filenames during startup reconciliation. That silent reconciliation has broken real external references (PR descriptions, commit messages, vault tasks) without warning the operator.
- This spec replaces that silent path with an explicit, read-only `dark-factory doctor` command that detects state anomalies and prescribes copy-paste fix commands.
- The daemon stops auto-mutating state on startup. The most it may do is emit a `dark-factory doctor` suggestion to its own log.
- An opt-in `--fix` flag exists for operators who want guided, per-finding, confirmed mutations — with an audit log entry and a recorded `previous_id` for reversibility.
- Seven detection categories are in scope, drawn from the real failure shapes seen in operation. Anything broader (auto-retry, cross-project, schema migrations) is explicitly out of scope.

## Problem

On 2026-06-01 a daemon restart silently renamed an approved-and-prompted spec from `056-plugin-version-bump.md` to `057-plugin-version-bump.md` to resolve a duplicate-number collision detected during startup reconciliation. Six already-merged prompts (`219-spec-056-…` through `226-spec-056-…`) and external references (PR description, commit messages, OpenClaw task, daily note) now point to a spec number that no longer exists. The operator only noticed because `dark-factory spec complete 056-plugin-version-bump` started returning the wrong spec via prefix matching. Silent state mutation costs more than the duplicate it tries to fix: a duplicate prefix is a recoverable file-rename, but stale external references are unrecoverable trust damage. Operators need a tool that surfaces these anomalies, prescribes the fix, and never moves files behind their back.

## Goal

After this work:

- Anomalies in spec/prompt state are visible on demand via a single command (`dark-factory doctor`), which exits non-zero when findings exist and prints copy-paste fix commands.
- The daemon never silently mutates spec numbers, prompt-spec frontmatter links, or on-disk filenames during startup or any background tick. If it detects a state condition that previously triggered a rename, it logs a suggestion to run `dark-factory doctor` and continues without writing.
- Operators who want guided cleanup invoke `dark-factory doctor --fix`, which prompts per-finding before any write and records every action in an audit log.
- The seven detection categories listed in Desired Behavior are each verifiable against a fixture project.

## Non-goals

- Do NOT auto-fix on startup. Any code path that previously mutated spec numbers or prompt-spec links during daemon startup or background ticks is removed or demoted to log-only suggestion.
- Do NOT add an "auto-fix on startup" config flag — if a future consumer demands silent reconciliation, that's a separate spec. An escape hatch on the Goal is itself a regression.
- Do NOT retry, regenerate, or replay failed prompts from `doctor`. It reports; it doesn't execute prompt lifecycle.
- Do NOT cross project boundaries. `doctor` operates only on the project whose `.dark-factory.yaml` is the current root.
- Do NOT introduce schema migration capabilities. That's a separate `dark-factory migrate` spec if ever needed.
- Do NOT add new detection categories beyond the seven below in this spec. Additional detectors are follow-up specs.

## Desired Behavior

1. Running `dark-factory doctor` in a project with no anomalies prints the fixed string `no findings` to stdout and exits 0.
2. Running `dark-factory doctor` in a project with at least one anomaly prints one section per detection category that has findings, prints a copy-paste fix command line for each finding, and exits 1. No file under `specs/` or `prompts/` is modified.
3. Detection category — **duplicate spec numbers**: two or more `.md` files within `specs/in-progress/` (or within any single `specs/<lifecycle-dir>/`) share the same leading `NNN-` numeric prefix. Each finding names every colliding file path, its current `status`, and its `Linked Prompts` count. Fix-command line: `dark-factory spec renumber <id-to-move>`.
4. Detection category — **prompted spec with all prompts complete but not transitioned**: a spec file has `status: prompted` AND its `Linked Prompts: N/N` are all in a terminal state (`completed`, `rejected`, or `cancelled`) with zero in `draft|approved|queued|executing` AND the spec has not transitioned to `verifying`. Fix-command line: `dark-factory spec sweep <spec-id>`.
5. Detection category — **stuck `verifying`**: a spec file has `status: verifying` AND no frontmatter `Verifying:` timestamp progression in the last 24h (configurable via `--verifying-stale-hours=N`, default 24). Fix-command line: `dark-factory spec verify <spec-id>` (or operator runs `/dark-factory:verify-spec`).
6. Detection category — **prompt links non-existent spec**: a prompt file frontmatter `spec: [<id>]` references a spec id that has no corresponding file in any `specs/<lifecycle-dir>/`. Each finding names the prompt file and the missing spec id. Fix-command line: `dark-factory prompt unlink <prompt-id>` OR `dark-factory prompt relink <prompt-id> <new-spec-id>`.
7. Detection category — **failed prompt for already-merged commit**: a prompt file has `status: failed` AND its recorded commit SHA is present on the `master` branch's history (i.e. `git merge-base --is-ancestor <sha> master` succeeds). Fix-command line: `dark-factory prompt complete <prompt-id> --reason=merged-externally`.
8. Detection category — **orphan in-progress prompts**: a prompt file lives in `prompts/in-progress/` AND its parent spec has `status: completed` or `status: rejected`. Fix-command line: `dark-factory prompt cancel <prompt-id>` OR `dark-factory prompt complete <prompt-id>`.
9. Detection category — **status contradicts directory**: a spec or prompt file's frontmatter `status` value does not match the lifecycle directory it lives in (e.g. `status: completed` in `specs/in-progress/`, or `status: draft` in `specs/in-progress/`). Fix-command line: `dark-factory spec move <spec-id>` (or the prompt equivalent).
10. Running `dark-factory doctor --fix` interactively prompts `Apply? [y/N]` per finding before any mutation; with `--yes`, all confirmations auto-accept. Each applied fix appends one line to `.dark-factory/doctor.log` containing timestamp (RFC3339), finding category, target file path(s), action taken, before-state, after-state.
11. When `doctor --fix` renumbers a spec, the new spec file's frontmatter records `previous_id: NNN` so the rename is traceable. Linked prompt files' `spec:` frontmatter is rewritten to the new id; their filenames are NOT touched.
12. The daemon's existing startup reconciliation path is **removed entirely**. The silent renumber code path (the call site(s) under `pkg/processor/`, `pkg/factory/`, `pkg/specwatcher/`, `cmd/`, `main.go` that today rename spec files during startup) is deleted, not demoted to log. Any detection-from-daemon is out of scope for this spec — operators run `dark-factory doctor` on demand. Demote-to-log was considered and rejected: it keeps dead code on the silent-mutation surface and creates a permanent grep-allowlist entry; if scheduled in-daemon detection is wanted later, that is a separate spec running `doctor` on a timer, not a partial revival of the renumber path.
13. `dark-factory doctor` and `dark-factory doctor --fix` are read-only with respect to `specs/completed/`, `specs/rejected/`, `prompts/completed/`, `prompts/cancelled/`. These directories are reported on but never written.

## Constraints

- Detection logic reuses the existing scanners under `pkg/spec/`, `pkg/prompt/`, `pkg/specnum/`, `pkg/reindex/`. No new file-walking code is introduced; the doctor command depends on those packages.
- The `doctor` subcommand is wired into the existing CLI tree under `pkg/cmd/` following the shape of `pkg/cmd/spec_status.go`, `pkg/cmd/status.go`, etc. (Cobra/CLI conventions in this repo are frozen.)
- All Go code follows `~/Documents/workspaces/coding/docs/`: error wrapping via `github.com/bborbe/errors`, Ginkgo v2 + Gomega for tests, Counterfeiter for mocks.
- The project layout (`specs/`, `specs/in-progress/`, `specs/completed/`, `specs/rejected/`, `specs/ideas/`, `prompts/`, `prompts/in-progress/`, `prompts/completed/`, `prompts/cancelled/`) is frozen — `doctor` reads from but does not introduce new lifecycle directories.
- The `.dark-factory.yaml` schema for this project (`autoRelease: false`, `pr: false`, spec-flow project) is unchanged. No new config keys are required for the default `doctor` invocation; only `--verifying-stale-hours` is exposed as a CLI flag.
- Daemon behavior change is limited to removing/demoting the silent reconciliation path. No other daemon ticks, watchers, or processors are modified.
- Existing tests in `pkg/spec/`, `pkg/prompt/`, `pkg/reindex/`, `pkg/specsweeper/`, `pkg/specwatcher/` must still pass.
- See `docs/rules/spec-writing.md` for spec format and `docs/architecture-flow.md` for the current daemon lifecycle this spec changes.

## Failure Modes

| Trigger | Detection | Expected behavior | Reversibility | Concurrency | Recovery |
|---|---|---|---|---|---|
| `specs/in-progress/` does not exist (uninitialized project) | `doctor` start | Exit 1 with stderr `not a dark-factory project: missing specs/in-progress/`. Do not crash. | n/a | n/a | Operator runs `dark-factory init`, then re-runs `doctor`. |
| A spec file's frontmatter is unparseable YAML | per-file parse | Skip the file, emit one finding under a `parse-errors` section naming the file and the YAML error. Continue scanning other files. Exit 1. | n/a | n/a | Operator fixes the YAML by hand; re-runs `doctor`. Recovery evidence: `dark-factory doctor` no longer lists that file. |
| `--fix` invoked but a finding's target file was deleted between scan and confirmation | per-fix apply | Print `skipped: <path> no longer exists` to stderr, log a `skipped` audit-log line, continue with remaining findings. Exit 1. | reversible (no write) | safe | Operator re-runs `doctor` to re-scan. |
| `--fix` invoked concurrently with the daemon mid-write on the same file | per-fix apply | Use the same on-disk file-lock primitive `pkg/lock/` exposes (the lock used by `dark-factory spec approve`). On lock-acquire timeout (5s), print `skipped: <path> locked by another process`, log `skipped`, continue. Exit 1. | reversible | safe | Operator stops the daemon or retries when idle. |
| Disk full mid-`--fix` write | per-fix apply | The errored fix's audit-log line records `action: failed` with the OS error. The write is left in whatever partial state the OS produced; `doctor` does NOT attempt cleanup. Exit non-zero (2 = partial). | partial | safe (only one fix in flight at a time, no parallelism) | Operator frees disk, runs `doctor` again; the next scan re-identifies whatever state the partial write left. |
| Daemon's repurposed log-only suggestion fires every tick, flooding logs | ongoing | The log-only suggestion emits at most once per daemon process lifetime per unique finding signature. The signature is `<category>:<sorted-target-paths>`. | n/a | safe | Restart daemon to reset the dedupe table, or run `dark-factory doctor --fix` to make the underlying finding go away. |
| Operator runs `--fix` against a renumber finding; another spec was meanwhile approved into the next-free slot | per-fix apply | The renumber computes the next-free slot at apply-time, not scan-time. If that slot is now taken, recompute. If recomputation fails 3 times, abort that finding, log `failed: slot churn`, continue with the next. Exit 1 (still has the original finding). | partial (no write occurred) | safe | Operator re-runs `doctor --fix`. |

## Security / Abuse Cases

- The `doctor` command reads operator-owned files under the project root only. No HTTP, no external API. Untrusted input surface is limited to YAML frontmatter and filenames in directories the operator already controls.
- Filename parsing for the leading `NNN-` prefix uses the existing `pkg/specnum/specnum.go` regex `^(\d+)`, which is anchored and bounded. No risk of catastrophic backtracking.
- The `--fix` path writes only inside the project's `specs/`, `prompts/`, and `.dark-factory/` directories. Path-joining uses `filepath.Join` against the project root resolved by the existing `.dark-factory.yaml` loader. No traversal outside the project root is possible.
- `previous_id` frontmatter is a numeric field; rewriting it cannot inject arbitrary YAML.
- The audit log path `.dark-factory/doctor.log` is created with mode `0644`; the directory `.dark-factory/` uses `0755`. Existing repo conventions for `.dark-factory/` are followed.
- No prompt-injection surface: `doctor` does not invoke Claude or any agent. Findings are formatted from fixed templates.

## Acceptance Criteria

- [ ] `dark-factory doctor` in a fixture project with zero anomalies prints exactly `no findings\n` to stdout and exits 0 — evidence: exit code 0 AND `diff <(dark-factory doctor) <(echo 'no findings')` is empty.
- [ ] `dark-factory doctor` in a fixture project containing two `specs/in-progress/*.md` files sharing the prefix `056-` prints a section whose first line matches the regex `^[⚠!] .* share number 056` and contains both file paths and the literal line `dark-factory spec renumber 056-plugin-version-bump`. Exit code 1 — evidence: exit code AND `grep -c 'share number 056' run.out` returns 1 AND `grep -c 'dark-factory spec renumber' run.out` returns ≥1.
- [ ] `dark-factory doctor` against a fixture with a `status: prompted` spec whose linked prompts are all `completed` prints a finding under category "prompted-but-not-swept" naming the spec id and prints the fix line `dark-factory spec sweep <id>`. Exit 1 — evidence: `grep "prompted-but-not-swept" run.out` returns ≥1 line AND exit code 1.
- [ ] `dark-factory doctor` against a fixture with a `status: verifying` spec whose `Verifying:` timestamp is older than 24h prints a finding under category "verifying-stale". Exit 1 — evidence: `grep "verifying-stale" run.out` returns ≥1 line.
- [ ] `dark-factory doctor` against a fixture with a prompt frontmatter `spec: [999]` and no `999-*.md` file in any `specs/` lifecycle dir prints a finding under "orphan-prompt-link" naming the prompt file path and the missing spec id `999`. Exit 1 — evidence: `grep "orphan-prompt-link" run.out` and `grep "999" run.out` each return ≥1 line.
- [ ] `dark-factory doctor` against a fixture with a `status: failed` prompt whose recorded commit SHA `<sha>` satisfies `git merge-base --is-ancestor <sha> master` prints a finding under "failed-but-merged" and the fix line `dark-factory prompt complete <prompt-id> --reason=merged-externally`. Exit 1 — evidence: `grep "failed-but-merged" run.out` returns ≥1 line.
- [ ] `dark-factory doctor` against a fixture with `prompts/in-progress/<id>.md` whose parent spec has `status: completed` prints a finding under "orphan-in-progress-prompt". Exit 1 — evidence: `grep "orphan-in-progress-prompt" run.out` returns ≥1 line.
- [ ] `dark-factory doctor` against a fixture with `specs/in-progress/077-foo.md` whose frontmatter has `status: completed` prints a finding under "status-dir-mismatch" naming the file path and the contradiction. Exit 1 — evidence: `grep "status-dir-mismatch" run.out` returns ≥1 line.
- [ ] `dark-factory doctor --fix --yes` against the duplicate-number fixture writes the renumber: the second spec file is renamed to use the next free number, its frontmatter records `previous_id: 056`, every linked prompt's `spec:` frontmatter is rewritten to the new id, and `.dark-factory/doctor.log` gains one audit line per action — evidence: `grep -c '^previous_id: 056' specs/in-progress/057-*.md` returns 1 AND `grep -c 'spec: \[057\]' prompts/in-progress/*spec-056*.md` plus `prompts/completed/*spec-056*.md` accounts for every prior `spec: [056]` linkage AND `wc -l .dark-factory/doctor.log` returns ≥1.
- [ ] A second invocation of `dark-factory doctor` against the now-fixed fixture is clean — evidence: exit code 0 AND `diff <(dark-factory doctor) <(echo 'no findings')` empty. (Splits the `--fix` outcome from idempotence proof so each failure mode is debuggable independently.)
- [ ] Without `--fix`, no file under `specs/` or `prompts/` is modified by `dark-factory doctor` — evidence: `git status --porcelain specs/ prompts/` returns 0 lines after a `doctor` run against any fixture.
- [ ] The daemon source no longer contains a write-path that silently renumbers specs on startup. The renumber call (and any helper that mutates `specs/` files from a daemon startup path) is removed — evidence: `grep -rn 'Reindex\|RenumberSpecsAfterRemoval' pkg/processor/ pkg/factory/ pkg/specwatcher/ cmd/ main.go` returns 0 lines. No demote-to-log fallback (rejected in Desired Behavior #12).
- [ ] The `doctor` command package has a Ginkgo v2 + Gomega test suite covering all 7 detection categories with golden fixture projects — evidence: `go test ./pkg/cmd/doctor_test.go ./pkg/doctor/...` passes AND `grep -c 'Describe\|Context\|It' pkg/doctor/*_test.go` returns ≥21 (3 cases per category minimum).
- [ ] `make precommit` exits 0 in the project root — evidence: exit code 0.
- [ ] `CHANGELOG.md` records the daemon behavior change under an `Unreleased` entry: removal of silent startup reconciliation, addition of `dark-factory doctor` — evidence: `grep -n 'dark-factory doctor' CHANGELOG.md` returns line ≥1 AND `grep -n 'silent.*reconcil\|startup reconciliation' CHANGELOG.md` returns line ≥1.

## Verification

```
# In project root
make precommit
go test ./pkg/doctor/... ./pkg/cmd/...

# Against a fixture exercising all 7 categories:
dark-factory doctor                  # expect exit 1, all 7 sections present
dark-factory doctor --fix --yes      # expect exit 0, audit log populated
dark-factory doctor                  # expect exit 0, stdout 'no findings'

# Verify daemon no longer mutates state:
grep -rn 'Reindex\|RenumberSpecsAfterRemoval' pkg/processor/ pkg/factory/ pkg/specwatcher/ cmd/ main.go
```

## Do-Nothing Option

If we don't do this, the daemon continues to silently rename spec files on startup whenever it detects a duplicate prefix. The next time this fires, the operator will again discover stale external references after they've already been published — and the trust cost grows with each occurrence. The current behavior is not acceptable: silent state mutation is a class of bug that compounds, and the workaround (operators learning to never trust `dark-factory spec complete` by prefix) is worse than the disease. The duplicate detection itself is useful; the silent fix is the regression. This spec keeps the detection and removes the silent mutation.
