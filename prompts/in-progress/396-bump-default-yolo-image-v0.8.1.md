---
status: committing
summary: Bumped DefaultContainerImage constant and all documentation references from claude-yolo:v0.7.0 to claude-yolo:v0.8.1, and added CHANGELOG Unreleased entry.
container: dark-factory-exec-396-bump-default-yolo-image-v0-8-1
dark-factory-version: v0.162.0
created: "2026-05-19T22:35:00Z"
queued: "2026-05-19T20:01:39Z"
started: "2026-05-19T20:12:24Z"
---

<summary>
- Bump the default YOLO container image from `docker.io/bborbe/claude-yolo:v0.7.0` to `docker.io/bborbe/claude-yolo:v0.8.1`
- v0.8.1 adds ANTHROPIC_MODEL-aware model resolution in the entrypoint (required for routing to non-Anthropic providers like MiniMax) plus a one-shot prompt-file permission fix
- Update all documentation references to the previous default tag so README, configuration guide, init-project guide, and yolo-container-setup guide all show the new tag
- No behavioral changes beyond the version bump
</summary>

<objective>
Make `docker.io/bborbe/claude-yolo:v0.8.1` the project-wide default container image. All hardcoded references to `v0.7.0` in code, docs, and the default-config table become `v0.8.1`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/dod.md` for the project's definition of done.

Files to read in full before editing:
- `pkg/const.go` — `DefaultContainerImage` constant (line 9)
- `README.md` — search for `claude-yolo:v0.7.0` (two occurrences expected)
- `docs/configuration.md` — search for `claude-yolo:v0.7.0` (three occurrences expected)
- `docs/init-project.md` — search for `claude-yolo:v0.7.0` (one occurrence)
- `docs/yolo-container-setup.md` — search for `claude-yolo:v0.7.0` (one occurrence)
</context>

<requirements>

## 1. Update the `DefaultContainerImage` constant

Edit `pkg/const.go`. Change line 9 from:

```go
const DefaultContainerImage = "docker.io/bborbe/claude-yolo:v0.7.0"
```

to:

```go
const DefaultContainerImage = "docker.io/bborbe/claude-yolo:v0.8.1"
```

## 2. Update documentation references

In each of the following files, replace every occurrence of the literal string `claude-yolo:v0.7.0` with `claude-yolo:v0.8.1`. Do not change any other version reference — only the explicit current default.

- `README.md`
- `docs/configuration.md`
- `docs/init-project.md`
- `docs/yolo-container-setup.md`

## 3. Do NOT change test fixtures or historical artifacts

The following files reference older tags in test cases or historical specs — leave them untouched:

- `main_internal_test.go`
- `pkg/config/config_loader_test.go`
- `specs/completed/*.md`
- `CHANGELOG.md` historical entries

## 4. Add CHANGELOG entry

If `CHANGELOG.md` does not already have an `## Unreleased` section above the topmost released version header (currently `## v0.163.2`), create one. Then add the bullet under it:

```
- Bump default container image to claude-yolo:v0.8.1 (ANTHROPIC_MODEL-aware model resolution for alt-provider routing + one-shot prompt-file permission fix)
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT change any test fixtures, historical changelog entries, or completed spec files.
- Do NOT alter the `pkg.DefaultContainerImage` constant's name, type, or godoc comment — only its string value.
- Do NOT introduce a new helper, function, or abstraction; this is a literal value bump.
- After the edit, `grep -rn 'claude-yolo:v0.7.0' README.md docs/ pkg/const.go` should return zero lines.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional checks:
1. `grep -n 'v0.8.1' pkg/const.go` — returns exactly one line (the constant value).
2. `grep -rn 'claude-yolo:v0.7.0' README.md docs/` — returns zero lines.
3. `grep -rn 'claude-yolo:v0.8.1' README.md docs/` — returns at least four lines (one per file mentioned in requirement 2).
4. `grep -n 'claude-yolo:v0.8.1' CHANGELOG.md` — returns at least one line under `## Unreleased`.
5. `grep -n 'pkg.DefaultContainerImage' pkg/config/` — confirms the constant is still referenced from `pkg/config/config.go` (where it seeds `ContainerImage` in `Defaults()`). No code path is added or removed; the constant flows unchanged.
</verification>
