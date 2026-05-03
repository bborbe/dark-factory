---
status: draft
---

## Summary

- CLI `<id>` arguments for `dark-factory spec` and `dark-factory prompt` subcommands accept four equivalent formats: padded number (`063`), unpadded number (`63`), full basename (`063-foo-bar`), and basename with `.md` extension (`063-foo-bar.md`)
- Today only the full basename works — `063` and `63` both fail with "spec not found" / "prompt not found"
- When invoked outside a project root (no `.dark-factory.yaml` in cwd), CLI walks up to find one, or exits with a clear error — never silently returns an empty result
- Applies uniformly to all `spec` and `prompt` subcommands taking an `<id>` argument: `approve`, `unapprove`, `cancel`, `requeue`, `retry`, `complete`, `reject`, `show`
- Reduces friction in common workflows (typing IDs from memory, copy-pasting from `ls` output, copy-pasting from `spec list` output)

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

| Trigger | Current | Expected |
|---------|---------|----------|
| `spec complete 63` | "spec not found: 63" | Resolves to `063-*.md`, marks complete |
| `spec complete 063-foo.md` | "spec not found: 063-foo.md" | Strips `.md`, resolves to `063-foo.md`, marks complete |
| `spec complete 063-foo-bar` (full) | Works today | Unchanged |
| `spec complete 999` (no match) | "spec not found: 999" | "spec not found: 999" (unchanged) |
| `spec complete 1` when both `001-foo` and `010-bar` exist | (depends on padding logic) | Resolved by exact integer match — `1` only matches `001`, never `010`. If still ambiguous, list both |
| `spec list` from non-project cwd | `Specs: 0 total` (silent) | Walks up to find `.dark-factory.yaml`; if found, runs there; if not, error "not a dark-factory project" |
| `prompt approve 365` | "prompt not found: 365" | Resolves to `365-*.md`, queues |

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

# From a non-project cwd
cd /tmp && /tmp/new-dark-factory spec status
# Must error with "not a dark-factory project: ..." or walk up to ~/Documents/workspaces/dark-factory

# Ambiguity case (synthetic)
mkdir -p /tmp/df-ambig/specs/in-progress
touch /tmp/df-ambig/specs/in-progress/001-foo.md
touch /tmp/df-ambig/specs/in-progress/001-bar.md
cd /tmp/df-ambig && /tmp/new-dark-factory spec show 001
# Must error listing both candidates

make precommit  # in dark-factory repo — must exit 0
```

## Verification (scenario)

A new scenario `scenarios/0XX-cli-flexible-id-matching.md` walks all four format resolutions plus the ambiguity case + cwd walk-up against `/tmp/new-dark-factory`. Standard scenario layout (Setup → Action → Expected).

## Open questions

- Should the project-detection walk-up be opt-in via a flag, or default-on like git? (Lean default-on to match `git` muscle memory.)
- Should ambiguity errors print full file paths or just IDs? (Lean full paths so the user can grep.)
- Should `spec show` learn a `--format=json` output to make scripting against the resolver easier? (Out of scope — separate feature.)
