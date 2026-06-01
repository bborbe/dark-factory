---
status: draft
spec: [091-doctor-command]
created: "2026-06-02T00:00:00Z"
---

<summary>
- Adds a new `Detecting State Anomalies` section to `docs/running.md` that introduces the `dark-factory doctor` command, its 7 detection categories, and the `--fix`/`--yes`/`--verifying-stale-hours` flags
- Adds a `doctor` row to the CLI Reference table at the bottom of `docs/running.md`
- Adds a "Why no auto-fix on startup?" note in the troubleshooting section that links the removal of the silent reconciliation path to the new `doctor` flow
- Updates the daemon-vs-run "When to use" table with a footnote pointing to `dark-factory doctor` for projects that suspect stale references after spec renumbers
- Adds a new `## Unreleased` section at the top of `CHANGELOG.md` with two `feat:` lines covering (1) the new `dark-factory doctor` command and (2) the removal of the silent startup reconciliation path
- All edits are prose-only and CHANGELOG-only — no Go code, no test changes, no `make precommit`-driven concerns beyond the markdown lint rules the repo already enforces
- Confirms the spec's Acceptance Criteria #14 (CHANGELOG entries for both `dark-factory doctor` and silent reconciliation removal) and #13 (`make precommit` exits 0) are satisfied end-to-end

</summary>

<objective>
Document the user-visible behavior of the new `dark-factory doctor` command for operators (release notes + a discoverable spot in `docs/running.md`), and add a CHANGELOG entry that captures both halves of spec 091: the new command (prompts 1 + 2) and the daemon behavior change (prompt 3). This prompt is prose-only and CHANGELOG-only; no Go code is touched.

</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` for the CHANGELOG format and prefix conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/documentation-guide.md` for `docs/running.md` style.

Files to read end-to-end before editing:
- `/workspace/specs/in-progress/091-doctor-command.md` — full spec, especially Desired Behavior #1–#11 (the 7 detection categories + their fix-command lines), Acceptance Criteria #14 (CHANGELOG evidence), and § Summary
- `/workspace/prompts/1-spec-091-add-doctor-detection-package.md` — what the doctor DETECTS
- `/workspace/prompts/2-spec-091-add-doctor-fixer-and-cli.md` — what the doctor FIXES and the CLI surface (`--fix`, `--yes`, `--verifying-stale-hours=N`)
- `/workspace/prompts/3-spec-091-remove-silent-reconciliation.md` — what the daemon USED to do that is now removed
- `/workspace/CHANGELOG.md` lines 1–30 — the current top entries to see the format and the most recent `## v0.X.Y` header (so the new `## Unreleased` sits above all version sections)
- `/workspace/docs/running.md` lines 1–80 — the "Starting the Daemon" / "When to use daemon vs run" sections (good model for a new "Detecting State Anomalies" section)
- `/workspace/docs/running.md` lines 220–265 — the CLI Reference table that needs a new `doctor` row
- `/workspace/docs/running.md` lines 200–220 — the "Troubleshooting" section that needs a "Why no auto-fix on startup?" line
- `/workspace/docs/running.md` lines 20–35 — the "When to use daemon vs run" table that needs a footnote or extra row pointing to doctor

</context>

<requirements>

1. **Add a new `## Detecting State Anomalies` section to `/workspace/docs/running.md`** — insert it AFTER the `## Two ways to generate prompts from an approved spec` section (which ends at the `## Switching modes` subsection, ~line 90) and BEFORE the `## Versioning` section. The new section uses the same heading level (`##`) and `###` subsections as the surrounding sections.

   The section must contain:
   - A 2-paragraph intro: one paragraph explaining what the command is ("`dark-factory doctor` is a read-only diagnostic that scans your `specs/` and `prompts/` trees for state anomalies — duplicate spec numbers, stale `verifying` timestamps, orphan spec/prompt links, etc.") and one paragraph explaining the workflow ("Run on demand. Exits 0 when the project is clean, 1 with a categorized report when findings exist. Each finding names the affected file paths and a copy-paste command line that an operator can run to fix it manually.").
   - A `### Usage` subsection with the four invocations:
     - `dark-factory doctor` — scan and report; exit 0/1
     - `dark-factory doctor --fix` — scan, prompt `Apply? [y/N]` per finding, apply the safe ones
     - `dark-factory doctor --fix --yes` — same as `--fix` but auto-accepts all confirmations (for scripted cleanup)
     - `dark-factory doctor --verifying-stale-hours=48` — override the default 24h stale-`verifying` threshold
   - A `### Detection categories` subsection with a 7-row table — one row per category from the spec. The table columns: `Category | What it catches | Fix command the operator can copy-paste`. Anchor each row to the spec's exact wording:
     | Category | Catches | Copy-paste fix |
     |---|---|---|
     | `duplicate-spec-numbers` | Two `.md` files in the same lifecycle dir share a `NNN-` prefix | `dark-factory spec renumber <id-to-move>` |
     | `prompted-but-not-swept` | A spec is in `prompted` state but all its prompts are already `completed`/`rejected`/`cancelled` and it hasn't transitioned to `verifying` | `dark-factory spec sweep <spec-id>` |
     | `verifying-stale` | A spec is in `verifying` with no progress in the last 24h (configurable via `--verifying-stale-hours`) | `dark-factory spec verify <spec-id>` (informational — no auto-fix) |
     | `orphan-prompt-link` | A prompt's `spec: [NNN]` references a spec id with no `.md` file in any `specs/*/` dir | `dark-factory prompt unlink <prompt-id>` OR `dark-factory prompt relink <prompt-id> <new-spec-id>` |
     | `failed-but-merged` | A prompt with `status: failed` whose recorded commit SHA is now on `master` | `dark-factory prompt complete <prompt-id> --reason=merged-externally` |
     | `orphan-in-progress-prompt` | A prompt lives in `prompts/in-progress/` but its parent spec is already `completed` or `rejected` | `dark-factory prompt cancel <prompt-id>` OR `dark-factory prompt complete <prompt-id>` |
     | `status-dir-mismatch` | A spec or prompt's `status:` field contradicts the lifecycle directory it lives in (e.g. `status: completed` inside `specs/in-progress/`) | `dark-factory spec move <spec-id>` (or the prompt equivalent) |
   - A `### Audit log` subsection, 2 short paragraphs: one explaining that `dark-factory doctor --fix` appends one line per action to `.dark-factory/doctor.log` (mode 0644) for traceability, and one explaining that the `previous_id` frontmatter field on a renumbered spec records the prior number so the rename is reversible by reading the frontmatter. Cite the spec.
   - A `### Read-only by default` one-sentence note: "`dark-factory doctor` (without `--fix`) never writes to `specs/` or `prompts/`. Safe to run from CI, from a script, or from a Claude Code session — the worst case is a non-zero exit code."

2. **Add a `doctor` row to the CLI Reference table** at the bottom of `/workspace/docs/running.md` (~line 260). Insert it AFTER the `dark-factory spec mark-prompted` row, BEFORE the closing blank line. Use this exact text:

   ```
   | `dark-factory doctor [--fix] [--yes] [--verifying-stale-hours=N]` | Detect (and optionally fix) state anomalies in `specs/` and `prompts/` |
   ```

3. **Add a "Why no auto-fix on startup?" entry to the Troubleshooting table** in `/workspace/docs/running.md` (~line 207). Insert it AFTER the `Lock error on start` row:

   ```
   | Stale external references after a spec renumber (PR description, commit message, vault task) | Run `dark-factory doctor` to see affected files; the daemon no longer silently renumbers specs on startup — see [Detecting State Anomalies](#detecting-state-anomalies) |
   ```

4. **Update the "When to use daemon vs run" table** in `/workspace/docs/running.md` (lines ~22–28). Add a one-line note in the "Multiple projects" paragraph (line ~36, the "Each project has its own lock file" line) that points to `dark-factory doctor`:

   - Append to the "Multiple projects" paragraph: "**After a spec renumber or external-reference drift**, run `dark-factory doctor` from the project root before resuming the daemon — it surfaces stale external references the daemon can no longer auto-fix. See [Detecting State Anomalies](#detecting-state-anomalies)."

5. **Add a `## Unreleased` section to `/workspace/CHANGELOG.md`** — insert it ABOVE the `## v0.173.1` line at the top of the file (the file currently has version sections in descending order). The section uses the same `##` heading as version sections and contains exactly these three bullet lines (the third is the silent-reconciliation removal that prompt 3 did — the CHANGELOG entry for prompt 3's deletion is owned by THIS prompt per prompt 3's constraints):

   ```
   - feat: Add `dark-factory doctor [--fix] [--yes] [--verifying-stale-hours=N]` command that detects 7 categories of state anomalies in `specs/` and `prompts/` and prints a copy-paste fix line per finding; `--fix` applies the safe mutations under a per-file lock with an audit log at `.dark-factory/doctor.log`. Replaces the silent startup reconciliation that previously renamed spec files on duplicate-number detection.
   - feat: Renumber fix records `previous_id: NNN` in the renamed spec's frontmatter so the rename is traceable; linked prompt `spec:` fields are rewritten to the new id, prompt filenames are not touched.
   - fix: Remove silent startup reconciliation — the daemon no longer renumbers spec or prompt files on startup. If a state condition that previously triggered a rename is detected, the daemon continues without writing; operators run `dark-factory doctor` to find and fix such conditions on demand.
   ```

   Read the changelog-guide.md before writing the entries. The prefix should be `feat:` for the two new-command lines and `fix:` for the deletion line (per the prefix rules: `feat:` for new features → minor bump; `fix:` for behavior change → patch bump). The version bump is auto-determined by dark-factory from the prefixes; this prompt does not need to pick a version.

6. **Verify the edits** with three grep commands (these mirror the spec's Acceptance Criteria #14):

   - `cd /workspace && grep -n 'dark-factory doctor' CHANGELOG.md` returns ≥ 1 line.
   - `cd /workspace && grep -n 'silent.*reconcil\|startup reconciliation\|silent startup reconciliation' CHANGELOG.md` returns ≥ 1 line.
   - `cd /workspace && grep -n 'dark-factory doctor' docs/running.md` returns ≥ 3 lines (one in the new section's usage subsection, one in the CLI Reference table, one in the "When to use daemon vs run" paragraph).

7. **Run `cd /workspace && make precommit`** — must pass. The repo's precommit runs markdown linting; fix any reported issues (most likely: line length, blank-line-around-headings, no-trailing-punctuation-in-headings). If the precommit fails on something unrelated to the four files touched, document it in `## Improvements` rather than trying to fix it.

</requirements>

<constraints>
- DO NOT touch any Go source file. This prompt is prose-only + CHANGELOG-only.
- DO NOT add new CLI flags, new config keys, or new commands. The flag set is fixed by prompt 2; the only edits here are the documents that describe them.
- DO NOT add emojis. Per `claude-md-guide.md` and the project's existing `docs/running.md` style, prose is plain text only.
- DO NOT change the heading hierarchy in `docs/running.md`. The new `## Detecting State Anomalies` is a peer of `## Two ways to generate prompts…` and `## Versioning`; its `### Usage` / `### Detection categories` / `### Audit log` / `### Read-only by default` subsections mirror the surrounding section's style.
- DO NOT duplicate the spec's exact verbatim category strings (`duplicate-spec-numbers`, `prompted-but-not-swept`, etc.) in the prose — the table in step 1 uses them in code-fences, which is the right place. Surrounding prose uses natural-language references ("renumber", "stale verifying", etc.).
- DO NOT touch the existing `## Unreleased` section if one already exists in `CHANGELOG.md` (the spec at /workspace/CHANGELOG.md has none today, but if a future re-run of dark-factory has already inserted one, APPEND to it, do NOT replace). Verify with `grep -n 'Unreleased' CHANGELOG.md` first.
- DO NOT add a version bump line to the CHANGELOG. The `## Unreleased` section's content determines the bump via the `feat:` / `fix:` prefix rules in `changelog-guide.md`; dark-factory picks the version when the release daemon runs.
- DO NOT commit. dark-factory handles git.
- Existing tests in the repo continue to pass (no test files are touched by this prompt).
- File modes do not apply (this prompt touches only `.md` files, not source files).

</constraints>

<verification>
- `cd /workspace && make precommit` exits 0. (Per the project's CLAUDE.md: "Multi-service repos: ONLY run `make test`/`make precommit` in the service directory that was changed — NEVER at repo root." This is a single-service repo, so repo-root precommit is the right invocation.)
- The three grep commands in step 6 all return their expected minimum line counts.
- `cd /workspace && git diff --name-only HEAD` shows ONLY files under `docs/running.md` and `CHANGELOG.md`. No Go source files.
- `head -30 /workspace/CHANGELOG.md` shows the new `## Unreleased` section at the top, ABOVE `## v0.173.1`. Verify the position with `grep -n 'Unreleased\|^## v' /workspace/CHANGELOG.md | head -5` — the `Unreleased` line must have a smaller line number than the first `## v` line.
- `grep -n '## Detecting State Anomalies' /workspace/docs/running.md` returns 1 line.
- `grep -n 'doctor \[' /workspace/docs/running.md` returns ≥ 1 line (the new CLI Reference row uses the `dark-factory doctor [--fix]` syntax).
- Manual read-through of the new `docs/running.md` section confirms: the 7-row table includes all 7 categories from the spec; the `### Audit log` subsection mentions `.dark-factory/doctor.log`; the `### Read-only by default` note explicitly says "Safe to run from CI".

</verification>
