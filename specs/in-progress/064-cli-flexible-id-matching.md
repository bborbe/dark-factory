---
status: prompted
approved: "2026-05-03T12:22:37Z"
generating: "2026-05-03T12:22:47Z"
prompted: "2026-05-03T12:34:19Z"
branch: dark-factory/cli-flexible-id-matching
---

## Summary

- CLI `<id>` arguments for `dark-factory spec` and `dark-factory prompt` subcommands accept four equivalent formats: padded number (`063`), unpadded number (`63`), full basename (`063-foo-bar`), and basename with `.md` extension (`063-foo-bar.md`)
- Today only the full basename works — `063` and `63` both fail with "spec not found" / "prompt not found"
- When invoked outside a project root (no `.dark-factory.yaml` in cwd), CLI walks up to find one, or exits with a clear error — never silently returns an empty result
- Applies uniformly to all `spec` and `prompt` subcommands taking an `<id>` argument: `approve`, `unapprove`, `cancel`, `requeue`, `retry`, `complete`, `reject`, `show`
- Reduces friction in common workflows (typing IDs from memory, copy-pasting from `ls` output, copy-pasting from `spec list` output)
- Both behaviors (flexible ID resolution + project-detection walk-up) ship together because they share the same code path: every `<id>` resolution must first locate the project root, then resolve the ID against it. Splitting would duplicate the project-root logic across two specs.

## Problem

Today `dark-factory spec complete <id>` and `dark-factory prompt approve <id>` accept only the full basename without `.md` extension. Three other formats users naturally try all fail:

```
$ dark-factory spec complete 063
error: spec not found: 063

$ dark-factory spec complete 63
error: spec not found: 63

$ dark-factory spec complete 063-bug-autorelease-overrides-pr-workflow.md
error: spec not found: 063-bug-autorelease-overrides-pr-workflow.md

$ dark-factory spec complete 063-bug-autorelease-overrides-pr-workflow
completed: 063-bug-autorelease-overrides-pr-workflow.md  ← only this works
```

Each failure forces the user to look up the full slug. The number alone is unique by construction (the daemon assigns sequential numbers on approval), so `063` should be sufficient.

A second papercut: when the working directory has no `.dark-factory.yaml` (e.g. running from a temp test repo), `spec list` and `spec status` silently return zero results instead of erroring. This makes troubleshooting hard — the user thinks the daemon lost state when really they're in the wrong directory.

```
$ cd /private/tmp/test-bug-config   # no .dark-factory.yaml here
$ dark-factory spec status
Specs: 0 total (0 idea, ...)        ← misleading; should error or walk up
```

### Reproduction

1. In a dark-factory project, approve a spec — let dark-factory assign a number (e.g. `063`)
2. Run `dark-factory spec complete 063` — fails with "spec not found"
3. Run `dark-factory spec complete 63` — fails with "spec not found"
4. Run `dark-factory spec complete 063-<full-slug>.md` — fails with "spec not found"
5. Only `dark-factory spec complete 063-<full-slug>` (no extension) works

Same for `dark-factory prompt approve|cancel|requeue|retry`.

For the cwd issue:
1. `cd /tmp/somewhere-without-dark-factory-yaml`
2. `dark-factory spec status` → returns `Specs: 0 total` (silent, misleading)

## Goal

1. **Resolve `<id>` flexibly:** the CLI accepts four formats for any `<id>` arg, returning a unique match or a clear ambiguity error.
2. **Detect project root:** when run outside a project root, walk up to find `.dark-factory.yaml`, or exit with a clear "not a dark-factory project" error. Never silently report empty state.
3. **Uniform behavior:** spec and prompt subcommands use the same resolver — no per-command divergence.

## Non-goals

- Substring matching (e.g. `autorelease` matching `063-bug-autorelease-overrides-pr-workflow`) — ambiguity handling is complex; the number alone is already a unique short ID
- Tab completion / shell integration — out of scope; can be a follow-up spec
- Fuzzy matching or typo correction
- Renaming the canonical spec/prompt filename format

## Desired Behavior

1. `dark-factory spec complete 063` resolves to `specs/in-progress/063-*.md` (or `specs/<status>/063-*.md` for non-`in-progress` lookups)
2. `dark-factory spec complete 63` (unpadded) resolves the same way as `063`
3. `dark-factory spec complete 063-bug-foo` (full basename, no extension) resolves the same way
4. `dark-factory spec complete 063-bug-foo.md` (full basename with extension) resolves the same way
5. Same four formats work for `dark-factory prompt approve`, `prompt cancel`, `prompt requeue`, `prompt retry`, `prompt complete` — all subcommands taking `<id>`
6. When the resolver finds zero matches, error: `spec not found: <input>` (current behavior, unchanged)
7. When the resolver finds two or more matches (ambiguous), error: `ambiguous spec id <input>: matches <list-of-files>` and list candidates
8. When no `.dark-factory.yaml` is in cwd, the CLI walks up the directory tree to find one (like `git`). If found, run from that root. If not found anywhere up to `$HOME`, exit with: `not a dark-factory project: no .dark-factory.yaml in <cwd> or any parent directory`
9. The current "silent zero" behavior on `spec list` / `spec status` / `prompt list` is replaced by the project-detection error from #8

## Constraints

- Must not break the current full-basename-without-extension format — that continues to work
- Must not slow down the common case (full match) measurably — resolver order: try as integer (parse) → try as basename → try with `.md` stripped
- Resolver applies to specs in any directory (`specs/`, `specs/ideas/`, `specs/in-progress/`, `specs/completed/`, `specs/rejected/`) — same for prompts
- The number-only match must be unique across all status directories (already true by construction — numbers are globally sequential)
- Project-detection walk-up stops at `$HOME` to avoid surprises in nested mount setups
- Error messages must be actionable — list what was searched and what to try next

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `spec complete 63` (unpadded number) | Resolves to `063-*.md`, marks complete | n/a (success) |
| `spec complete 063-foo.md` (with extension) | Strips `.md`, resolves, marks complete | n/a (success) |
| `spec complete 063-foo-bar` (full basename) | Resolves, marks complete (unchanged from today) | n/a (success) |
| `spec complete 999` (no match) | Error: `spec not found: 999`, exit non-zero | Run `dark-factory spec list` to see valid IDs |
| `spec complete 1` when `001-foo` and `010-bar` both exist | Exact integer match: `1` resolves to `001` only, never `010` | n/a (resolved unambiguously) |
| Two specs share a number (impossible by daemon invariant, but defensive) | Error: `ambiguous spec id <input>: matches <list-of-paths>` | Use full basename to disambiguate, or fix the duplicate-number invariant violation |
| `spec list` from non-project cwd, with `.dark-factory.yaml` in ancestor | Walks up, runs against ancestor's project root | n/a (success) |
| `spec list` from non-project cwd, no `.dark-factory.yaml` in any ancestor up to `$HOME` | Error: `not a dark-factory project: no .dark-factory.yaml in <cwd> or any parent directory` | `cd` to a project root or run `dark-factory init` |
| `prompt approve 365` (unpadded or padded number) | Resolves to `365-*.md`, queues | n/a (success) |

## Do-Nothing Option

Cost: ongoing UX friction. Every CLI invocation requires copy-pasting the full slug. New users hit "spec not found: 063" and have to look up the full filename. Misleading "0 specs" output sends people on wild goose chases when they're just in the wrong directory.

Not catastrophic — workaround exists (use the full slug). Pure quality-of-life improvement.

## Acceptance Criteria

- [ ] `dark-factory spec complete <padded-number>` (e.g. `063`) resolves and completes the spec
- [ ] `dark-factory spec complete <unpadded-number>` (e.g. `63`) resolves and completes the spec
- [ ] `dark-factory spec complete <full-basename>` (current behavior) continues to work
- [ ] `dark-factory spec complete <full-basename>.md` resolves and completes the spec
- [ ] Same four formats work for `dark-factory prompt approve`, `prompt cancel`, `prompt requeue`, `prompt retry`
- [ ] Ambiguous matches print all candidates and exit non-zero
- [ ] Running from a non-project cwd walks up to find `.dark-factory.yaml`, or exits with "not a dark-factory project" — no silent empty results
- [ ] Project-detection walk stops at `$HOME` (verified by setting cwd to `/tmp/foo` with no `.dark-factory.yaml` anywhere up to `/`)
- [ ] Unit tests cover all four resolution formats + ambiguity case + zero-match case
- [ ] Integration test covers project-detection walk-up
- [ ] CLI help text for each affected subcommand mentions the accepted formats

## Verification

```bash
# Build fresh binary
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .

# Inside a real project
cd ~/Documents/workspaces/dark-factory
/tmp/new-dark-factory spec list      # confirm at least one spec exists in in-progress

# Pick the latest spec id and try all four formats (it's idempotent for show; use show)
/tmp/new-dark-factory spec show 063
/tmp/new-dark-factory spec show 63
/tmp/new-dark-factory spec show 063-foo-bar
/tmp/new-dark-factory spec show 063-foo-bar.md

# All four must succeed and print the same spec

# From a non-project cwd, no ancestor has .dark-factory.yaml
cd /tmp && /tmp/new-dark-factory spec status
# Must error with "not a dark-factory project: no .dark-factory.yaml in /tmp or any parent directory"

# Walk-up boundary: $HOME stop
# Place a .dark-factory.yaml above $HOME (e.g. at /), confirm walk does NOT find it
# (Skip if running as non-root — this verifies the walk doesn't escape $HOME)
cd /tmp/some/deep/path 2>/dev/null || mkdir -p /tmp/some/deep/path && cd /tmp/some/deep/path
/tmp/new-dark-factory spec status
# Must error — walk-up stops at $HOME, never reaches /

# Walk-up positive case: cd into a subdirectory of a real project
cd ~/Documents/workspaces/dark-factory/pkg/config
/tmp/new-dark-factory spec status
# Must succeed and show specs from ~/Documents/workspaces/dark-factory

# Ambiguity case (synthetic — should never happen in real use; defensive test)
mkdir -p /tmp/df-ambig/specs/in-progress
touch /tmp/df-ambig/specs/in-progress/001-foo.md
touch /tmp/df-ambig/specs/in-progress/001-bar.md
echo 'workflow: direct' > /tmp/df-ambig/.dark-factory.yaml
cd /tmp/df-ambig && /tmp/new-dark-factory spec show 001
# Must error listing both candidates with full paths

# CLI help text mentions accepted formats
/tmp/new-dark-factory spec complete --help 2>&1 | grep -E "padded|unpadded|number|basename|formats"
# Must show non-empty match — help text documents the 4 formats

make precommit  # in dark-factory repo — must exit 0
```

## Verification (scenario)

A new scenario `scenarios/0XX-cli-flexible-id-matching.md` walks all four format resolutions plus the ambiguity case + cwd walk-up against `/tmp/new-dark-factory`. Standard scenario layout (Setup → Action → Expected).

## Resolved decisions

The following architectural choices are resolved at draft time, not deferred to prompt-time:

- **Walk-up is default-on** (matches `git` muscle memory). No opt-in flag. To disable, the user must `cd` to a directory that has no `.dark-factory.yaml` ancestor.
- **Ambiguity errors print full file paths**, one per line, so users can grep / inspect. Exit code is non-zero.
- **`spec show --format=json`** is explicitly out of scope — separate feature spec if needed.

## Open questions

None at draft level. Implementation choices (data structures, exact regex for number matching, etc.) deferred to prompt-time per `spec-writing.md` (specs stay behavioral).
