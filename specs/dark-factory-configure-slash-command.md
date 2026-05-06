---
status: draft
---

# `/dark-factory:configure` slash command — interactive create/reconfigure/auto-migrate for `.dark-factory.yaml`

## Summary

Today there is one Claude Code slash command for project setup: `/dark-factory:init-project`. It only handles greenfield projects (refuses to run when `.dark-factory.yaml` already exists). Operators who want to:

- **Add a missing field** (e.g. enable `autoMerge` on an existing project)
- **Reconfigure a flag** (e.g. switch `workflow: direct` → `worktree`)
- **Migrate after a breaking config change** (e.g. spec 073 removed `autoReview`/`allowedReviewers` — operators with legacy configs must edit by hand)

…are on their own. They open the YAML in an editor, guess at the new shape, restart the daemon, and find out from the friendly-error message what they got wrong.

This spec adds `/dark-factory:configure` — a single slash command that creates, reconfigures, OR auto-migrates `.dark-factory.yaml`. It detects the project's current state and routes to the right flow. The slash command is modeled on `~/Documents/workspaces/semantic-search/commands/configure.md`, which already does this for the semantic-search HTTP service (greenfield install vs. add another instance vs. reconfigure existing).

## Problem

- `init-project` refuses on existing configs — operators who want to tweak one field have no guided path.
- Schema migrations (like spec 073's removal of three fields) currently surface as friendly errors at daemon startup; the slash command should be able to apply the migration automatically when the YAML is the only thing blocking startup.
- Every config change today requires the operator to remember which fields exist, what their defaults are, and which combinations validate (e.g. `autoMerge` requires `pr: true` for any branch workflow).
- Without a guided slash command, the cost of changing config is high enough that operators either over-configure (set every field "just in case") or under-configure (silent fallback to defaults they didn't intend).

## Goal

`/dark-factory:configure` is the one entry point for any `.dark-factory.yaml` change. It auto-detects the project state and routes:

| State | Flow |
|---|---|
| No `.dark-factory.yaml` | Same as `init-project` — greenfield create |
| `.dark-factory.yaml` exists, validates cleanly | Reconfigure flow — show current values, ask which to change |
| `.dark-factory.yaml` exists, fails to load (legacy fields, schema mismatch) | Auto-migrate flow — propose the new YAML, show the diff, write on approval |

After running the slash command, `dark-factory daemon` starts cleanly without the operator hand-editing YAML or grepping `pkg/config/config.go`.

## Non-goals

- Removing or replacing `init-project`. The two slash commands coexist; `configure` is the superset that delegates to `init-project` for greenfield. (Or `init-project` becomes a thin wrapper around `configure --new`. Decide in the fix prompt.)
- Editing fields outside `.dark-factory.yaml` (no global `~/.dark-factory/config.yaml` editing in v1; add later if needed).
- Schema discovery from arbitrary Go reflection. The slash command reads field metadata from a single source (the `Defaults()` function and yaml tags in `pkg/config/config.go`); no plugin system, no auto-discovery.
- Bitbucket-Server-specific guided setup. Provider stays explicit; the slash command prompts for `provider: github | bitbucket-server` and asks for the Bitbucket fields when needed.

## Desired Behavior

1. Running `/dark-factory:configure` with no `.dark-factory.yaml` present runs the existing `init-project` greenfield flow (or its successor) — no behavior change for new projects.
2. Running the slash command against a valid existing `.dark-factory.yaml` shows the current effective values (per-field, with `default` / `set in yaml` annotations), then offers a single AskUserQuestion menu of common changes (e.g. "1. Switch workflow", "2. Enable autoMerge", "3. Toggle autoRelease", "4. Add/edit allowedReviewers replacement (branch protection)", "5. Edit a specific field by name", "6. Migrate to latest schema", "7. Cancel").
3. Running the slash command against a `.dark-factory.yaml` that fails to load with a friendly-error (e.g. spec 073's "unknown field `autoReview`") detects the error class, proposes the migration (drop the deprecated field, suggest the GitHub branch-protection replacement), shows the diff, writes on approval.
4. Every write is preceded by a diff display and an explicit AskUserQuestion confirm. The slash command never writes silently.
5. After every write, the slash command runs `dark-factory config` (or equivalent validate command) to confirm the new YAML loads. On failure, it reverts to the prior version (kept in `.dark-factory.yaml.bak`) and reports the error.
6. The slash command is self-contained: it reads field metadata from the dark-factory binary itself (e.g. `dark-factory config defaults --json` if available, otherwise the docs-shipped reference table), so it does not require manual sync when fields are added.
7. Auto-migrate handles the spec 073 removal class out of the box: drop `autoReview`, `allowedReviewers`, `useCollaborators`, `maxReviewRetries`, `pollIntervalSec` if present; emit a one-line note pointing to GitHub branch protection as the replacement.

## Constraints

- The slash command MUST NOT modify any file outside the project root and the user's MCP/Claude config.
- The slash command MUST back up `.dark-factory.yaml` to `.dark-factory.yaml.bak` before any write.
- The slash command MUST present a diff before any write — no silent overwrites.
- The slash command MUST validate the new YAML by invoking the dark-factory binary (`dark-factory config` or `dark-factory daemon --check`, whichever exists) and revert on failure.
- The slash command MUST handle the case where `dark-factory` is not on `$PATH` — fall back to manual validation hints, do not silently no-op.
- The slash command MUST preserve YAML comments and key order from the existing file when only changing a subset of fields. (Use a YAML round-trip library, not naive marshal/unmarshal.)
- The slash command MUST NOT introduce a new schema field. Adding fields is the daemon's job; the slash command's job is configuring existing fields.
- Failure modes (port-in-use-style equivalents, e.g. invalid token, invalid workflow value) MUST surface a clear actionable message — mirror the friendly-error pattern from spec 073.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `.dark-factory.yaml` is well-formed and valid | Reconfigure menu offered | Operator picks an action |
| `.dark-factory.yaml` is well-formed but rejected by `dark-factory config` (legacy fields per spec 073) | Auto-migrate flow proposes the new YAML, shows diff, writes on approval | Operator approves; daemon starts cleanly |
| `.dark-factory.yaml` has YAML syntax errors | Slash command surfaces parse error with line + column from yaml lib; offers "edit by hand" exit | Operator fixes manually, re-runs slash command |
| Operator declines a proposed change | Slash command exits with no write; original file untouched | Re-run later |
| Validation after write fails | Slash command restores `.dark-factory.yaml.bak`, reports the validation error | Operator inspects, re-runs |
| `dark-factory` binary not on PATH | Slash command warns, asks the operator to install/add to PATH; offers to write the YAML anyway with a "validate-yourself" hint | Operator installs binary or accepts hint |
| Greenfield (no file) | Delegates to `init-project` (or its successor) | Existing greenfield path |

## Acceptance Criteria

- [ ] `commands/configure.md` exists in the dark-factory plugin's commands directory and is invocable as `/dark-factory:configure`.
- [ ] Frontmatter declares `allowed-tools: [Read, Bash, Write, Edit, AskUserQuestion]` (mirrors `init-project`).
- [ ] With no `.dark-factory.yaml` present, the slash command produces the same end state as running `/dark-factory:init-project`.
- [ ] With a valid `.dark-factory.yaml`, the slash command prints current effective values and offers a numbered AskUserQuestion menu of common reconfigure actions.
- [ ] With a `.dark-factory.yaml` containing legacy `autoReview: true`, the slash command detects the spec-073 friendly error, proposes the migration (drop `autoReview` + sibling fields, point to branch protection), shows the diff, writes on approval.
- [ ] After every write, the slash command validates the result via `dark-factory config` (or equivalent) and reverts to `.dark-factory.yaml.bak` on failure.
- [ ] YAML comments and key order from the original file are preserved when changing a subset of fields.
- [ ] Slash command exits cleanly with a single-paragraph summary listing what changed (or "no changes — operator cancelled").
- [ ] CHANGELOG.md `## Unreleased` entry added.
- [ ] `docs/configuration.md` updated with a "Use `/dark-factory:configure` to manage this file" pointer at the top.

## Verification

```bash
cd ~/Documents/workspaces/dark-factory
make precommit
```

**Runtime replay — greenfield:**

```bash
WORK_DIR=$(mktemp -d)
cd "$WORK_DIR"
git init
# In a Claude Code session in this dir:
#   /dark-factory:configure
# Walk through greenfield flow; pick workflow: direct
# Expected: .dark-factory.yaml created, no .bak (no prior file)
ls .dark-factory.yaml
dark-factory config   # must load cleanly
```

**Runtime replay — reconfigure:**

```bash
# In a project with an existing valid .dark-factory.yaml:
# /dark-factory:configure
# Pick "Toggle autoRelease"
# Expected: diff shown, .dark-factory.yaml.bak created, autoRelease flipped, validation passes
diff .dark-factory.yaml.bak .dark-factory.yaml   # exactly the autoRelease line differs
dark-factory config
```

**Runtime replay — auto-migrate (spec-073 legacy):**

```bash
# In a project, manually add to .dark-factory.yaml:
echo "autoReview: true" >> .dark-factory.yaml
echo "allowedReviewers:" >> .dark-factory.yaml
echo "  - bborbe" >> .dark-factory.yaml
dark-factory daemon   # must fail with the spec-073 friendly error
# /dark-factory:configure
# Expected: detects the error, proposes diff dropping the two fields, writes on approval
dark-factory daemon &  # must now start cleanly
DAEMON_PID=$!
sleep 5 && kill $DAEMON_PID
```

**Acceptable evidence for `verifying → completed`:**

| Evidence | Acceptable? |
|---|---|
| The three runtime replays above all produce the documented end state | Yes |
| `commands/configure.md` exists and Claude Code shows it in slash command list | Yes |
| Code inspection only ("the file exists") | No — must run the three replays |
| "All tests pass" alone | No |

## See also

- `~/Documents/workspaces/semantic-search/commands/configure.md` — the analog slash command (greenfield + add-instance + reconfigure flow). Not a literal copy: dark-factory is single-instance per project, so "add instance" maps to "switch workflow / change provider"; the rest carries over.
- `commands/init-project.md` — existing greenfield-only slash command. Either keep both (with `configure` calling into `init-project`) or absorb `init-project` as the `--new` flag of `configure`.
- `docs/configuration.md` — the field reference the slash command reads from (and the doc that should point back to the slash command).
- Spec 073 (`simplify-merge-gate-by-relying-on-mergestatestatus`) — established the friendly-error migration pattern that `/dark-factory:configure` automates.
- `pkg/config/config.go` `Defaults()`, yaml tags, `Validate()` — the source of truth for field metadata. The slash command should read this via the binary, not duplicate it.
