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

The rule: **before every `make install`, run all active scenarios against a freshly built binary**. No surface-scoped skipping unless the diff is genuinely empty.

### Expectations

| Aspect | Value |
|--------|-------|
| Wall time (full active gate) | ~30 min |
| YOLO cost (across LLM-driving scenarios) | ~$0.60 |
| LLM-driving scenarios | 001, 002, 003, 006, 019 (each ≈25–35 s, one ≈2.5 min) |
| Pure-CLI scenarios (no YOLO) | 011, 013 |
| Mixed (one YOLO + CLI assertions) | 010, 012 |

Plan to babysit. Scenarios that touch GitHub (002, 015, 018) leave real PRs/branches behind that need manual cleanup.

### Preflight (before starting the gate)

```bash
docker info >/dev/null 2>&1 || { echo "Docker daemon required"; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "gh CLI must be authed for PR scenarios"; exit 1; }
```

If a dark-factory daemon is running on dark-factory itself (the autoRelease loop), leave it running. Scenarios use sandbox copies and a per-run `maxContainers: 999`, which intentionally bypasses the system-wide cap so the gate is not throttled by the live daemon.

### Steps

```bash
# 1. Build a fresh binary (NOT the installed one)
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .

# 2. Confirm it built and reports the unreleased version
/tmp/new-dark-factory --version  # should say "dark-factory dev"

# 3. Walk every scenario whose frontmatter says `status: active`.
#    Skip `status: draft` (incomplete, may fail for unrelated reasons) and
#    `status: idea` (stubs, nothing to walk).
awk '/^status:/{print FILENAME": "$2; nextfile}' scenarios/*.md  # status per file

# 4. Run the helper-automated batch first (fastest signal — 42 sub-scenarios):
scenarios/helper/run-013-all.sh   # automates scenario 013 sub-scenarios A–M
# Expected: "Result: 42 passed, 0 failed"

# 5. Walk each remaining active markdown scenario by hand.
#    Each file's Setup → Action → Expected must all pass.
```

### Scenario ↔ runner map

| # | Status | Runner |
|---|--------|--------|
| 001 | active | manual (Setup uses `scenarios/helper/lib.sh::setup_sandbox_copy`) |
| 002 | active | manual — opens real PR on dark-factory-sandbox; close + delete branch after |
| 003 | active | manual |
| 006 | active | manual |
| 010 | active | manual |
| 011 | active | manual (pure CLI, no YOLO) |
| 012 | active | manual |
| 013 | active | **`scenarios/helper/run-013-all.sh`** (fully automated, 42 assertions) |
| 019 | active | manual (full spec lifecycle, ≈2.5 min) |
| 014–018 | draft | skip (exploratory, may fail for non-binary reasons) |
| 004, 005, 007–009, 020 | idea | skip (stubs, nothing to walk) |

If any active scenario fails: do **not** proceed to install. Fix the regression first, then rerun the gate.

### When a scenario fails — where to look first

| Symptom | Most likely surface |
|---------|---------------------|
| YOLO container fails to start, log shows `root/sudo privileges` | `pkg/runner/` UID remapping, or container image bump |
| Stream formatter crash, log truncated after `Starting headless session...` | `pkg/executor/streamfmt/`, container image |
| Git push fails, error swallowed | `pkg/git/` — verify stderr is captured into the wrapped error |
| Spec stuck in `prompted` after prompts done | `pkg/processor/` workflow_executor phase ordering or sweep ticker |
| Container name collision / wrong project segment | `pkg/generator/`, `pkg/processor/` spawn sites |
| Plugin fields stale after release | `.claude-plugin/plugin.json` + both `marketplace.json` version fields drifted |

### When the diff is empty

The one valid skip: nothing on the binary surface changed since the installed binary.

```bash
INSTALLED=$(dark-factory --version | awk '{print $NF}')
git diff "$INSTALLED"..HEAD --name-only | grep -E '\.(go|mod|sum)$|^Makefile$|^Dockerfile$'
# empty output → installed binary is byte-equivalent to /tmp/new-dark-factory → skip
```

This is the ONLY documented skip. Do not invent others ("docs-only changes shouldn't break anything") — surface mappings are fragile and have been wrong before. If `INSTALLED` is far behind HEAD (e.g., several auto-releases happened without an install), the diff will be large and the skip does NOT apply — walk the full gate.

## Version alignment check (release-time)

`scripts/check-versions.sh` enforces the locked model: top CHANGELOG entry == `plugin.json` `version` == `marketplace.json` `metadata.version` == `marketplace.json` `plugins[0].version`. Run via `make check-versions`, or via `make release-check` (which adds `make precommit` first).

```bash
make release-check          # full gate: precommit + check-versions
# or, just the version check:
make check-versions
# or directly:
bash scripts/check-versions.sh
```

The git tag is bound to CHANGELOG by `autoRelease` at release time, so it is not separately checked here.

**NOT wired into `make precommit`** — drift between binary CHANGELOG (advanced freely by `autoRelease`) and plugin JSONs (manually bumped when commands/agents change) is the expected state during development. Alignment is enforced at install time only, so plugin bumps catch up to the binary's CHANGELOG before downstream consumers pick up a new version.

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

## GitHub Release (manual — when to surface a milestone)

`autoRelease` creates a `vX.Y.Z` git tag after every approved prompt. Tags are sufficient for `go install github.com/bborbe/dark-factory@vX.Y.Z`, `git describe`, and any tag-aware consumer.

A **GitHub Release** is a separate, deliberate act — distinct from the tag. It adds release notes, an entry on the repo's Releases tab, an RSS/atom feed for subscribers, and optional binary assets. Create one **only after**:

1. All `scenarios/` pass against the current source tree.
2. Plugin JSONs are aligned (if `commands/`, `agents/`, `docs/`, or `skills/` changed since the last plugin release).
3. The `CHANGELOG.md` entry summarises what users should care about — not the internal commit log.

Skip the GitHub Release for internal refactors, pre-release/experimental work, or chains of small tags. It is fine to skip several auto-tags and cumulate them into a single milestone Release later.

How:

```bash
TAG=$(git describe --tags --abbrev=0)
gh release create "$TAG" \
  --target master \
  --title "$TAG" \
  --notes "$(awk "/^## $TAG/,/^## v/" CHANGELOG.md | head -n -1)"
```

Verify on github.com → Releases tab. The Release object can be edited (notes, draft state) without retagging.

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
5. **Run the version alignment check** as a gate, not a verify — `make check-versions` MUST exit 0 before commit. Easy to bump only 3 of the 4 places; this is the catch.
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
