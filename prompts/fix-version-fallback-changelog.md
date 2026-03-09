<summary>
- Projects without git tags always got version v0.1.0, even when the changelog already tracked higher versions
- After this fix, projects without git tags get the correct next version based on their changelog history
- Projects that already use git tags are completely unaffected
- New tests verify the fallback behavior for all combinations (tags present, changelog only, neither)
</summary>

<objective>
Fix `getNextVersion` in `pkg/git/git.go` to fall back to parsing CHANGELOG.md for the latest version when no git tags exist, instead of defaulting to `v0.1.0`. This ensures repos that have CHANGELOG.md version entries but no corresponding git tags get correct version increments.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/git/git.go` — focus on `getNextVersion` (around line 209) and `GetNextVersion`.
Read `pkg/git/semver.go` — `ParseSemanticVersionNumber` is defined here (same package, no import needed).
Read `pkg/git/git_test.go` for existing version-related tests.

The bug: `getNextVersion` runs `git tag --list v*`, collects semver tags, and if none found returns `v0.1.0`. But some repos (like dark-factory-sandbox) have CHANGELOG.md with entries like `## v1.3.4` but no corresponding git tags. The function should parse CHANGELOG.md as fallback.
</context>

<requirements>
1. In `pkg/git/git.go`, modify `getNextVersion` (or add a helper): when `len(versions) == 0` after scanning git tags, attempt to parse `CHANGELOG.md` in the current working directory for the latest version.

2. Parse CHANGELOG.md by scanning for lines matching `^## v[0-9]+\.[0-9]+\.[0-9]+`. Use `ParseSemanticVersionNumber` on each match. Find the maximum version among all matches.

3. If CHANGELOG.md parsing finds valid versions, use the maximum as the base version and apply the bump. If CHANGELOG.md doesn't exist or has no valid versions, keep the existing `v0.1.0` fallback.

4. Use `os.Getwd()` + `CHANGELOG.md` as the path inside `getNextVersion`. No new exported function or signature change needed — keep it simple.

5. Add tests in `pkg/git/git_test.go` covering:
   - No git tags + CHANGELOG.md with versions → returns incremented version from CHANGELOG
   - No git tags + no CHANGELOG.md → returns v0.1.0 (existing behavior preserved)
   - No git tags + CHANGELOG.md with no version entries → returns v0.1.0
   - Git tags exist → uses tags (ignores CHANGELOG.md, existing behavior)

6. The CHANGELOG parsing regex should match the format `## vX.Y.Z` (same pattern used by the commit workflow).
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Do not change the behavior when git tags exist (tags remain primary source of truth)
- Follow existing code patterns in pkg/git/git.go
- Use `github.com/bborbe/errors` for error wrapping
</constraints>

<verification>
Run `make precommit` -- must pass.
Run `make test` -- all tests must pass including new fallback tests.
</verification>
