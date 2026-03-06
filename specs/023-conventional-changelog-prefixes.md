---
status: draft
---

# Conventional Changelog Prefixes for Version Bumping

## Problem

`determineBump()` uses keyword regex on free-form changelog text to decide between minor and patch bumps. This is fragile — it can match substrings unintentionally, misses valid feature words, and requires dark-factory to "understand" natural language. The YOLO container, which has full context of what it built, has no explicit way to signal intent.

## Goal

Replace keyword matching with conventional commit prefixes in `## Unreleased` changelog entries. YOLO writes prefixed entries; dark-factory reads the prefix — no guessing.

## Non-goals

- No change to commit message format (only changelog entries)
- No major-bump automation (`feat!:` / breaking changes handled manually)
- No external tooling (no commitlint, no semantic-release)

## Desired Behavior

### Changelog entry format

```markdown
## Unreleased

- feat: Add SpecWatcher to monitor specs/ for approved status changes
- fix: Remove stale Docker container before starting a new executor run
- refactor: Extract worktree cleanup to reduce cognitive complexity
- test: Improve processor test coverage to ≥80%
- docs: Add changelog writing guide to YOLO docs
- chore: Update github.com/bborbe/errors to v1.5.2
```

### Prefix → version bump mapping

| Prefix | Bump |
|--------|------|
| `feat:` | Minor (`vX.Y+1.0`) |
| `fix:`, `refactor:`, `test:`, `docs:`, `chore:`, `perf:`, `style:` | Patch (`vX.Y.Z+1`) |

### `determineBump()` logic

1. Read `CHANGELOG.md`
2. Find `## Unreleased` section
3. If any entry starts with `- feat:` → `MinorBump`
4. Otherwise → `PatchBump`
5. If no `CHANGELOG.md` or no `## Unreleased` → `PatchBump`

### YOLO container instructions

`~/.claude-yolo/CLAUDE.md` and `docs/changelog-guide.md` are updated to require conventional prefixes on every `## Unreleased` entry.

## Constraints

- Existing changelog entries (already versioned) are not modified
- Backward compatible: entries without a prefix → treated as patch (no crash)
- `make precommit` must pass

## Failure Modes

| Trigger | Expected behavior |
|---------|------------------|
| Entry has no prefix | Treated as patch bump |
| `## Unreleased` section missing | Default to patch bump |
| `CHANGELOG.md` missing | Default to patch bump |

## Acceptance Criteria

- [ ] `determineBump()` returns `MinorBump` when Unreleased contains `- feat:` entry
- [ ] `determineBump()` returns `PatchBump` for `fix:`, `refactor:`, `chore:`, `test:`, `docs:`
- [ ] `determineBump()` returns `PatchBump` when no prefix present (backward compat)
- [ ] Old keyword matching removed
- [ ] `~/.claude-yolo/CLAUDE.md` updated: require conventional prefix on every changelog entry
- [ ] `docs/changelog-guide.md` updated: document prefix table and examples
- [ ] All existing tests updated to use prefixed entries
- [ ] `make precommit` passes

## Verification

```
# In a repo with CHANGELOG.md containing:
# ## Unreleased
# - feat: Add SpecWatcher
grep "feat:" CHANGELOG.md   # entry present
dark-factory status          # next bump would be minor
```

## Do-Nothing Option

Keep keyword matching. Entries like `"additional context"` silently trigger minor bumps; feature entries using `"implement"` but not `"add"` stay on patch. Grows more fragile as YOLO writes more varied prose.
