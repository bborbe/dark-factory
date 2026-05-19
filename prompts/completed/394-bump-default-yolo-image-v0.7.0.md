---
status: completed
summary: Bumped DefaultContainerImage constant and all documentation references from claude-yolo:v0.6.3 to claude-yolo:v0.7.0, and added CHANGELOG Unreleased entry.
container: dark-factory-exec-394-bump-default-yolo-image-v0-7-0
dark-factory-version: v0.162.0
created: "2026-05-19T19:55:00Z"
queued: "2026-05-19T17:58:44Z"
started: "2026-05-19T17:58:46Z"
completed: "2026-05-19T18:03:58Z"
---

<summary>
- Bump the default YOLO container image from `docker.io/bborbe/claude-yolo:v0.6.3` to `docker.io/bborbe/claude-yolo:v0.7.0` so new projects and projects without a `containerImage:` override pick up the v0.7.0 release (which adds `api.minimax.io` to the tinyproxy egress allowlist)
- Update all documentation references to the previous default image tag so README, configuration guide, init-project guide, and yolo-container-setup guide all show the new tag
- No behavioral changes beyond the version bump
</summary>

<objective>
Make `docker.io/bborbe/claude-yolo:v0.7.0` the project-wide default container image. All hardcoded references to `v0.6.3` in code, docs, and the default-config table become `v0.7.0`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/dod.md` for the project's definition of done.

Files to read in full before editing:
- `pkg/const.go` — `DefaultContainerImage` constant (line 9)
- `README.md` — references at lines ~31 and ~148
- `docs/configuration.md` — references at lines ~177, ~183, and ~634 (three occurrences total)
- `docs/init-project.md` — reference at line ~11
- `docs/yolo-container-setup.md` — reference at line ~172
</context>

<requirements>

## 1. Update the `DefaultContainerImage` constant

Edit `pkg/const.go`. Change line 9 from:

```go
const DefaultContainerImage = "docker.io/bborbe/claude-yolo:v0.6.3"
```

to:

```go
const DefaultContainerImage = "docker.io/bborbe/claude-yolo:v0.7.0"
```

## 2. Update documentation references

In each of the following files, replace every occurrence of the literal string `claude-yolo:v0.6.3` with `claude-yolo:v0.7.0`. Do not change any other version reference (do not bump `v0.6.1`, `v0.2.9`, or any other tag elsewhere — only the explicit current default).

- `README.md`
- `docs/configuration.md`
- `docs/init-project.md`
- `docs/yolo-container-setup.md`

Use a search-and-replace approach but verify each replacement is in the expected context (default-image documentation, not historical examples).

## 3. Do NOT change test fixtures

The following files reference older tags (`v0.6.1`, `v0.2.9`, `v0.1.2`) and `latest` in test cases that are NOT about the default image — leave them untouched:

- `main_internal_test.go`
- `pkg/config/config_loader_test.go`
- `specs/completed/060-config-layering-phase-1.md`
- `specs/completed/079-bug-completion-report-parser-tail-boundary.md`
- `CHANGELOG.md` historical entries

## 4. Add CHANGELOG entry

If `CHANGELOG.md` does not already have an `## Unreleased` section above the topmost released version header (currently `## v0.163.1`), create one directly below the preamble. Then add the bullet under it:

```
- Bump default container image to claude-yolo:v0.7.0 (adds api.minimax.io to tinyproxy egress allowlist for MiniMax Anthropic-compatible API)
```

After the edit, the top of `CHANGELOG.md` (after the preamble) should look like:

```
## Unreleased

- Bump default container image to claude-yolo:v0.7.0 (adds api.minimax.io to tinyproxy egress allowlist for MiniMax Anthropic-compatible API)

## v0.163.1
...
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT change any test fixtures, historical changelog entries, or completed spec files.
- Do NOT alter the `pkg.DefaultContainerImage` constant's name, type, or godoc comment — only its string value.
- Do NOT introduce a new helper, function, or abstraction; this is a literal value bump.
- After the edit, `grep -rn 'claude-yolo:v0.6.3' .` (excluding `vendor/`, `CHANGELOG.md` historical entries, `specs/completed/`, and `*_test.go`) should return zero lines.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional checks:
1. `grep -n 'v0.7.0' pkg/const.go` — returns exactly one line (the constant value).
2. `grep -rn 'claude-yolo:v0.6.3' README.md docs/` — returns zero lines.
3. `grep -rn 'claude-yolo:v0.7.0' README.md docs/` — returns at least four lines (one per file mentioned in requirement 2).
4. `grep -n 'claude-yolo:v0.7.0' CHANGELOG.md` — returns at least one line under `## Unreleased`.
5. `grep -n 'pkg.DefaultContainerImage' pkg/config/` — confirms the constant is still referenced from `pkg/config/config.go` (where it seeds `ContainerImage` in `Defaults()`). No code path is added or removed; the constant flows unchanged.
</verification>
