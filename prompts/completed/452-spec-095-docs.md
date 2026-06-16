---
status: completed
spec: [095-healthcheck-cli]
summary: Added `## Healthcheck` section + Troubleshooting + CLI Reference rows to docs/running.md, a 'first diagnostic step' intro paragraph to docs/troubleshooting.md, a cross-reference sentence to scenarios/003-smoke-test-container.md, and a feat(cli) bullet appended to the existing `## Unreleased` block in CHANGELOG.md. make precommit exits 0.
container: dark-factory-healthcheck-exec-452-spec-095-docs
dark-factory-version: v0.177.1
created: "2026-06-16T13:00:10Z"
queued: "2026-06-16T13:18:11Z"
started: "2026-06-16T14:25:56Z"
completed: "2026-06-16T14:30:02Z"
branch: dark-factory/healthcheck-cli
---

<summary>
- `docs/running.md` gains a `## Healthcheck` section that explains what the command probes, when to run it, and what the exit codes mean, placed adjacent to the existing `## Detecting State Anomalies` (doctor) section.
- `docs/troubleshooting.md` adds a top-level "First diagnostic step" paragraph that points operators at `dark-factory healthcheck` before any manual `docker logs` / config inspection, with a one-line copy-paste example.
- `scenarios/003-smoke-test-container.md` is annotated with a one-line cross-reference: "you can run `dark-factory healthcheck` for a faster automated check of the boot probe in isolation" — the markdown scenario is the operator-facing harness, and pointing it at the new command is the contract for "shared helper between the two" the spec AC requires.
- `CHANGELOG.md` gets a `## Unreleased` section with a single `feat(cli):` bullet referencing the new `healthcheck` subcommand. Per the project's changelog policy (prefix-driven version bump), the autoRelease daemon will determine the actual version number on tag.
- The `make precommit` gate stays green; the new docs are pure markdown and do not affect the Go build.
</summary>

<objective>
Update operator-facing documentation and the changelog for the new `dark-factory healthcheck` subcommand. The docs are the contract the spec's "docs/running.md section" and "docs/troubleshooting.md first-thing" ACs verify.
</objective>

<context>
Read these files first (paths absolute):
- `/workspace/docs/running.md` lines 234-262 — existing `## Detecting State Anomalies` (doctor) section structure; mirror the heading style
- `/workspace/docs/running.md` line 296+ — `## CLI Reference` table at the bottom; add a row for `healthcheck` if such a table exists
- `/workspace/docs/troubleshooting.md` line 1 is `# Troubleshooting`, line 2 is blank, line 3 is `## Reading prompt-failure errors`. There is NO existing intro section between line 1 and line 3 — req 2 creates one.
- `/workspace/docs/running.md` line 264+ — the existing `## Troubleshooting` table (in running.md, NOT troubleshooting.md) that points at `dark-factory doctor`; add a `dark-factory healthcheck` row adjacent. **Critical:** the Problem/Fix table is in `docs/running.md`, the file `docs/troubleshooting.md` has no such table.
- `/workspace/scenarios/003-smoke-test-container.md` lines 1-30 — the operator-facing scenario header; the cross-reference goes near the top
- `/workspace/CHANGELOG.md` lines 1-13 — the most recent entries; the new `## Unreleased` block goes at the top, before the highest existing `## v` heading
- Coding plugin docs (in-container): `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`, `/home/node/.claude/plugins/marketplaces/coding/docs/documentation-guide.md`
</context>

<requirements>

1. Edit `/workspace/docs/running.md` — add a new `## Healthcheck` section, placed BETWEEN the `## Detecting State Anomalies` section (ends around line 262) and the `## Troubleshooting` section (starts around line 264). Use the existing heading style (`## Heading`). Required content:
   - One-sentence description: what the command does and the seven probes it runs (Docker, image, boot, claude, mount, gh, notifications).
   - A `### Usage` subsection with at least these two rows:
     - `dark-factory healthcheck` — full probe sequence; exit 0 on all pass, non-zero with categorized table on any failure
     - `dark-factory healthcheck --no-claude` — skips the only token-spending probe; useful for cheap smoke runs or when `ANTHROPIC_API_KEY` is unset
   - A `### Exit codes` bullet list: 0 = all probes passed; non-zero = at least one probe failed (row in stdout table names the category and the captured error snippet).
   - A one-paragraph "When to run it" note recommending the command as the first diagnostic step for any mysteriously-failing prompt (the spec's "operator-facing diagnostic flow" entry in the spec's `See` line under Constraints).
   - A back-reference to `docs/troubleshooting.md` for the larger troubleshooting flow.

2. Edit `/workspace/docs/troubleshooting.md` — **create a new intro section** at the top of the file. The file currently has `# Troubleshooting` on line 1, blank line 2, then `## Reading prompt-failure errors` on line 3 (NO existing intro region). Insert a new "First diagnostic step" paragraph between line 2 and the `## Reading prompt-failure errors` heading: leave the existing `# Troubleshooting` heading + blank line, then write the paragraph, then a blank line, then the existing `## Reading prompt-failure errors` continues. The paragraph MUST contain the literal string `dark-factory healthcheck` (the spec's AC: `grep -n 'dark-factory healthcheck' docs/troubleshooting.md` returns ≥1 line, AND that line number is less than the line number of the next `## ` heading). Suggested wording (≤ 5 lines):
   > When a prompt mysteriously fails or stalls, run `dark-factory healthcheck` first. The command probes Docker, the container image, the boot sequence, the Claude session, the workspace mount, and (when configured) `gh` and notifications. It exits 0 on a full pass and non-zero with a categorized table naming the failing probe. If `healthcheck` passes, the pipeline is green and the failure is in prompt content or the project tree itself — proceed to `dark-factory doctor` next. The `--no-claude` flag skips the only token-spending probe.

3. Edit `/workspace/docs/running.md` (NOT troubleshooting.md) — add a new row to the existing `## Troubleshooting` table at lines 264-275. The table currently has columns `Problem | Fix` and starts with rows like `Lock error on start`, `Stale external references after a spec renumber`, etc. Add a new row at the END of the table (after the `go mod download fails` row):
   - Problem: `Prompt fails or stalls without obvious cause`
   - Fix: ``Run `dark-factory healthcheck` first; if it fails, follow the categorized table to the broken subsystem; if it passes, run `dark-factory doctor` for state-anomaly detection.``

4. Edit `/workspace/scenarios/003-smoke-test-container.md` — add a single sentence to the top of the document (after the `# Pre-release container smoke test` heading and before the first `## ` heading). The sentence MUST reference `dark-factory healthcheck` so the spec's "container-boot logic is shared between the new command and scenario 003" AC has a documented evidence trail. Suggested wording:
   > For a faster automated check of the boot probe alone (without running a full prompt), use `dark-factory healthcheck` — it executes the same container-boot logic as this scenario in a few seconds and exits non-zero if boot regresses.

5. Edit `/workspace/CHANGELOG.md` — add a `## Unreleased` section at the very top, BEFORE the highest existing `## v` heading. Format per the project's changelog policy:
   ```
   ## Unreleased

   - feat(cli): add \`dark-factory healthcheck\` subcommand that probes the full pipeline-execution stack (Docker, image, boot, claude, mount, gh, notifications) in fixed order with fail-fast semantics; exit 0 on all-pass, non-zero with a categorized table naming the failing probe; \`--no-claude\` skips the only token-spending probe. New shared boot helper under \`pkg/runner/\` is also reused by the scenario-003 smoke test.
   ```
   The single bullet covers prompts 1, 2a, and 2b collectively (the prompt focus is docs-only, but the changelog entry summarizes the whole feature per the changelog-guide rule "One bullet per logical change"). If a `## Unreleased` section already exists, APPEND the new bullet — do not replace existing content.

6. Verify each AC literally with a grep:
   ```bash
   # AC: docs/running.md documents the command
   grep -nE '^#+ .*[Hh]ealthcheck' docs/running.md   # ≥1 line
   grep -n 'dark-factory healthcheck' docs/running.md # ≥1 line
   # AC: docs/troubleshooting.md lists dark-factory healthcheck as the first thing
   grep -n 'dark-factory healthcheck' docs/troubleshooting.md  # ≥1 line
   # Confirm the first match's line number is less than the first ^## heading AFTER the intro
   grep -n '^## ' docs/troubleshooting.md | head -3
   # AC: CHANGELOG.md has the bullet
   grep -n '^## Unreleased' CHANGELOG.md
   grep -nE '^- .*healthcheck' CHANGELOG.md           # ≥1 line in the Unreleased block
   ```
   All four must produce non-empty results. If any returns empty, fix the corresponding file.

7. Run `make precommit`. Since the change is docs-only, this should be quick (no Go compile, no Go test, but `go.mod`/`go.sum` checks and any markdown linters configured in the project still run). The change is also non-Go — `make test` is unnecessary.

8. Optional but recommended: open the four edited files in `git diff` and skim the changes for typos and broken markdown link references. Markdown link rot will not fail `make precommit` (no markdown linter is configured for the project's docs directory as of the current state — verify with `grep -r 'markdownlint\|mdformat' Makefile` and report finding in `## Improvements` if absent), so a manual review is the only safety net.

</requirements>

<constraints>
- The shape of `dark-factory doctor`'s exit semantics (0 = clean, non-zero = findings) and table layout must be preserved by `healthcheck` — operators read both outputs and the mental model must stay one model.
- `pkg/runner/health_check.go` (spec 043, periodic container-liveness check) is not modified by this spec. It is a different surface; both can co-exist.
- The container-boot helper extracted under `pkg/runner/` must remain callable from both the `healthcheck` command and the scenario-003 harness; the helper must not depend on Cobra, on the `pkg/cmd/` package, or on the scenarios package — only on `pkg/runner/` and below.
- The `.dark-factory.yaml` schema is not extended by this spec (no new fields, no new defaults).
- All new Go code conforms to the project's coding rules: `errors.Wrapf` from `github.com/bborbe/errors` for every error path; no bare `return err`; no `fmt.Errorf`; `log/slog` to stderr.
- The probe execution must respect context cancellation: `Ctrl-C` aborts within ~1s and the command exits with a non-zero code distinct from a probe failure (agent decides exact code at impl time).
- See `docs/troubleshooting.md` for the operator-facing diagnostic flow this command slots into; the doc update lives in this prompt.
- See `docs/running.md` for the operator-facing "how to run dark-factory" surface this command is added to.
- Changelog prefix per the changelog guide: `feat:` for a new user-facing feature (minor version bump). Use `feat(cli):` to scope it to the CLI surface.
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
```bash
make precommit
```
Manual verification of every literal AC (copy-paste the grep commands from requirement 6 above and run each — all must produce non-empty output).
</verification>
