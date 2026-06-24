---
status: approved
spec: [098-bug-unify-container-launch-policy]
created: "2026-06-24T19:30:00Z"
queued: "2026-06-24T19:40:37Z"
---

<summary>

- Adds a `## Unreleased` CHANGELOG entry summarising the unified launch policy and the OrbStack startup fix, referencing spec 098.
- Adds a troubleshooting paragraph for the OrbStack `claude session probe failed: stdout=""` failure mode, so future operators hitting the symptom can confirm the fix shipped.
- Re-runs the spec's hermetic-layer architectural invariants (greps + tests) to confirm prompts 1-4 left the tree in the expected state — if any invariant fails, surface the failure rather than patching.
- No `.go` file is touched — this prompt is documentation only.
- The spec's bug-mandatory runtime replay on OrbStack is **not** part of this prompt; it lives in the spec's Verification block (lines 107-115) and is executed by the operator via `/dark-factory:verify-spec 098` before `dark-factory spec complete`.

</summary>

<objective>
Add a `## Unreleased` CHANGELOG entry and a troubleshooting paragraph for the OrbStack claude-probe failure mode, so the fix is discoverable post-release. Re-check the spec's hermetic-layer invariants as a guard-rail against incomplete prior-prompt output.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — `## Unreleased` entry format, prefix (`feat:` / `fix:` / `refactor:`) required, one bullet per discrete change, terse bullets matching the repo's `v0.180.0+` style
- `/home/node/.claude/plugins/marketplaces/coding/docs/documentation-guide.md` — concise prose, match surrounding section tone

Read these source files:

- `/workspace/CHANGELOG.md` — top of file. Check whether `## Unreleased` already exists above the latest `## vX.Y.Z`; create it if absent, append to it if present.
- `/workspace/docs/troubleshooting.md` — read end-to-end; check existing section style (e.g. `## Preflight baseline failure on daemon start`) to match tone for the new section.
- `/workspace/specs/in-progress/098-bug-unify-container-launch-policy.md` — `Reproduction` block + `Acceptance Criteria`. The spec's own Verification block names the operator-driven OrbStack runtime replay — that step is **out of scope for this prompt** and lives in the spec's verification rung (`/dark-factory:verify-spec 098`), not here.

The runtime replay (`dark-factory daemon` on OrbStack) cannot run inside the YOLO container and is not the agent's job. dark-factory's spec-verification rung exists exactly to gate "shipped but unverified" work; the operator runs it after merge. This prompt's deliverable is the post-release discoverability surface (CHANGELOG + troubleshooting doc).

</context>

<requirements>

## 1. CHANGELOG entry

In `/workspace/CHANGELOG.md`, add or extend a `## Unreleased` section at the top of the version list (above the latest `## vX.Y.Z`).

If `## Unreleased` does not exist, create it. If it exists, append the bullets below to it (do NOT create a duplicate `## Unreleased` section).

Add these bullets (one line each, matching the repo's `v0.180.0+` terse style):

```
- fix: healthcheck probes now carry NET_ADMIN/NET_RAW caps; daemon starts on OrbStack without --skip-healthcheck (spec 098)
- refactor: pkg/launchpolicy is the single source of truth for container launch shape; canonical capability set lives there only (spec 098)
```

Prefixes (`fix:`, `refactor:`) match the changelog guide's required forms.

## 2. Troubleshooting doc

In `/workspace/docs/troubleshooting.md`, ensure there is a paragraph covering the OrbStack startup failure mode. If a section like `## Healthcheck` or `## Daemon startup` exists, extend it; otherwise add a new section in a position consistent with the existing structure (e.g. near `## Preflight baseline failure on daemon start` — match that section's tone and length).

The paragraph must cover:
- **Symptom**: macOS + OrbStack; `dark-factory daemon` aborts with `healthcheck: claude session probe failed: stdout=""`
- **Root cause**: one sentence — probe's container lacked `NET_ADMIN`/`NET_RAW`; `claude-yolo` entrypoint's `init-firewall.sh` failed iptables ops
- **Status**: fixed in the release containing spec 098 (or referencing the new `pkg/launchpolicy` package as the single source of truth)
- **Workaround mention**: `dark-factory daemon --skip-healthcheck` bypasses the gate but disables its protection — prefer upgrading

Match the surrounding section style for headings, bullet vs prose, and length. Do not over-prescribe; the doc author chooses the wording.

## 3. Hermetic-layer guard-rail (catches incomplete prior-prompt output)

Before claiming completion, re-run the spec's full architectural and test invariants from inside the container. These were delivered by prompts 1-4; this prompt re-asserts them as a guard so an incomplete chain does not slip through:

```bash
cd /workspace
go test ./pkg/launchpolicy/... ./pkg/executor/... ./pkg/cmd/healthcheck/... ./pkg/factory/...
# expected: PASS

grep -rn "NET_ADMIN" pkg/ | grep -v _test.go
# expected: 1 line (pkg/launchpolicy/policy.go)

grep -rn "ContainerLaunchOpts{" pkg/ | grep -v _test.go
# expected: 1 line (pkg/launchpolicy/policy.go BuildOpts)

grep -n "NET_ADMIN" pkg/executor/executor.go
# expected: 0 lines

grep -n "ContainerLaunchOpts{" pkg/cmd/healthcheck/probes.go
# expected: 0 lines

grep -rn "ProbeLaunchConfig" pkg/
# expected: 0 lines

grep -rn "SYS_PTRACE" pkg/ mocks/ cmd/
# expected: matches only in pkg/launchpolicy/regression_lock_test.go

make precommit
# expected: exit 0
```

ALL of these must hold. Any failure means a prior prompt's change is incomplete — STOP and report the failing invariant. Do NOT patch `.go` files from here.

## 4. No code changes

This prompt MUST NOT modify any `.go` file. If the guard-rail at step 3 fails, the gap lives in prompts 1-4; STOP and surface it rather than patching here.

</requirements>

<constraints>

- The CHANGELOG entry MUST reference spec 098 in at least one bullet (spec convention; see `changelog-guide.md`).
- Prefixes on CHANGELOG bullets are mandatory (`fix:`, `refactor:`); the supplied prefixes are the right ones.
- Do NOT add multiple `## Unreleased` sections — append to the existing one if present.
- Do NOT modify any `.go` source file.
- Do NOT invent a "human-verification-required" completion-report flag — dark-factory's spec-verification rung (`/dark-factory:verify-spec 098`) is the documented gate for operator-driven verification. This prompt's job ends at the documentation surface.
- Do NOT commit — dark-factory handles git.

</constraints>

<verification>

```bash
cd /workspace

# 1. Hermetic-layer rerun (gates everything else in this prompt)
go test ./pkg/launchpolicy/... ./pkg/executor/... ./pkg/cmd/healthcheck/... ./pkg/factory/...
# expected: PASS

# 2. Architectural invariants (spec ACs — guard-rail; failures point at prompts 1-4)
test "$(grep -rn 'NET_ADMIN' pkg/ | grep -v _test.go | wc -l)" -eq 1
test "$(grep -rn 'ContainerLaunchOpts{' pkg/ | grep -v _test.go | wc -l)" -eq 1
test "$(grep -n 'NET_ADMIN' pkg/executor/executor.go | wc -l)" -eq 0
test "$(grep -n 'ContainerLaunchOpts{' pkg/cmd/healthcheck/probes.go | wc -l)" -eq 0
test "$(grep -rn 'ProbeLaunchConfig' pkg/ | wc -l)" -eq 0

# 3. CHANGELOG sanity
test "$(grep -c '^## Unreleased$' CHANGELOG.md)" -eq 1
grep -E '^- (fix|refactor): .*spec 098' CHANGELOG.md   # expected: >= 1 line

# 4. Troubleshooting doc has the verified-fix paragraph
grep -E 'spec 098|OrbStack' docs/troubleshooting.md   # expected: >= 1 match

# 5. Full precommit (also runs check-changelog — the new Unreleased entry must validate)
make precommit
# expected: exit 0
```

On any failure in step 1 or 2, STOP — the prior prompts have a gap and this prompt cannot close it. On `make precommit` failure inside `check-changelog`, fix the entry shape (prefix, position above the latest release header, exactly one `## Unreleased`) and re-run.

</verification>
