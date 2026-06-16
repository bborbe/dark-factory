---
status: completed
spec: [096-healthcheck-on-daemon-startup]
summary: 'Documented the daemon healthcheck startup gate across configuration.md (Healthcheck Startup Gate section + --skip-healthcheck CLI flag), running.md (startup gate subsection cross-referencing the CLI probe sequence), architecture-flow.md (terminal-failure policy paragraph), and CHANGELOG.md (feat: docs entry under ## Unreleased).'
container: dark-factory-healthcheck-startup-exec-456-spec-096-docs-and-changelog
dark-factory-version: v0.180.2
created: "2026-06-16T20:10:00Z"
queued: "2026-06-16T20:22:17Z"
started: "2026-06-16T21:04:17Z"
completed: "2026-06-16T21:05:51Z"
branch: dark-factory/healthcheck-on-daemon-startup
---

<summary>
- Documents the new daemon healthcheck startup gate for operators across the three docs that cover daemon startup and configuration.
- The configuration reference gains a "Healthcheck Startup Gate" section listing the two new settings, their defaults, and the source-precedence note.
- The new `--skip-healthcheck` CLI flag is documented alongside the existing `--skip-preflight` flag, including its daemon-only scope and position-agnostic behaviour.
- The running guide's healthcheck section cross-references the new daemon-startup invocation so operators understand the gate is the same probe sequence the CLI runs.
- The architecture failure-policy section is extended so the gate's terminal-exit semantics sit next to the identical preflight policy.
- The changelog records the feature so dark-factory's auto-release picks up the version bump.
- No code changes — documentation only; this prompt lands after the behaviour ships so the wording reflects reality.
</summary>

<objective>
Document the shipped daemon healthcheck startup gate: add a configuration section for `healthcheckEnabled` / `healthcheckInterval` and the `--skip-healthcheck` flag, cross-reference the gate from the running guide's healthcheck section, extend the architecture failure-policy section to cover the gate's terminal semantics, and add a `## Unreleased` CHANGELOG entry.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions (especially the Plugin Release Checklist and Changelog rules — but note: this is a docs-only prompt, do NOT bump plugin versions here; only the `## Unreleased` CHANGELOG entry is in scope).

Read these files fully before editing:
- `/workspace/docs/configuration.md` — the "Preflight Baseline Check" section (around the `preflightCommand`/`preflightInterval` table) and the "CLI Flags" section (the `--skip-preflight` block). Mirror both for the healthcheck gate.
- `/workspace/docs/running.md` — the "Healthcheck" section (starts at the `## Healthcheck` heading describing the seven-probe CLI sequence).
- `/workspace/docs/architecture-flow.md` — the "## Preflight Failure Policy" section.
- `/workspace/CHANGELOG.md` — top of file; check whether a `## Unreleased` block already exists (append, don't replace).

Coding-plugin docs (in-container — read it):
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — entry format `- <prefix>: <what> [context]`; prefix required; `feat:` for a new feature.

Verified facts (from the shipped behaviour in prompts 1-3):
- Config fields: `healthcheckEnabled` (bool, default `true`), `healthcheckInterval` (Go duration string, default `"8h"`).
- `healthcheckEnabled: false` disables the gate entirely.
- `--skip-healthcheck` is a CLI flag valid ONLY for `daemon` (not `run`); position-agnostic; no cache read/write; emits `healthcheck skipped via --skip-healthcheck`.
- The gate runs at daemon startup AFTER preflight, BEFORE the watch loop; failure is terminal (non-zero exit, `^healthcheck failed: .+$`).
- Successful results are cached on the host filesystem under `~/.dark-factory/healthcheck-cache/`, success-only, keyed by SHA256 of `<containerImage>:<projectName>:<intervalSeconds>`; failures are never cached; corrupt/future-dated cache → treated as a miss and re-run.
- Effective-config log line includes `healthcheckEnabled`, `healthcheckEnabledSource`, `healthcheckInterval`, `healthcheckIntervalSource` (source values: `default`, `project`, or `arg`).
- The gate reuses the SAME probe sequence as `dark-factory healthcheck` (docker, image, boot, claude, mount, gh, notifications).
</context>

<requirements>
1. **`/workspace/docs/configuration.md`** — after the "Preflight Baseline Check" section (immediately before the "### CLI Flags" heading), add a new section:
   ```markdown
   ### Healthcheck Startup Gate

   On `dark-factory daemon` start, run the same probe sequence as `dark-factory healthcheck`
   (Docker daemon, container image, container boot, Claude session, workspace mount, `gh` auth
   when `pr: true`, notification channels when configured) once before the prompt-watch loop
   begins. This re-validates the pipeline stack the daemon depends on, complementing the
   project-level `preflightCommand` (which only proves the project compiles in-container).

   The gate runs only in `daemon` mode — `run` (one-shot) is unaffected.

   ```yaml
   healthcheckEnabled: true
   healthcheckInterval: "8h"
   ```

   | Field | Default | Purpose |
   |-------|---------|---------|
   | `healthcheckEnabled` | `true` | Whether the daemon runs the healthcheck startup gate. Set `false` to disable the gate entirely — the daemon proceeds directly to the watch loop. |
   | `healthcheckInterval` | `8h` | How long a successful healthcheck result is cached. A daemon restart within the interval skips the probes (cache hit). Failures are never cached, so an operator fix (image rebuild, token rotation) is picked up on the next start. Accepts Go duration strings: `"30m"`, `"2h"`, `"8h"`. Invalid strings are rejected at daemon startup. |

   **Caching:** Successful results are cached on the host under `~/.dark-factory/healthcheck-cache/`, keyed by container image + project + interval, so different repos never collide. Only successes are cached; a corrupt or future-dated cache file is treated as a miss and the gate re-runs.

   **On failure:** the daemon exits non-zero with a category-naming cause (e.g. `healthcheck failed: docker daemon unreachable`), fires a notification, and does NOT cache the result — matching the preflight terminal-failure policy.

   **Source precedence:** the effective-config startup log reports `healthcheckEnabled`/`healthcheckInterval` together with `healthcheckEnabledSource`/`healthcheckIntervalSource` (`default`, `project`, or `arg`) so the active gate config is auditable.

   **Override:** pass `--skip-healthcheck` to `daemon` to bypass the gate for a single invocation — see [CLI Flags](#cli-flags) below.
   ```
   (Keep the triple-backtick fences balanced; the YAML block and the inner code fence must each open and close.)

2. **`/workspace/docs/configuration.md`** — in the "### CLI Flags" section, after the `--skip-preflight` block (before `**`--model NAME`**`), add:
   ```markdown
   **`--skip-healthcheck`**

   ```bash
   dark-factory daemon --skip-healthcheck
   ```

   Bypasses the healthcheck startup gate for this invocation (daemon only — the flag is rejected on other commands). When set:

   - The probe sequence is not executed.
   - No healthcheck cache is read or written.
   - The daemon proceeds directly to the watch loop.
   - A startup log line records that the healthcheck was skipped.

   The flag is position-agnostic: `dark-factory --skip-healthcheck daemon` and `dark-factory daemon --skip-healthcheck` are equivalent.

   **Safety note:** the daemon may run prompts against a broken pipeline stack when this flag is used; a stack failure then surfaces mid-prompt instead of at startup. Use only as an explicit override. The flag does not persist; the next invocation runs the gate as configured.
   ```

3. **`/workspace/docs/running.md`** — in the `## Healthcheck` section, add a paragraph (after the existing description of the CLI's seven-probe sequence) cross-referencing the daemon startup gate:
   ```markdown
   ### Healthcheck startup gate (daemon)

   `dark-factory daemon` runs this same probe sequence once at startup, before the
   prompt-watch loop begins, as a startup gate. A successful result is cached for
   `healthcheckInterval` (default `8h`), so a restart within the window skips the probes.
   A failure is terminal: the daemon exits non-zero with the failing probe category. Disable
   the gate with `healthcheckEnabled: false`, or bypass it for one invocation with
   `--skip-healthcheck`. See [configuration.md](configuration.md#healthcheck-startup-gate)
   for details. The gate runs only in `daemon` mode; `run` (one-shot) is unaffected.
   ```

4. **`/workspace/docs/architecture-flow.md`** — extend the "## Preflight Failure Policy" section. Append a paragraph at the end of that section (after the `**Rule:**` paragraph, before the next `##` heading) describing the gate's identical semantics:
   ```markdown
   The **healthcheck startup gate** (daemon-only) follows the same terminal policy: on
   `dark-factory daemon` start it runs the healthcheck probe sequence once before the watch
   loop. A gate failure exits the daemon non-zero with a category-naming cause (e.g.
   `healthcheck failed: docker daemon unreachable`) and fires a notification — no retry, no
   skip-and-continue. Successful results are cached for `healthcheckInterval`; failures are
   never cached, so an operator fix is re-checked on the next start. The gate is disabled with
   `healthcheckEnabled: false` and bypassed for one run with `--skip-healthcheck`.
   ```
   Keep the existing `## Preflight Failure Policy` heading unchanged. Append the new paragraph beneath it. Do NOT rename the heading.

5. **`/workspace/CHANGELOG.md`** — add a `## Unreleased` block at the top (or append to the existing one) with a `feat:` entry:
   ```markdown
   - feat: Add healthcheck startup gate to `dark-factory daemon` — runs the healthcheck probe sequence once at startup (after preflight, before the watch loop), with `healthcheckEnabled`/`healthcheckInterval` config, success-only host cache, and a `--skip-healthcheck` flag; gate failure is terminal
   ```
   Follow `changelog-guide.md`: do NOT copy verification bash comments, do NOT use the prompt filename, describe what was implemented. If `## Unreleased` already exists, append this bullet to it.

6. Do NOT change any `.go` files, `.dark-factory.yaml`, plugin version JSON, or `plugin.json`/`marketplace.json` in this prompt. Documentation + CHANGELOG only.
</requirements>

<constraints>
- Copied from spec: the gate applies only to `dark-factory daemon`; `run` (one-shot) is out of scope — docs must say so.
- Copied from spec: failure is terminal, mirroring the preflight failure policy; success-only caching; failures never cached.
- Copied from spec: `--skip-healthcheck` is daemon-only, position-agnostic, no cache read/write.
- Docs must reflect the SHIPPED behaviour from prompts 1-3 — do not document config knobs the spec forbade (no `healthcheckCommand`; no per-prompt or periodic healthcheck loop).
- Do NOT commit — dark-factory handles git.
- Do NOT bump plugin versions — this is a docs/CHANGELOG-only prompt.
</constraints>

<verification>
Run in `/workspace`:
```bash
grep -n 'healthcheckEnabled' docs/configuration.md
grep -n 'healthcheckInterval' docs/configuration.md
grep -n 'skip-healthcheck' docs/configuration.md
grep -n 'healthcheck startup gate\|Healthcheck startup gate' docs/running.md
grep -n 'healthcheck' docs/architecture-flow.md
# Verify ## Unreleased block exists AND contains a healthcheck-related bullet
grep -nE '^## Unreleased$' CHANGELOG.md                   # must return exactly 1 line
awk '/^## Unreleased$/{f=1;next} /^## /{f=0} f && /healthcheck/{print}' CHANGELOG.md  # must return ≥1 line
```
- Each `grep` must return at least one line.
- The `## Unreleased` existence check MUST return exactly 1; if missing, the agent edited the wrong block (e.g. accidentally appended into `## v0.180.2`) and must add the Unreleased block.
- This is a docs-only change with no Go edits, so `make precommit` is not required for verification; if the project's precommit lints markdown, run it and ensure it exits 0. Otherwise confirm the markdown fences are balanced (the new sections render correctly).
</verification>
