---
status: completed
summary: Bumped default container image from claude-yolo:v0.8.1 to v0.9.0 in pkg/const.go, README.md, docs/yolo-container-setup.md, docs/init-project.md, docs/configuration.md, and added CHANGELOG.md Unreleased entry
container: dark-factory-exec-434-bump-default-image-v0-9-0
dark-factory-version: v0.173.0
created: "2026-05-31T18:56:44Z"
queued: "2026-05-31T19:25:28Z"
started: "2026-05-31T19:29:37Z"
completed: "2026-05-31T19:33:10Z"
---

<summary>
- claude-yolo:v0.9.0 adds `@ast-grep/cli` to the image (PR bborbe/claude-yolo#8, released as v0.9.0)
- dark-factory's default `containerImage` is still pinned to `claude-yolo:v0.8.1`
- Every project that doesn't set `containerImage` in its `.dark-factory.yaml` runs against v0.8.1 — pr-reviewer's mechanical-rules step would silently no-op
- Bump the constant + the three doc spots that show the version string
</summary>

<objective>
Bump dark-factory's default container image from `docker.io/bborbe/claude-yolo:v0.8.1` to `docker.io/bborbe/claude-yolo:v0.9.0` so projects using the default get ast-grep available inside the YOLO container.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

The version string appears in **eight** places across source + docs (verified by `grep -rn 'claude-yolo:v0\.8\.1' pkg/ README.md docs/`). Anchored by surrounding text snippet (line numbers are ~hints, may drift):

| File | ~Line | Surrounding text |
|---|---|---|
| `pkg/const.go` | ~9 | `const DefaultContainerImage = "docker.io/bborbe/claude-yolo:v0.8.1"` — source of truth |
| `README.md` | ~31 | prerequisites bullet `**claude-yolo image** — \`docker pull docker.io/bborbe/claude-yolo:v0.8.1\`` |
| `README.md` | ~148 | config example `containerImage: docker.io/bborbe/claude-yolo:v0.8.1  # YOLO Docker image` |
| `docs/yolo-container-setup.md` | ~172 | `docker pull docker.io/bborbe/claude-yolo:v0.8.1` |
| `docs/init-project.md` | ~11 | prerequisite bullet `**claude-yolo image** pulled (\`docker pull docker.io/bborbe/claude-yolo:v0.8.1\`)` |
| `docs/configuration.md` | ~177 | first config example `containerImage: "docker.io/bborbe/claude-yolo:v0.8.1"` |
| `docs/configuration.md` | ~183 | defaults table row `\| \`containerImage\` \| \`docker.io/bborbe/claude-yolo:v0.8.1\` \| Docker image for YOLO execution \|` |
| `docs/configuration.md` | ~679 | second config example `containerImage: "docker.io/bborbe/claude-yolo:v0.8.1"` |

CHANGELOG.md lines 203 and 211 are historical `chore: Bump default container image to claude-yolo:vX.Y.Z` entries — **do not modify those** (they are history). The pattern there is the template for the new `## Unreleased` entry.

`example/.dark-factory.yaml` pins `claude-yolo:v0.5.4` — intentionally older, do not touch (it is a documentation artifact frozen at the version the example was first authored against).

Reference release: bborbe/claude-yolo#8 added `@ast-grep/cli` to the image. Released as v0.9.0 via maintainer-agent-releaser (minor bump because the PR used `feat:`).
</context>

<requirements>
1. In `pkg/const.go`, change `v0.8.1` → `v0.9.0` in the `DefaultContainerImage` constant. Do not change anything else in the file.
2. In `README.md`, replace **both** occurrences of `claude-yolo:v0.8.1` with `claude-yolo:v0.9.0` (prerequisites bullet + config example).
3. In `docs/yolo-container-setup.md`, replace the one occurrence of `claude-yolo:v0.8.1` with `claude-yolo:v0.9.0`.
4. In `docs/init-project.md`, replace the one occurrence of `claude-yolo:v0.8.1` with `claude-yolo:v0.9.0` (prerequisite bullet).
5. In `docs/configuration.md`, replace **all three** occurrences of `claude-yolo:v0.8.1` with `claude-yolo:v0.9.0` (two config examples + one defaults table row).
6. Add a `## Unreleased` section to `CHANGELOG.md` (above the topmost `## vX.Y.Z` heading — check whether `## Unreleased` already exists first; if so, append to it). Bullet:
   ```
   - chore: Bump default container image to claude-yolo:v0.9.0 (adds `@ast-grep/cli` so projects using the default image have ast-grep available for the doc-driven code-review pipeline's mechanical-rules step)
   ```
   This matches the existing CHANGELOG line 203 / 211 pattern verbatim.
7. Do NOT modify any other files — do not touch tests (`main_internal_test.go` mentions a `v0.6.1` string for `validateModelArg` which is a different concern), do not touch `example/.dark-factory.yaml` (older pin is intentional as a documentation artifact), do not touch any historical CHANGELOG lines.
8. Run `make precommit` to verify.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT attempt `docker build` or `docker run` of claude-yolo — no Docker socket inside the dark-factory container per CLAUDE.md.
- Do NOT modify `pkg/const.go` beyond the version string change.
- Preserve all existing formatting (line endings, indentation, quote style).
</constraints>

<verification>
- `make precommit` must pass.
- `grep -rn 'claude-yolo:v0\.9\.0' pkg/ README.md docs/ | wc -l` returns 8 (the constant + 2 README + 1 yolo-container-setup + 1 init-project + 3 configuration).
- `grep -rn 'claude-yolo:v0\.8\.1' pkg/ README.md docs/` returns no matches (CHANGELOG history excluded by path scope, `example/.dark-factory.yaml`'s `v0.5.4` is intentional and not matched).
- `grep -E '^## Unreleased' CHANGELOG.md` returns one match; `grep -A5 '^## Unreleased' CHANGELOG.md | grep -q 'claude-yolo:v0.9.0'` succeeds.
</verification>
