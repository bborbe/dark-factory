---
status: completed
spec: [083-bug-get-next-version-ignores-changelog]
summary: Fixed getNextVersion to bump from max(highest_tag, highest_changelog) instead of highest tag alone, preventing semver regression when a CHANGELOG vX.Y.Z heading is written above the highest git tag; added six new Ginkgo tests covering all cases including slog.Warn capture for the orphan case; updated existing test that encoded the old (buggy) behavior.
container: dark-factory-exec-397-fix-get-next-version-ignores-changelog
dark-factory-version: v0.162.0
created: "2026-05-20T18:15:00Z"
queued: "2026-05-20T18:21:19Z"
started: "2026-05-20T18:21:20Z"
completed: "2026-05-20T18:26:39Z"
branch: dark-factory/bug-get-next-version-ignores-changelog
---

<summary>
- `getNextVersion` now reads both `git tag --list v*` AND `latestVersionFromChangelog` unconditionally, then bumps from `max(highest_tag, highest_changelog)`
- When a CHANGELOG `## vX.Y.Z` heading is higher than the highest git tag (an "orphan" entry), `getNextVersion` reconciles by bumping from the changelog version instead of regressing below it
- A `slog.Warn` is emitted when the changelog is ahead of the highest tag, naming both versions so the divergence is visible in `.dark-factory.log`
- When tag and changelog versions are equal (the normal case), behavior is byte-identical to today — no warning, same version produced
- When the highest tag is above the highest changelog entry, the tag wins — unchanged behavior
- When no tags exist but CHANGELOG has version entries, bumps from the changelog — existing fallback behavior preserved
- When neither tags nor changelog versions exist, returns `v0.1.0` — existing default preserved
- Six new Ginkgo tests cover all cases including a `slog.Warn` capture assertion for the orphan case
- No new exported functions, no new external dependencies
</summary>

<objective>
Fix `getNextVersion` in `pkg/git/git.go` to bump from `max(highest_tag, highest_changelog_versioned_heading)` instead of highest tag alone. This prevents the semver regression that occurs when a CHANGELOG `## vX.Y.Z` heading exists above the highest git tag (an "orphan" entry written directly by a prompt instead of via `## Unreleased`).
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` for which test types to write for each code change.

Files to read in full before editing:
- `pkg/git/git.go` — focus on `getNextVersion` (~line 332), `latestVersionFromChangelog` (~line 400), and `GetNextVersion` (~line 327). The fix is entirely within `getNextVersion`.
- `pkg/git/git_test.go` — external test package `git_test`. Contains `Describe("GetNextVersion", ...)` starting at line 286 with a full BeforeEach/AfterEach scaffold (temp dir, `git init`, `git config`, `os.Chdir`). **Extend this block** with new Contexts — reuse the scaffold, do NOT add a separate Describe in `git_internal_test.go`.
- `pkg/git/git_suite_test.go` — Ginkgo suite bootstrap. Suite function is named `TestSuite`.
- `pkg/git/semver.go` — confirms `BumpPatch()`, `BumpMinor()`, `Less()`, and `String()` on `SemanticVersionNumber` (all value receivers).
- `pkg/git/git_internal_test.go` — internal test package `git`. Read for reference only — new tests go to `git_test.go`.

Relevant background (spec 083):
- Concrete incident: vault-cli commit `c86f20e` wrote `## v0.65.0` directly into CHANGELOG without using `## Unreleased`. The next autoRelease bumped from highest tag `v0.64.2` → produced `v0.64.3`. CHANGELOG then had `## v0.64.3` above `## v0.65.0` — semantically inverted. Operator had to hand-fix via `de39cca`.
- `latestVersionFromChangelog(ctx)` already exists at ~line 400 and parses `## vX.Y.Z` headings — reuse it, do not write new parsing code.
- `slog` is already imported in `pkg/git/git.go`.
- `SemanticVersionNumber.Less(other)` returns true if receiver < other; false when equal or greater.
</context>

<requirements>

## 1. Fix `getNextVersion` in `pkg/git/git.go`

The section to replace starts at the `// If no valid semver tags exist, fall back to CHANGELOG.md` comment (~line 357) and ends at the final `return nextVersion.String(), nil` (~line 394). The git-tag-listing section above that populates `versions []SemanticVersionNumber` is **not changed**.

Replace that entire section with:

```go
// Find the maximum version among git tags (if any).
var maxTagVersion *SemanticVersionNumber
if len(versions) > 0 {
    mv := versions[0]
    for _, v := range versions[1:] {
        if mv.Less(v) {
            mv = v
        }
    }
    maxTagVersion = &mv
}

// Read CHANGELOG.md unconditionally. latestVersionFromChangelog returns nil when
// the file is missing or contains no valid version headings; that is expected.
changelogVersion, _ := latestVersionFromChangelog(ctx)

// Compute base version: max(highest_tag, highest_changelog).
var base SemanticVersionNumber
switch {
case maxTagVersion == nil && changelogVersion == nil:
    return "v0.1.0", nil
case maxTagVersion == nil:
    base = *changelogVersion
case changelogVersion == nil:
    base = *maxTagVersion
case maxTagVersion.Less(*changelogVersion):
    // CHANGELOG has an orphan version above the highest git tag.
    // Warn so the operator can see the divergence in .dark-factory.log,
    // then reconcile by bumping from the changelog version.
    slog.Warn("changelog has orphan version above highest tag; bumping from changelog to avoid semver regression",
        "orphan_version", changelogVersion.String(),
        "highest_tag", maxTagVersion.String(),
    )
    base = *changelogVersion
default:
    base = *maxTagVersion
}

// Apply the appropriate bump.
var nextVersion SemanticVersionNumber
switch bump {
case MinorBump:
    nextVersion = base.BumpMinor()
case PatchBump:
    nextVersion = base.BumpPatch()
default:
    nextVersion = base.BumpPatch()
}
return nextVersion.String(), nil
```

**FREEZE everything outside `getNextVersion`** — do not touch any other function, struct, or file outside this one function body.

## 2. Add six tests by extending `Describe("GetNextVersion", ...)` in `pkg/git/git_test.go`

The existing block at line 286 (package `git_test`) already provides:
- `BeforeEach`: temp dir, `git init`, `git config user.email/user.name`, `os.Chdir(tempDir)` — DO NOT duplicate.
- `AfterEach`: restore `originalDir` and `os.RemoveAll(tempDir)` — DO NOT duplicate.
- Existing Contexts call the exported `git.GetNextVersion(ctx, git.PatchBump)`.

**Add new `Context` blocks inside the existing `Describe("GetNextVersion", ...)`**, after the existing Contexts. Each test must:
1. Create an initial commit using the existing pattern (`os.WriteFile` + `git add` + `git commit`).
2. Optionally add tags via `exec.Command("git", "tag", "<version>")` with `cmd.Dir = tempDir`.
3. Optionally write `CHANGELOG.md` to `tempDir` via `os.WriteFile`.
4. Call `git.GetNextVersion(ctx, bump)` and assert via Gomega.

Use the exported `git.GetNextVersion` — the same wrapper the existing Contexts use. The internal `getNextVersion` is the implementation; behavior is verified through the public surface.

### Test cases

**test 1: orphan_changelog_patch** (mark this `Context` with `Serial` because it swaps `slog.SetDefault` — see [Ginkgo Serial](https://onsi.github.io/ginkgo/#serial-specs))

Setup: tag `v0.64.2`, CHANGELOG top heading `## v0.65.0`, bump = `git.PatchBump`.

Before calling `git.GetNextVersion`, capture slog output (do this in the `It` body, not BeforeEach, to keep the scope local):
```go
var logBuf bytes.Buffer
handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn})
origDefault := slog.Default()
slog.SetDefault(slog.New(handler))
defer slog.SetDefault(origDefault)
```

Context decorator: `Context("orphan changelog above highest tag", Serial, func() { ... })`.

Expected:
- `git.GetNextVersion(ctx, git.PatchBump)` returns `"v0.65.1"`, no error
- `logBuf.String()` contains `"v0.65.0"` (orphan version)
- `logBuf.String()` contains `"v0.64.2"` (highest tag)

**test 2: orphan_changelog_minor**

Setup: tag `v0.64.2`, CHANGELOG top heading `## v0.65.0`, bump = `MinorBump`.

Expected: `git.GetNextVersion(ctx, git.MinorBump)` returns `"v0.66.0"`, no error. (No slog assertion needed in this test.)

**test 3: tag_equals_changelog**

Setup: tag `v0.64.2`, CHANGELOG top heading `## v0.64.2`, bump = `PatchBump`.

Expected: returns `"v0.64.3"` — byte-identical to today's behavior. No warning (do not assert slog here).

**test 4: tag_above_changelog**

Setup: tag `v0.65.0`, CHANGELOG top heading `## v0.10.0`, bump = `PatchBump`.

Expected: returns `"v0.65.1"` (tag wins, changelog below tag is ignored). Guards against an implementation that always favors changelog.

**test 5: no_tags_with_changelog**

Setup: no git tags (only `git init` + empty commit), CHANGELOG top heading `## v0.10.0`, bump = `PatchBump`.

Expected: returns `"v0.10.1"` — existing fallback path preserved.

**test 6: no_tags_no_changelog**

Setup: no git tags, no `CHANGELOG.md`, bump = `PatchBump`.

Expected: returns `"v0.1.0"` — existing default preserved.

### Required imports for the new test block

Add to the existing import block in `pkg/git/git_test.go` (package `git_test`):
- `"bytes"` — for `bytes.Buffer` in the slog capture
- `"log/slog"` — for `slog.NewTextHandler`, `slog.HandlerOptions`, `slog.Default`, `slog.SetDefault`

`"os/exec"`, `"context"`, `"os"`, `"path/filepath"`, and the Ginkgo/Gomega dot imports are already present.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- **FREEZE all code outside `getNextVersion`** — no other functions, types, or files change
- No new exported functions (`getNextVersion` stays unexported)
- No new external dependencies
- `latestVersionFromChangelog` must be called unconditionally — NOT inside `if len(versions) == 0`. The spec AC explicitly verifies: `grep -n 'latestVersionFromChangelog' pkg/git/git.go` must return ≥1 hit outside any `len(versions) == 0` branch
- When `highest_changelog > highest_tag`, emit `slog.Warn` naming both the orphan version and the highest tag — the captured text handler output must contain both version strings
- When tag and changelog are equal, behavior must be byte-identical to today — bump from the (equal) tag, no warning
- When tag is above changelog, bump from tag (unchanged behavior)
- When no tags and no changelog, return `"v0.1.0"` (unchanged behavior)
- Existing tests must all pass
- Wrap any new errors with `errors.Wrapf` from `github.com/bborbe/errors` — but no new error paths are expected in this change
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional checks:
1. `grep -n 'latestVersionFromChangelog' pkg/git/git.go` — must show ≥1 hit OUTSIDE any `len(versions) == 0` conditional block (the AC for spec 083).
2. `go test ./pkg/git/... -v -ginkgo.focus="GetNextVersion"` — all GetNextVersion tests (existing + six new) must pass.
3. `grep -n 'slog.Warn' pkg/git/git.go` — exactly one new occurrence in `getNextVersion`, for the orphan-changelog case.
4. `grep -c 'orphan changelog above highest tag' pkg/git/git_test.go` — returns ≥1 (the new orphan Context exists).
5. `grep -c 'tag_above_changelog\|tag above changelog' pkg/git/git_test.go` — returns ≥1 (the guard test against the lazy "always read changelog" implementation).
</verification>
