# Release Process

Two release surfaces in this repo. They ship independently and follow different procedures.

| Surface | What ships | How |
|---------|------------|-----|
| **Binary** | The `dark-factory` Go binary (consumed by other projects) | Automatic per prompt — `autoRelease: true` + `CHANGELOG.md` bumps version, tags, pushes |
| **Plugin** | `commands/`, `agents/`, `docs/`, `skills/` (consumed by Claude Code) | Manual — bump JSON files + CHANGELOG, commit, push |

A single change can touch either surface or both. Plan the release accordingly.

## Binary Release

The `dark-factory` repo runs `dark-factory` on itself with `autoRelease: true` and a `CHANGELOG.md`. Every successful prompt that updates `## Unreleased` produces a new `vX.Y.Z` tag and pushes it. There is **no manual binary release step**. The release is the side-effect of completing a prompt.

### Pre-release gate (mandatory before producing a binary release)

`make precommit` is necessary but not sufficient. Unit tests + lint do **not** cover host↔container, host↔git remote, or config→runtime seams.

Before any prompt completes against this repo (and therefore before the auto-release fires), run all scenarios against a freshly built binary:

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
# Then run the scenarios in scenarios/ using /tmp/new-dark-factory <cmd>.
# NEVER use the bare `dark-factory` binary — that's the previously installed version
# and misses unshipped code.
```

### Skip rule (the only exception)

If no binary-relevant files changed since the installed version, the installed binary is byte-equivalent to what the next release would produce, so scenarios add no signal:

```bash
INSTALLED=$(dark-factory --version | awk '{print $2}')
git diff "$INSTALLED"..HEAD --name-only | grep -E '\.(go|mod|sum)$|^Makefile$|^Dockerfile$'
# empty output → skip scenarios; any hit → run them
```

Do **not** shortcut by intuition ("docs-only", "pipeline-only") — always run the diff.

### What autoRelease does

When `autoRelease: true` and `CHANGELOG.md` exists, after each successful prompt:
1. Stage all changes (including the agent's `## Unreleased` entry).
2. Determine bump (patch/minor) from the changelog content.
3. Rename `## Unreleased` → `## vX.Y.Z`.
4. Commit `release vX.Y.Z`.
5. Tag `vX.Y.Z`.
6. `git push` + `git push origin vX.Y.Z`.
7. Move the prompt file to `prompts/completed/` and push that commit too.

When `autoRelease: true` without `CHANGELOG.md`, only push happens — no version bump, no tag.

When `autoRelease: false`, commits stay local. See [configuration.md](configuration.md) and [workflows.md](workflows.md) for the full matrix.

### Verifying a release shipped

```bash
git fetch --tags
git describe --tags --abbrev=0     # latest tag
git log "$(git describe --tags --abbrev=0)"..HEAD --oneline   # any unpushed commits
```

After a successful prompt with `autoRelease: true` + CHANGELOG, both `git status` (clean) and `git rev-list @{u}..HEAD --count` (zero) should hold.

## Plugin Release

The plugin ships separately and requires a manual version bump. **Any change to `commands/`, `agents/`, `docs/`, or `skills/` requires a plugin version bump** — these files ship as part of the plugin, not via the Go binary tag.

### Procedure

1. **Pick the next version.** Increment minor from the latest `CHANGELOG.md` entry (e.g. `v0.103.0` → `v0.104.0`).
2. **Update all four files** — version string must be identical everywhere (no `v` prefix in JSON):

   | File | Field |
   |------|-------|
   | `CHANGELOG.md` | New `## vX.Y.Z` section at top with all changes (binary + plugin in the same section) |
   | `.claude-plugin/plugin.json` | `"version": "X.Y.Z"` |
   | `.claude-plugin/marketplace.json` | `"version": "X.Y.Z"` in **both** `metadata` and `plugins[0]` |

3. **Commit:** `release plugin vX.Y.Z: <summary>`.
4. **Push:** `git push`.

### Common mistakes

- Forgetting `.claude-plugin/` files — plugin stays at the old version while CHANGELOG advanced.
- Creating a separate "Plugin vX" changelog section — wrong, one version covers everything.
- Different versions across the three JSON fields — all must match exactly.
- Not including binary changes (fetch timeout, lock fix, etc.) in the changelog when they're uncommitted at release time.

## When the two surfaces interact

A prompt that touches `commands/`, `agents/`, `docs/`, or `skills/` will be auto-released by `autoRelease` as a binary tag (because `CHANGELOG.md` exists), but the **plugin** version in `.claude-plugin/*.json` does **not** auto-bump. After such a prompt completes, follow the [Plugin Release](#plugin-release) procedure manually.

A prompt that touches only Go code does not require the plugin procedure.

## See also

- [configuration.md](configuration.md) — `autoRelease` field semantics
- [workflows.md](workflows.md) — combinations table (workflow × pr × autoMerge × autoRelease)
- [scenario-writing.md](scenario-writing.md) — how to write scenarios that gate the release
- `CLAUDE.md` — concise rule for "scenarios before `make install`"
