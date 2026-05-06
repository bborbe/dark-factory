# Releasing Dark Factory

How to ship a new version of dark-factory itself. Mandatory reading before every `make install`.

> Looking for how `autoRelease` works in projects that **use** dark-factory?
> See [release-process.md](release-process.md). This doc is for releasing dark-factory the tool.

## Two surfaces, two version streams

Dark-factory ships two artifacts that version independently:

| Surface | Versioned by | Consumed by | Bumped how |
|---------|--------------|-------------|------------|
| **Binary** | git tag `vX.Y.Z` + matching `## vX.Y.Z` section in `CHANGELOG.md` | other Go projects via `go install github.com/bborbe/dark-factory@latest` | Auto-tagged by dark-factory's own daemon (`autoRelease: true`) when a prompt completes and updates `## Unreleased` |
| **Plugin** | `.claude-plugin/plugin.json` `version` + `.claude-plugin/marketplace.json` (`metadata.version` AND `plugins[0].version`) | Claude Code via the marketplace | Manual — operator bumps the three JSON fields |

A single change can touch one surface or both.

## The release gate (run BEFORE every `make install`)

The gate exists because `make precommit` does NOT cover host↔container, host↔git remote, or config→runtime seams. Unit tests pass while runtime behavior is broken — that has bitten the project repeatedly (cancellation race, `WaitAndMerge` field mismatch, autoReview unreachable path).

The rule: **before every `make install`, run all scenarios against a freshly built binary**. No surface-scoped skipping unless the diff is genuinely empty.

```bash
# 1. Build a fresh binary (NOT the installed one)
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .

# 2. Confirm it built and reports the unreleased version
/tmp/new-dark-factory --version  # should say "dark-factory dev"

# 3. Run all scenarios against it
scenarios/helper/run-all.sh   # TODO: this script does not yet exist — port markdown scenarios first
# Until run-all.sh exists, the operator must walk every scripted helper:
scenarios/helper/run-013-all.sh
# AND walk every markdown scenario manually:
ls scenarios/*.md  # 001 through 012+; each one's "Action" + "Expected" must pass
```

If any scenario fails: do **not** proceed to install. Fix the regression first, then rerun the gate.

### When the diff is empty

The one valid skip: nothing on the binary surface changed since the installed binary.

```bash
INSTALLED=$(dark-factory --version | awk '{print $NF}')
git diff "$INSTALLED"..HEAD --name-only | grep -E '\.(go|mod|sum)$|^Makefile$|^Dockerfile$'
# empty output → installed binary is byte-equivalent to /tmp/new-dark-factory → skip
```

This is the ONLY documented skip. Do not invent others ("docs-only changes shouldn't break anything") — surface mappings are fragile and have been wrong before.

## Version alignment check (run BEFORE every commit that bumps versions)

Whenever you bump any version (binary tag OR plugin JSON), all related fields must align. Run:

```bash
# Binary alignment: latest tag matches latest CHANGELOG section
LATEST_TAG=$(git tag -l | sort -V | tail -1)
LATEST_CHANGELOG=$(grep -m1 '^## v' CHANGELOG.md | sed 's/^## //')
test "$LATEST_TAG" = "$LATEST_CHANGELOG" && echo "✅ binary aligned" || echo "❌ tag=$LATEST_TAG changelog=$LATEST_CHANGELOG"

# Plugin alignment: three JSON fields must match each other
PLUGIN=$(jq -r .version .claude-plugin/plugin.json)
META=$(jq -r .metadata.version .claude-plugin/marketplace.json)
PLUGINS0=$(jq -r '.plugins[0].version' .claude-plugin/marketplace.json)
test "$PLUGIN" = "$META" -a "$META" = "$PLUGINS0" && echo "✅ plugin aligned ($PLUGIN)" || echo "❌ plugin=$PLUGIN meta=$META plugins[0]=$PLUGINS0"
```

(Future: ship `scripts/check-versions.sh` that runs both checks and exits non-zero on any mismatch.)

Plugin version is independent of the binary tag — the two streams are not required to match each other, only to be internally consistent within their surface.

## Binary release (automatic — but the operator owns the gate)

Dark-factory runs against itself as a daemon with `autoRelease: true`. Every successful prompt that touches `## Unreleased` triggers:

1. Stage all changes (including the agent's `## Unreleased` entry)
2. Determine bump (patch/minor) from changelog content
3. Rename `## Unreleased` → `## vX.Y.Z`
4. Commit `release vX.Y.Z`
5. Tag `vX.Y.Z`, push tag and commit
6. Move the prompt file to `prompts/completed/` and push that commit too

The operator's responsibility is to **run the release gate before approving any prompt** that may produce a binary change. Once the prompt is approved, the daemon ships whatever the agent produced — there is no second checkpoint.

To verify a release shipped:

```bash
git fetch --tags
git describe --tags --abbrev=0           # latest tag, e.g. v0.151.2
git log "$(git describe --tags --abbrev=0)"..HEAD --oneline   # any unpushed commits beyond it
```

After a successful auto-release, both `git status` (clean) and `git rev-list @{u}..HEAD --count` (zero) should hold.

## Plugin release (manual)

Whenever any of `commands/`, `agents/`, `docs/`, or `skills/` change, the plugin version must be bumped. The binary's `autoRelease` does **not** bump the plugin version — these JSON files are not part of the binary CHANGELOG-driven flow.

### When to bump

```bash
LAST_PLUGIN_TAG=$(git log --oneline -- .claude-plugin/ | head -1 | awk '{print $1}')
git diff "$LAST_PLUGIN_TAG"..HEAD --name-only -- commands/ agents/ docs/ skills/
# any output → plugin needs a bump
```

### Procedure

1. **Run the release gate** (above) if any binary surface also changed.
2. **Pick the next plugin version.** Increment minor from the latest `CHANGELOG.md` entry. Plugin and binary share the same CHANGELOG and the same monotonic version sequence, so if the binary just shipped `v0.151.2`, the next plugin release is `v0.152.0`.
3. **Update all three plugin fields** to the new version (no `v` prefix in JSON):
   - `.claude-plugin/plugin.json` `"version"`
   - `.claude-plugin/marketplace.json` `metadata.version`
   - `.claude-plugin/marketplace.json` `plugins[0].version`
4. **Add a `## vX.Y.Z` section** to `CHANGELOG.md` at the top, covering all changes since the previous CHANGELOG entry (binary AND plugin in the same section — there is one CHANGELOG, not two).
5. **Run the version alignment check** (above) — must report `✅ plugin aligned`.
6. **Commit:** `git commit -m "release plugin vX.Y.Z: <summary>"`.
7. **Push:** `git push`.

### Common plugin-release mistakes

- Forgetting `.claude-plugin/` files — CHANGELOG advances but plugin stays at old version. Operators see the new commands but `plugin --version` is stale.
- Creating a separate "Plugin vX" CHANGELOG section. Wrong — one CHANGELOG, one version sequence per release.
- Different version strings across the three JSON fields. The marketplace rejects mismatches silently and refuses to load the plugin.
- Bumping the plugin version BEFORE running the release gate. Binary surface changes that ship in the same release escape scenario coverage.

## Install (the moment the new version reaches consumers)

```bash
go install github.com/bborbe/dark-factory@latest
dark-factory --version  # should now match the latest tag
```

This is the step that bites another project if the gate was skipped. Other projects that run dark-factory will pick up the new binary the next time they `go install`. A regression in the new binary surfaces in their workflow, not yours.

The plugin's install is automatic via the marketplace once the bumped JSON files reach `master` — Claude Code re-checks the marketplace periodically.

## See also

- [release-process.md](release-process.md) — autoRelease behavior in projects that USE dark-factory (not how dark-factory itself ships)
- [scenario-writing.md](scenario-writing.md) — how to write the scenarios this gate runs
- [configuration.md](configuration.md) — `autoRelease` field semantics
- [workflows.md](workflows.md) — workflow × pr × autoMerge × autoRelease combinations
- `CLAUDE.md` "Before `make install`" — the concise rule that points back to this doc
