---
status: approved
tags:
    - dark-factory
    - spec
approved: "2026-05-19T16:03:22Z"
branch: dark-factory/global-env-layering
---

## Summary

- Today the `env:` node only exists in the per-project config; it can't be set machine-wide.
- Allow `env:` in `~/.dark-factory/config.yaml` so per-user provider/auth settings live outside any git repo.
- Project `.dark-factory.yaml` `env:` overrides global `env:` per-key (key-level merge, not section replacement).
- The merged environment map is what flows into the YOLO container — same code path as today's project-only env.
- Document a secrets exception: literal values are permitted in the global home file (never committed) but still forbidden in project yaml; a file-permission warning surfaces if the home file is world/group-readable.

## Problem

Users running dark-factory against alternate Anthropic-compatible providers (or with other per-machine environment variables) currently have only two bad options: commit provider auth values into `.dark-factory.yaml` (a checked-in file, leaks secrets), or maintain ad-hoc shell wrappers that export environment variables before invoking dark-factory (fragile, easy to forget). The existing layering chain already merges user-prefs from `~/.dark-factory/config.yaml` into the project config for fields like `model` and `hideGit` — extending the same merge to `env:` removes the unsafe defaults and lets each project still override individual keys (e.g., `GOPATH`) without having to redeclare the whole environment.

## Goal

After this change is shipped:

- A user can set environment variables once in their home config and have every project pick them up automatically.
- A project config can override or add individual environment keys without touching the rest of the inherited map.
- The container launched by dark-factory sees the fully merged environment map, with project keys winning on collision.
- The home config is permitted to carry literal secret values; the project yaml is not. The policy difference is documented and enforced at load.
- An operator running dark-factory can verify, from logs, which env keys came from which layer — without secret values appearing in those logs.

## Non-goals

- Does NOT introduce env-ref indirection (e.g., `tokenEnv: NAME`) for the global env node — literal values are allowed in the home file by design.
- Does NOT change the project-only status of `extraMounts`, `notifications.*`, or any other category-B field.
- Does NOT implement the Phase 2 `DF_<FIELD>` env-var override layer; the precedence chain remains `default ← global ← project ← arg`.
- Does NOT validate the semantic correctness of env values (no provider URL checks, no credential format checks).
- Does NOT redact env values from process memory, dumps, or non-dark-factory logs — only from dark-factory's own `effective config` log line.
- Does NOT define behavior on non-POSIX filesystems beyond skipping the home-file permission check.

## Desired Behavior

1. Global config alone defines env: with no project env, the container launched by dark-factory receives every key/value from the global env map.
2. Project config alone defines env: behavior is unchanged from today — only the project keys reach the container.
3. Both configs define env with overlapping keys: for each overlapping key, the project value wins; non-overlapping keys from both layers are preserved (union semantics).
4. Both configs define env with disjoint keys: the container receives the union of both sets.
5. The `effective config` log line emitted at startup reports, for env, the set of keys grouped by source (e.g., `from-global=[...]`, `project-overrides=[...]`) and never reports values.
6. When the home config file exists but is readable by group or other, dark-factory logs a single warning at load and continues; the file is still loaded.
7. An env key that does not match the allowed identifier pattern causes config load to fail with a clear error naming the offending key; the run does not proceed with a partially loaded env.

## Constraints

- The global config schema (`~/.dark-factory/config.yaml`) gains exactly one new top-level field: `env`, a string-to-string map. No other schema changes at the global layer.
- The project config schema is unchanged.
- Env key names must match `^[A-Z_][A-Z0-9_]*$`. Keys outside this pattern are rejected at config load with the offending key named in the error.
- Env values are opaque strings. No interpretation, expansion, or escaping is performed by dark-factory; values are passed verbatim to the container launcher.
- Merge precedence: project keys win over global keys on collision. Within a single yaml document, the standard yaml.v3 "last duplicate wins" rule applies and is not overridden.
- The same downstream code that today consumes project `env:` and forwards it to the container must, after this change, consume the merged map. No second code path may be introduced.
- Home file permission check: if `stat` reports any of group-read, group-write, other-read, other-write on `~/.dark-factory/config.yaml`, emit one warning log line at load time. Loading continues regardless. The check is best-effort on platforms without POSIX permission bits.
- Literal secret values are permitted only in the home config. Project yaml continues to forbid literal secrets per the existing secrets carve-out — this spec does not relax that.
- Documentation updates are part of scope:
  - `docs/config-layering.md` must remove `env` from the "Out of scope: per-project only" list and document the new key-level merge plus the home-file secrets exception.
  - `docs/configuration.md` must gain a "Global env" subsection under its Global Config section describing the home-file env node, override semantics, and the permission warning.
- The existing `effective config` log line is the single point where env-source reporting appears. No new log line is introduced for this purpose.
- All currently passing tests in `pkg/globalconfig/...`, `pkg/config/...`, and the container-launch code path must continue to pass.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection | Reversibility |
|---------|-------------------|----------|-----------|---------------|
| Global config contains env key with lowercase or invalid chars | Load fails with error naming the offending key; run does not start | Fix the key in `~/.dark-factory/config.yaml` and re-run | stderr / non-zero exit at startup | Reversible |
| Global config file is malformed yaml | Load fails with yaml parse error including line number; run does not start | Fix the yaml syntax | stderr / non-zero exit at startup | Reversible |
| Global config file is missing | Treated as empty global config; project env (if any) is used as-is | No recovery — expected state for users without a home config | No warning emitted | n/a |
| Global config file is world- or group-readable | One warning log line at load; loading continues, env values still used | Tighten perms with `chmod 600 ~/.dark-factory/config.yaml` | Warning log line at startup | Reversible without restart |
| Same env key declared twice within a single yaml layer | yaml.v3 default applies (last occurrence wins); no special handling | n/a — author should deduplicate | None at runtime | Reversible |
| Project env overrides a global env key the user forgot they set | Project value wins; container receives project value | Inspect `effective config` log line to see which keys are project-overrides | `effective config` log line lists `project-overrides=[KEY,...]` | Reversible |
| Validation failure during load (any of the above fatal cases) | Error message does NOT include env values for any key | Read the error, fix the config | stderr text contains no env values | n/a |

## Security / Abuse Cases

- **Trust boundary**: `~/.dark-factory/config.yaml` is user-owned and not committed. `.dark-factory.yaml` is repo-tracked and may be read by anyone with repo access. Allowing literal secrets in the former and forbidding them in the latter is the central security decision of this spec.
- **Permission leak**: a user with a permissive umask could create the home config world-readable. Mitigation: load-time warning. Refusing to load was considered and rejected (too many users have `umask 022` and would hit it on first run).
- **Log leak**: env values must never appear in dark-factory's `effective config` log line, validation error messages, or any other startup-time log emitted by dark-factory. Keys may appear; values may not.
- **No shell interpolation**: env values are passed verbatim to the container launcher's argument list (no shell substitution by dark-factory), so a value containing shell metacharacters is not a host-side injection vector. Restrictions on values are intentionally absent.
- **No attacker-controlled inputs cross a network boundary** in this feature; both yaml files are local to the operator.

## Acceptance Criteria

- [ ] The global config struct exposes a string-to-string env map — evidence: `grep -n '^\tEnv ' pkg/globalconfig/globalconfig.go` returns at least one line.
- [ ] Loading a global config with env passes validation for keys matching `^[A-Z_][A-Z0-9_]*$` and rejects others — evidence: `go test ./pkg/globalconfig/... -run TestEnv -v` exits 0 and includes both positive and negative test cases.
- [ ] Project env overrides global env on a per-key basis (project keys win, non-colliding keys from both layers are preserved) — evidence: `go test ./pkg/config/... -run TestEnvMerge -v` exits 0 with at least three sub-cases: global-only, both-with-overlap, both-disjoint.
- [ ] The container launched by dark-factory receives the merged env map — evidence: `go test ./pkg/runner/... -run TestContainerLaunchReceivesMergedEnv -v` (or the matching test in `pkg/processor/...` if the launcher seam lives there) exits 0 and asserts that the env flags passed to the container launcher reflect the merged map (global ∪ project, project-wins-on-collision).
- [ ] The startup `effective config` log line reports env keys grouped by source and contains no env values — evidence: a test captures the log line and asserts (a) every env key appears in exactly one of the source groupings, (b) no env value substring appears anywhere in the line.
- [ ] When the home config is world- or group-readable, a single warning line is emitted at load and loading proceeds — evidence: a test sets perms to `0644`, captures logs, asserts one warning line containing the file path and that the loaded config is non-empty.
- [ ] `docs/configuration.md` documents the global env node — evidence: `grep -n 'Global env' docs/configuration.md` returns at least one line.
- [ ] `docs/config-layering.md` reflects env's new global-eligible status — evidence: `grep -n 'env: now supported globally' docs/config-layering.md` returns at least one line (or an equivalent phrasing that supersedes the old "Out of scope: per-project only" entry; the prior phrase must no longer appear: `grep -n 'env.*per-project only' docs/config-layering.md` returns zero lines).
- [ ] All existing tests continue to pass — evidence: `make precommit` exits 0.

## Verification

```
make precommit
go test ./pkg/globalconfig/... -run TestEnv -v
go test ./pkg/config/... -run TestEnvMerge -v
grep -n '^\tEnv ' pkg/globalconfig/globalconfig.go
grep -n 'Global env' docs/configuration.md
grep -n 'env: now supported globally' docs/config-layering.md
grep -n 'env.*per-project only' docs/config-layering.md   # must return nothing
```

## Do-Nothing Option

If this change is not made, users running dark-factory against alternate Anthropic-compatible providers have two unsafe choices: commit provider auth tokens into `.dark-factory.yaml` (leaks secrets through git history; every repo with a provider override becomes a credential leak vector), or maintain handwritten shell wrappers that export environment variables before invoking dark-factory (fragile, undiscoverable, breaks across shells, breaks in CI). Both options will continue to spread as more providers come online. The cost of inaction is normalizing unsafe defaults — secrets in tracked files — across an unbounded number of repos.
