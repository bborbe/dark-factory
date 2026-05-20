---
status: completed
approved: "2026-05-20T18:06:56Z"
generating: "2026-05-20T18:08:10Z"
prompted: "2026-05-20T18:13:09Z"
verifying: "2026-05-20T18:26:40Z"
completed: "2026-05-20T18:37:01Z"
branch: dark-factory/bug-get-next-version-ignores-changelog
---

## Summary

- `pkg/git/git.go:332` `getNextVersion` computes the next release version by bumping from the highest git tag (`git tag --list v*`). It only consults `CHANGELOG.md` as a fallback when **no valid semver tags exist at all**.
- If `CHANGELOG.md` already contains a `## vX.Y.Z` heading higher than the highest tag (an "orphan" entry — written by a prompt directly, never auto-tagged), the next auto-release bumps from the lower tag, producing a tag that is numerically below the orphan changelog entry.
- Concrete instance: vault-cli c86f20e wrote `## v0.65.0` into CHANGELOG without using `## Unreleased`, so autoRelease made no tag. The next prompt (c130211) used `## Unreleased` correctly; autoRelease bumped `v0.64.2 → v0.64.3`. CHANGELOG and plugin manifests were at `0.65.0+`; the new git tag was `v0.64.3`. Operator had to retag manually.
- Fix: `getNextVersion` should bump from `max(highest_tag, highest_changelog_versioned_heading)` — not just the highest tag.

## Problem

The release-version decision in `getNextVersion` reads only one source of truth (git tags), but the actual project state is captured across **two** sources of truth that can diverge:

1. **Git tags** — what was tagged + pushed by previous auto-releases.
2. **`CHANGELOG.md` `## vX.Y.Z` headings** — what's been written into the file. Most of the time these match, but a prompt can write a versioned heading directly (instead of `## Unreleased`), creating a CHANGELOG entry that was never tagged.

When the two diverge, the bump-from-highest-tag logic produces a version that:

- Is numerically below the highest CHANGELOG entry on disk (semver regression on the tag stream).
- Misaligns with plugin manifests if they were bumped to match the orphan CHANGELOG version (the marketplace alignment check fails or, worse, silently ships the wrong version).
- Forces the operator to manually retag, rewrite CHANGELOG, and clean up the orphan tag.

The CHANGELOG-fallback path (line ~358) already knows how to parse `## vX.Y.Z` headings — it just doesn't run when tags exist.

## Why this is a bug

Violates the semver monotonicity invariant on the project's tag stream: the on-disk `CHANGELOG.md` advertises a version higher than the git tag that follows it. Downstream plugin-version alignment (per dark-factory's `check-versions`) silently fails or ships the wrong version. Documented intent in `release-process.md` is that CHANGELOG and tags stay aligned; `getNextVersion` is the gatekeeper and currently only reads half the state.

## Goal

`getNextVersion` returns `bump(max(highest_tag, highest_changelog_versioned_heading))`. Orphan CHANGELOG entries are reconciled into the version stream instead of being silently overrun by a lower tag bump.

Behavioral end-state:

1. `highest_tag` comes from `git tag --list v*` (today's source).
2. `highest_changelog` comes from `latestVersionFromChangelog(ctx)` — read unconditionally, not only as fallback.
3. `base = max(highest_tag, highest_changelog)`.
4. `next = base.Bump<Minor|Patch>()` per the requested bump kind.
5. When `highest_changelog > highest_tag`, emit `slog.Warn` naming both versions so the divergence is visible to the operator.

## Expected vs Actual

| Aspect | Expected | Actual |
|--------|----------|--------|
| Base for next version | `max(highest_tag, highest_changelog_versioned_heading)` | `highest_tag` only (changelog ignored when any tag exists) |
| Tag after orphan changelog entry | `vX.(Y+1).1` (patch above the orphan) | `vX.Y.(Z+1)` (semver-regressed below the orphan) |
| Operator visibility on divergence | `slog.Warn` in `.dark-factory.log` naming both versions | Silent — divergence only surfaces later during manual cleanup |

Documented intent: `release-process.md` and `getNextVersion`'s own changelog-fallback path (`pkg/git/git.go:357-369`) both treat CHANGELOG as authoritative when tags are absent; the bug is that the same source is ignored when tags are present.

## Reproduction

dark-factory version: v0.162.0 (as of 2026-05-20).

Repo: any project with `autoRelease: true` and a `CHANGELOG.md`.

1. State: highest git tag is `vX.Y.Z` (e.g. `v0.64.2`); CHANGELOG top entry is also `## vX.Y.Z`.
2. Approve a prompt whose CHANGELOG edit writes `## vX.(Y+1).0` directly (e.g. `## v0.65.0`) instead of the conventional `## Unreleased`. AutoRelease sees a versioned heading already present, takes no rename/tag action.
3. State after step 2: git tag stuck at `vX.Y.Z`, CHANGELOG top is `## vX.(Y+1).0`, plugin manifests possibly at `X.(Y+1).0` too.
4. Approve a SECOND prompt that writes `## Unreleased` correctly.
5. AutoRelease runs `getNextVersion`. It walks `git tag --list v*`, finds highest tag is `vX.Y.Z`, applies the bump (patch by default), produces `vX.Y.(Z+1)`.
6. Observed: new tag is `vX.Y.(Z+1)` (e.g. `v0.64.3`). Highest CHANGELOG entry is now `## vX.(Y+1).0` above `## vX.Y.(Z+1)` — semantically inverted.

Concrete instance:

- vault-cli `c86f20e` "Next Status Task" — wrote `## v0.65.0` directly. No tag.
- vault-cli `c130211` "release v0.64.3" — autoRelease bumped from `v0.64.2` → `v0.64.3`. CHANGELOG has `## v0.64.3` above `## v0.65.0` after the rename. Plugin JSONs at `0.65.0`. Tag `v0.64.3` below the JSONs.
- Operator hand-fixed in `de39cca`: renamed CHANGELOG `## v0.64.3 → ## v0.65.1`, bumped JSONs to `0.65.1`, retagged.

## Root cause

`pkg/git/git.go:332` — `getNextVersion`:

```go
func getNextVersion(ctx context.Context, bump VersionBump) (string, error) {
    // 1. List git tags
    cmd := exec.CommandContext(ctx, "git", "tag", "--list", "v*")
    ...
    // 2. Parse semver
    for _, line := range lines {
        version, parseErr := ParseSemanticVersionNumber(ctx, line)
        if parseErr != nil { continue }
        versions = append(versions, version)
    }

    // 3. Fallback ONLY if zero valid tags exist
    if len(versions) == 0 {
        changelogVersion, err := latestVersionFromChangelog(ctx)
        ...
    }

    // 4. Bump from highest tag — ignores changelog entirely
    sort.Slice(versions, ...)
    return versions[last].BumpMinor() / BumpPatch(), nil
}
```

The CHANGELOG is only consulted when zero tags exist. When tags exist + CHANGELOG has a higher orphan entry, the orphan is ignored.

## Constraints

- `latestVersionFromChangelog` already exists and is used in the fallback path — no new parsing code needed.
- `ParseSemanticVersionNumber` and `SemanticVersionNumber.BumpMinor()` / `BumpPatch()` are already in the file — reuse, do not introduce new types.
- The "orphan changelog entry" case should produce a `slog.Warn` so the divergence is visible in `.dark-factory.log`. Do not silently reconcile — operators need to know the prompt-writing pattern broke.
- Behavior when `highest_tag == highest_changelog` (the normal case) must be byte-identical to today's behavior.
- No new external dependency, no new exported function.
- Existing tests must continue to pass. Add new tests for: orphan-changelog-above-tag, tag-above-orphan-changelog, equal, no-tags-with-changelog (existing path), no-tags-no-changelog (existing path).

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---------|-------------------|----------|-----------|
| CHANGELOG has `## vX.(Y+1).0`, highest tag `vX.Y.Z` | `getNextVersion` returns `vX.(Y+1).1` (patch bump from `## vX.(Y+1).0`). Warning logged: "changelog has orphan version vX.(Y+1).0 above highest tag vX.Y.Z; reconciling base from changelog". | None — auto-reconciled. | Warning visible in `.dark-factory.log`. |
| CHANGELOG and highest tag equal | No warning. Bump from the (equal) base as today. | None. | Behavior unchanged. |
| CHANGELOG missing or unreadable | Skip the changelog leg of `max`. Fall through to highest-tag bump (current behavior). | None. | No regression vs today. |
| Highest changelog entry is below highest tag (rare — manual rollback?) | Bump from highest tag. Ignore changelog. | None. | Behavior unchanged. |

## Acceptance Criteria

- [ ] `getNextVersion` reads both `git tag --list v*` AND `latestVersionFromChangelog(ctx)` unconditionally, then bumps `max(...)`. Evidence: `grep -n 'latestVersionFromChangelog' pkg/git/git.go` returns ≥1 hit OUTSIDE any `len(versions) == 0` branch.
- [ ] New test `orphan_changelog_patch`: tag `v0.64.2`, CHANGELOG top `## v0.65.0`, `bump=Patch` → returns `v0.65.1`. Evidence: `go test -run TestGetNextVersion/orphan_changelog_patch ./pkg/git/...` exits 0.
- [ ] New test `orphan_changelog_minor`: tag `v0.64.2`, CHANGELOG top `## v0.65.0`, `bump=Minor` → returns `v0.66.0`. Evidence: `go test -run TestGetNextVersion/orphan_changelog_minor ./pkg/git/...` exits 0.
- [ ] New test `tag_equals_changelog`: tag `v0.64.2`, CHANGELOG top `## v0.64.2`, `bump=Patch` → returns `v0.64.3` (byte-identical to today). Evidence: `go test -run TestGetNextVersion/tag_equals_changelog ./pkg/git/...` exits 0.
- [ ] New test `tag_above_changelog`: tag `v0.65.0`, CHANGELOG top `## v0.10.0`, `bump=Patch` → returns `v0.65.1`. (Guards against the lazy "always read changelog" implementation.) Evidence: `go test -run TestGetNextVersion/tag_above_changelog ./pkg/git/...` exits 0.
- [ ] New test `no_tags_with_changelog`: no tags, CHANGELOG top `## v0.10.0`, `bump=Patch` → returns `v0.10.1`. Evidence: `go test -run TestGetNextVersion/no_tags_with_changelog ./pkg/git/...` exits 0.
- [ ] New test `no_tags_no_changelog`: no tags, no CHANGELOG → returns `v0.1.0`. Evidence: `go test -run TestGetNextVersion/no_tags_no_changelog ./pkg/git/...` exits 0.
- [ ] When `highest_changelog > highest_tag`, `slog.Warn` emitted naming both versions. Evidence: test captures `slog` records and asserts one Warn entry whose message contains both the orphan version and the highest tag (e.g. `"orphan version v0.65.0 above highest tag v0.64.2"`).
- [ ] `make precommit` exits 0 (linters + existing + new tests).

## Verification

This is a `kind: bug` spec — verification MUST replay the Reproduction, not just rely on `make precommit`.

1. Build the fix: `go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .`
2. Create a sandbox project under `/tmp/` with: `autoRelease: true`, a `CHANGELOG.md` whose top heading is `## v0.65.0`, and a git tag `v0.64.2` (matching Reproduction step 1+2 state).
3. Approve a no-op prompt that updates `## Unreleased` (matching Reproduction step 4).
4. Run `/tmp/new-dark-factory run` against the sandbox.
5. **Expected (fix in place):** new git tag is `v0.65.1` (NOT `v0.64.3`); `.dark-factory.log` contains a `slog.Warn` line naming both `v0.65.0` and `v0.64.2`.
6. **Negative evidence:** `git tag --list 'v0.64.3'` returns empty — the buggy version is not produced.

If step 5 fails, the bug is not fixed regardless of unit-test results.

## Out of Scope

- Changing how prompts write CHANGELOG entries (the `## Unreleased` convention vs writing `## vX.Y.Z` directly) — that's a separate prompt-writing-guide fix.
- Adding a `dark-factory release reconcile` CLI command. Auto-reconciliation in `getNextVersion` is sufficient; an explicit tool is overkill.
- Migration of existing orphan tags (e.g. vault-cli's now-deleted `v0.64.3`) — operators handle one-off cleanup manually.
- Considering the highest version across other artifact sources (plugin.json, go.mod-style version files). CHANGELOG is the single project-level source of truth for version history; manifest files follow it.

## Do-Nothing Option

Tolerable but degrading. Every time a prompt writes a versioned CHANGELOG heading directly (which the prompt-writing guide permits in some cases, e.g. plugin-only release), the next autoRelease produces a semver-regressed tag. Operators retag manually. The cost compounds with autoRelease cadence — every divergence creates orphan tags and version-alignment failures downstream. Two such incidents have already occurred (the vault-cli one above; the operator suspects at least one other earlier). Fix once, save indefinitely.

## Related

- vault-cli incident: `c86f20e` → orphan `## v0.65.0`; `c130211` → orphan tag `v0.64.3`; `de39cca` → manual fix to `v0.65.1`.
- Source: `pkg/git/git.go:332` (`getNextVersion`), `pkg/git/git.go:357-369` (existing fallback that already parses CHANGELOG).
- Helper: `latestVersionFromChangelog(ctx)` — already exists in `pkg/git/git.go`.

## Verification Result

**Verified:** 2026-05-20T18:36:30Z (HEAD 11159fe)
**Binary:** /tmp/dark-factory-11159fe (built from HEAD)
**Scenario:** Sandbox replay in /tmp/df-083-sandbox (git init, tag v0.64.2, CHANGELOG `## v0.65.0`). Replay program imported `pkg/git` from HEAD, chdir'd into sandbox, called `GetNextVersion(ctx, PatchBump)`. Plus 6 Ginkgo contexts in `pkg/git/git_test.go` and `make precommit`.
**Evidence:**
- Sandbox `GetNextVersion(PatchBump)` returned `v0.65.1` (Reproduction expected); negative `git tag --list 'v0.64.3'` was empty.
- `.dark-factory.log` Warn line: `level=WARN msg="changelog has orphan version above highest tag; bumping from changelog to avoid semver regression" orphan_version=v0.65.0 highest_tag=v0.64.2`.
- `pkg/git/git.go:371` reads `latestVersionFromChangelog(ctx)` unconditionally outside any `len(versions)==0` branch.
- Ginkgo `Git GetNextVersion` contexts `orphan changelog above highest tag`, `orphan changelog above highest tag - minor bump`, `tag equals changelog version`, `tag above changelog version`, `no tags with changelog`, `no tags no changelog`: `Ran 6 of 241 Specs ... 6 Passed | 0 Failed`.
- `make precommit` exited 0 ("ready to commit").
**Verdict:** PASS
