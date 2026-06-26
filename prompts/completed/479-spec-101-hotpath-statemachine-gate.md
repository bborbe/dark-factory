---
status: completed
spec: ["101"]
summary: Created scripts/hotpath-statemachine-check.sh gating the six migrated hot-path targets for inline prompt.PromptStatus type-token usage, wired into Makefile precommit chain alongside hotpath-logcheck; AC-6 transient edit was verified (gate tripped on pkg/cancellationwatcher/watcher.go) and reverted before final commit.
container: dark-factory-exec-479-spec-101-hotpath-statemachine-gate
dark-factory-version: v0.183.0
created: "2026-06-26T08:03:00Z"
queued: "2026-06-26T08:00:41Z"
started: "2026-06-26T09:11:05Z"
completed: "2026-06-26T09:18:58Z"
---

<summary>

- Adds a precommit gate that fails the build if any of the migrated hot-path files reintroduces an inline prompt-status comparison, locking the boundary so future edits keep routing state interpretation through the one owning package.
- The gate scans a checked-in allow-list of exactly the files this spec migrated (the five consumers plus the core orchestrator), mirroring the existing logging gate's scoped-package design — it is NOT a repo-wide scan.
- Wires the gate into the precommit dependency chain so it runs on every commit alongside the existing checks.
- Prints the offending file:line when it trips, so an author knows exactly where the leak is and can move the comparison behind the owning package.
- Documents the allow-list and the deliberate exclusions (CLI commands and repair tools that read status by design) so the scope is explicit and auditable.

</summary>

<objective>
Add a `make hotpath-statemachine-check` target backed by a checked-in shell script that greps for inline `prompt.PromptStatus` (and `prompt.<X>PromptStatus`) comparisons in the migrated hot-path files, exits non-zero on any offender (printing file:line), and is wired into the `precommit` dependency chain. Mirror the existing `scripts/hotpath-logcheck.sh` design.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-makefile-commands.md` — Makefile target conventions.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-security-linting.md` — script file perms.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — changelog format.

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/101-extract-unified-prompt-state-machine.md` — Desired Behavior item 6; Constraints; Failure Modes row 1; Acceptance Criteria 4, 5, 6, 7. PAY ATTENTION to the AC-4 reconciliation note below.

Prompts 2 and 3 MUST be on the tree (the five consumers + processor migrated). If `grep -rnE 'prompt\.PromptStatus' pkg/runner/lifecycle.go pkg/promptresumer/resumer.go pkg/committingrecoverer/recoverer.go pkg/queuescanner/scanner.go pkg/cancellationwatcher/watcher.go` returns any line, STOP and report `Status: failed` with message "consumers not yet migrated (prompts 2/3)".

Read these files END-TO-END before editing (mirror their pattern exactly):
- `/workspace/scripts/hotpath-logcheck.sh` — the EXACT script structure to mirror: `MODE` arg (`warn`/`strict`), `ROOT=$(cd "$(dirname "$0")/.." && pwd)`, a `PACKAGES`/files allow-list at the top, POSIX `grep -nE`, per-file `*_test.go` and counterfeiter skips, accumulate `OFFENDERS`, print to stderr, `exit 1` in strict mode. The new script is the same shape with a different pattern and allow-list.
- `/workspace/Makefile` lines 1-50 — the `precommit` target (line 16: `precommit: ensure format generate test hotpath-logcheck check addlicense check-changelog check-links`) and the `hotpath-logcheck` target (lines 45-47: `@bash scripts/hotpath-logcheck.sh strict`). Mirror the target; add the new target to the `precommit` dependency list.
- `/workspace/scripts/check-versions.sh` — for the leading-comment / executable-bit convention.

VERIFIED FACTS (do not re-derive):
- The five migrated consumer files: `pkg/runner/lifecycle.go`, `pkg/promptresumer/resumer.go`, `pkg/committingrecoverer/recoverer.go`, `pkg/queuescanner/scanner.go`, `pkg/cancellationwatcher/watcher.go`. Plus the migrated orchestrator: `pkg/processor/*.go` (non-test). These SIX targets are the gate's allow-list — the set this spec migrated.
- `pkg/promptstate/*.go` is the OWNER and is allow-listed OUT (it legitimately holds the `prompt.PromptStatus(...)` conversions inside `InterpretRawTuple`/`IsPreExecutionStatus`/`StatusFromRaw`).
- `pkg/prompt/prompt.go` is the storage type and is allow-listed OUT.
- `*_test.go` files are allow-listed OUT.

AC-4 RECONCILIATION (this is the resolved decision; implement the scoped gate and surface the note):
The spec AC-4 evidence command is a REPO-WIDE grep:
`grep -rE 'prompt\.PromptStatus' pkg/ | grep -v '^pkg/promptstate/' | grep -v '_test\.go:' | grep -v '^pkg/prompt/prompt\.go:'` and asserts it returns 0 lines. But the spec's stated SCOPE migrates only the five consumers (+ processor in prompt 3). These files use `prompt.PromptStatus` BY DESIGN and are NOT in scope: `pkg/cmd/cancel.go`, `pkg/cmd/prompt_complete.go`, `pkg/cmd/reject.go`, `pkg/cmd/spec_reject.go`, `pkg/cmd/unapprove.go`, `pkg/doctor/fix_orphan_in_progress.go`, `pkg/doctor/prompted_not_swept.go`, `pkg/doctor/status_dir_mismatch.go`, `pkg/runner/health_check.go` — interactive CLI commands and repair/doctor tools that read status as part of their job (validating transitions, repairing mislocated files), not the per-prompt daemon hot path.
RESOLVED: the GATE is SCOPED to the six migrated targets (mirroring `hotpath-logcheck`'s scoped-package design), NOT the repo-wide grep. This makes `make hotpath-statemachine-check` exit 0 on the migrated tree (AC-5) and exit non-zero when a comparison is reintroduced into any of the six (AC-6). The literal repo-wide AC-4 grep is intentionally NOT enforced by the gate, because it would force migrating out-of-scope CLI/doctor packages the spec did not list. Record this reconciliation in `## Improvements` (category PROMPT) so the spec author can confirm AC-4's wording should read "the five consumers" not "repo-wide". Do NOT migrate `pkg/cmd/*`/`pkg/doctor/*`/`health_check.go` in this prompt.

</context>

<requirements>

## 1. Create `scripts/hotpath-statemachine-check.sh`

Create `/workspace/scripts/hotpath-statemachine-check.sh`, mirroring `scripts/hotpath-logcheck.sh`. Make it `chmod 0755`. The leading comment block (mirror the logcheck header) MUST include the AC-4 reconciliation note from context (scoped, not repo-wide; the six in-scope targets; the out-of-scope `pkg/cmd/*`, `pkg/doctor/*`, `pkg/runner/health_check.go`; and the owner `pkg/promptstate`).

Behaviour:
- `MODE="${1:-strict}"` — accept `warn` (print, exit 0) and `strict` (print, exit 1 on offenders). Default `strict`.
- `ROOT=$(cd "$(dirname "$0")/.." && pwd); cd "$ROOT"`.
- Define the allow-list of targets to scan (the six migrated targets) at the top:
  ```sh
  FILES="pkg/runner/lifecycle.go pkg/promptresumer/resumer.go pkg/committingrecoverer/recoverer.go pkg/queuescanner/scanner.go pkg/cancellationwatcher/watcher.go"
  PACKAGES="pkg/processor"
  ```
  For each dir in `PACKAGES`, scan all non-test `*.go` files in the dir (mirror logcheck's per-dir loop). For each path in `FILES`, scan that file directly.
- For each scanned file: skip `*_test.go`; skip counterfeiter-generated files (`head -3 "$f" | grep -q "Code generated by counterfeiter"`).
- Match the offending pattern with POSIX `grep -nE`: `prompt\.[A-Za-z]*PromptStatus`. This catches `prompt.PromptStatus(...)` casts AND `prompt.CancelledPromptStatus`/`prompt.ExecutingPromptStatus`/etc. constant comparisons — every inline tuple-reading token the migration removed.
- Accumulate offenders as `file:line: <text>`, print to stderr with a header like `hotpath-statemachine-check: inline prompt-status comparison found (route via pkg/promptstate):`, and `exit 1` in strict mode (else `exit 0`).
- Use POSIX `grep`/`awk`/`sed` only — no GNU-specific flags (must run on macOS zsh AND Linux bash; spec Constraint).

After migration (prompts 2+3), running `bash scripts/hotpath-statemachine-check.sh strict` MUST exit 0 (no offenders in the six targets).

## 2. Add the `hotpath-statemachine-check` Makefile target

In `/workspace/Makefile`, add (mirror the `hotpath-logcheck` target at lines 45-47):

```makefile
.PHONY: hotpath-statemachine-check
hotpath-statemachine-check:
	@bash scripts/hotpath-statemachine-check.sh strict
```

## 3. Wire into `precommit`

In `/workspace/Makefile` line 16, add `hotpath-statemachine-check` to the `precommit` dependency list, adjacent to `hotpath-logcheck`:

```makefile
precommit: ensure format generate test hotpath-logcheck hotpath-statemachine-check check addlicense check-changelog check-links
```

This satisfies AC-7 (`grep -n 'hotpath-statemachine-check' Makefile` returns >= 1 line inside the precommit chain).

## 4. Document the gate's allow-list

The allow-list and its exclusions are documented in the script's leading comment header (the canonical, checked-in record). State there: the six in-scope targets; the out-of-scope `pkg/cmd/*`, `pkg/doctor/*`, `pkg/runner/health_check.go`; and the owner `pkg/promptstate`. Do NOT edit `docs/architecture-flow.md` in this prompt — prompt 5 owns that file and will reference the gate from the architecture doc, so editing it here would create a merge conflict.

## 5. Verify the gate trips on a reintroduced comparison (AC-6) — transiently

Verify, WITHOUT committing the transient edit:
1. Temporarily append a line `var _ = prompt.ExecutingPromptStatus` to `pkg/cancellationwatcher/watcher.go`.
2. Run `make hotpath-statemachine-check` — it MUST exit non-zero and print `pkg/cancellationwatcher/watcher.go:<line>` on stderr.
3. REVERT the transient edit (`cd /workspace && git checkout pkg/cancellationwatcher/watcher.go`) BEFORE running precommit. The committed tree MUST be clean of the transient edit.

Record in the completion report that AC-6 was verified transiently and reverted.

## 6. CHANGELOG

Append to `## Unreleased` in `/workspace/CHANGELOG.md` ONE bullet:

```
- feat: add make hotpath-statemachine-check gate (scripts/hotpath-statemachine-check.sh) wired into precommit; blocks inline prompt-status comparisons in the migrated hot-path files (spec 101 prompt 4)
```

</requirements>

<constraints>

- The gate is SCOPED to the six migrated targets (mirroring `hotpath-logcheck`'s scoped-package design) — NOT a repo-wide grep. Do NOT migrate or scan `pkg/cmd/*`, `pkg/doctor/*`, or `pkg/runner/health_check.go` (out of scope per the AC-4 reconciliation).
- `make hotpath-statemachine-check` MUST exit 0 on the migrated tree (AC-5) and non-zero on a reintroduced comparison in any of the six targets (AC-6).
- The existing `make hotpath-logcheck` MUST continue to pass — the new check AUGMENTS, never replaces (spec Constraint).
- The script uses POSIX `grep`/`awk`/`sed` only, no GNU-specific flags (spec Constraint — runs on macOS zsh and Linux bash).
- The allow-list lives in the checked-in script (spec Constraint).
- Script file mode `0755`; if any `#nosec` is needed for a shell-exec lint, add it WITH a reason (project rule). No Go code changes in this prompt.
- Do NOT edit `docs/architecture-flow.md` in this prompt (prompt 5 owns that file — avoid a conflict).
- Do NOT commit — dark-factory handles git. The transient AC-6 verification edit MUST be reverted before finishing.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# AC 5 — gate exits 0 on the migrated tree
make hotpath-statemachine-check; echo "exit=$?"
# expected: exit=0 (no offenders in the six targets)

# AC 7 — wired into precommit
grep -n 'hotpath-statemachine-check' Makefile
# expected: >= 2 lines (the target + the precommit dependency)

# script is executable and POSIX
test -x scripts/hotpath-statemachine-check.sh && echo "executable"
# expected: executable

# AC 6 — trips on a reintroduced comparison, then revert
printf '\nvar _ = prompt.ExecutingPromptStatus\n' >> pkg/cancellationwatcher/watcher.go
make hotpath-statemachine-check; echo "exit=$?"
# expected: prints pkg/cancellationwatcher/watcher.go:<line>, exit=1
git checkout pkg/cancellationwatcher/watcher.go
make hotpath-statemachine-check; echo "exit=$?"
# expected: exit=0 again (transient edit reverted)

# existing logcheck still green
make hotpath-logcheck; echo "exit=$?"
# expected: exit=0

# CHANGELOG entry present
grep -n 'spec 101 prompt 4' CHANGELOG.md
# expected: >= 1 line

# full precommit (now includes the new gate)
make precommit
# expected: exit 0
```

</verification>
