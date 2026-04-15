---
status: completed
summary: Bumped default claude-yolo container image from v0.5.3/v0.5.1 to v0.5.4 in pkg/const.go, docs/configuration.md (3 occurrences), and example/.dark-factory.yaml; added CHANGELOG Unreleased entry.
container: dark-factory-288-bump-claude-yolo-v054
dark-factory-version: v0.110.0
created: "2026-04-15T18:27:38Z"
queued: "2026-04-15T18:27:38Z"
started: "2026-04-15T18:27:47Z"
completed: "2026-04-15T18:37:17Z"
---

<summary>
- Default container image is bumped to the new claude-yolo release
- Docs and example config reflect the same version
- No behavior changes beyond consuming the new image
</summary>

<objective>
Bump the default `claude-yolo` container image from `v0.5.1` (docs/example) and `v0.5.3` (code constant) to `v0.5.4` everywhere it appears in this repo, so users get the proxy allowlist fix (`console.anthropic.com`) and `LogLevel Connect` by default.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `pkg/const.go` — find the `DefaultContainerImage` constant.
Read `docs/configuration.md` — check all occurrences of `claude-yolo:v0.5.1`.
Read `example/.dark-factory.yaml` — check the `containerImage:` field.
</context>

<requirements>
1. In `pkg/const.go`, change `DefaultContainerImage = "docker.io/bborbe/claude-yolo:v0.5.3"` to `"docker.io/bborbe/claude-yolo:v0.5.4"`.
2. In `docs/configuration.md`, replace all three occurrences of `docker.io/bborbe/claude-yolo:v0.5.1` with `docker.io/bborbe/claude-yolo:v0.5.4`.
3. In `example/.dark-factory.yaml`, replace `docker.io/bborbe/claude-yolo:v0.5.1` with `docker.io/bborbe/claude-yolo:v0.5.4`.
4. Add a CHANGELOG entry under a new `## Unreleased` section at the top (after preamble): `- Bump default claude-yolo container image to v0.5.4 (proxy allowlist + debug logging)`.
5. Run `make precommit` to verify.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT change any other code or docs.
- Existing tests must still pass.
</constraints>

<verification>
Run `grep -rE "claude-yolo:v0\.5\.[0-3]" pkg docs example` — must return zero lines (no stale version strings).
Run `make precommit` -- must pass.
</verification>
