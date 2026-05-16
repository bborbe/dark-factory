---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-05-16T11:45:09Z"
generating: "2026-05-16T11:48:07Z"
prompted: "2026-05-16T12:00:12Z"
verifying: "2026-05-16T12:37:42Z"
branch: dark-factory/container-naming-project-role-prefix
---

## Summary

- Generation containers currently spawn as `dark-factory-gen-<spec>` regardless of which host project owns the work.
- Execution containers spawn as `<project>-<prompt>` with no `-exec-` infix, breaking visual symmetry with generation containers.
- Rename to a uniform `<project>-<role>-<name>` schema where role is `gen` or `exec`.
- Operators running multiple dark-factory projects (agent, maintainer, trading) can then filter `docker ps` by project, by role, or by both.
- No internal Go identifier changes; only the externally-visible Docker container name strings change.

## Problem

Container names emitted by dark-factory mislead operators about which host project a container is doing work for. Generation containers are hardcoded with the prefix `dark-factory-gen-`, which reads as if dark-factory itself were the work target. In reality the host project (maintainer, agent, trading, etc.) is the consumer; dark-factory is just the tool running on its behalf. With multiple dark-factory daemons running concurrently, operators cannot tell from `docker ps` which project a `dark-factory-gen-*` container belongs to. The execution-side naming is also asymmetric — it carries the project name but no `-exec-` role infix — so the two container types do not visually pair, and a single grep cannot select "all containers for project X" or "all generation runs across projects".

## Goal

Every dark-factory-spawned container — both generation and execution — uses the schema `<project>-<role>-<name>`, where:

- `<project>` is the host project name (directory basename of the project root by default, or an explicit `project:` field in `.dark-factory.yaml`).
- `<role>` is exactly `gen` for spec-generation containers or `exec` for prompt-execution containers.
- `<name>` is the spec basename (for `gen`) or the prompt basename (for `exec`), reused verbatim from the existing logic.

After the change, `docker ps | grep maintainer-` lists every maintainer-owned container; `docker ps | grep -- -gen-` lists every generation container across all projects; and `docker ps | grep maintainer-exec-` lists only maintainer's execution containers.

## Non-goals

- Renaming internal Go package paths, struct names, or function names (e.g., `dockerSpecGenerator`). Only the externally-visible container-name strings change.
- Adding any other metadata to container names (timestamps, daemon PID, host, branch, etc.). Scope is strictly project + role + existing name segment.
- Renaming containers that dark-factory does not spawn (`maintainer-watcher-*` and any other external tooling).
- Migrating or rewriting historical log file names, docker volume names, or status records of already-completed runs. The next container spawn picks up the new name; nothing retroactive.
- Changing the spec or prompt basename derivation logic.

## Desired Behavior

1. A generation container started by dark-factory is named `<project>-gen-<spec-basename>`. Example: maintainer running spec `031-bug-task-controller-respawns-on-terminal-phase` spawns `maintainer-gen-031-bug-task-controller-respawns-on-terminal-phase`.
2. An execution container started by dark-factory is named `<project>-exec-<prompt-basename>`. Example: maintainer running prompt `120-spec-030-verdict-parser-fix` spawns `maintainer-exec-120-spec-030-verdict-parser-fix`.
3. The project name defaults to the basename of the project root directory. Projects with no `.dark-factory.yaml` change continue to work and get the new schema using their directory name.
4. An optional `project:` field in `.dark-factory.yaml` overrides the default project name. The value is used verbatim (after Docker-name sanitization).
5. Status output, log file names, and any operator-facing surfaces that already display the container name show the new schema directly — no separate translation layer.
6. The grep pattern `dark-factory-gen-` no longer appears in any spawned container name produced by `pkg/`.
7. `docker ps --filter name=<project>-` returns both gen and exec containers for that project; `docker ps --filter name=<project>-gen-` returns only generation; `docker ps --filter name=<project>-exec-` returns only execution.

## Constraints

- Container names MUST satisfy Docker's identifier rule `[a-zA-Z0-9][a-zA-Z0-9_.-]*`. The dash-separated schema already complies; sanitization of `<project>` follows the existing prompt `ContainerName.Sanitize()` rules.
- Default project name MUST be the basename of the project root directory so existing projects work without any config change.
- Spec-basename and prompt-basename segments are reused verbatim from current derivation — no re-truncation, no re-formatting in this work.
- Public Go API of `pkg/generator/`, `pkg/processor/`, `pkg/executor/`, `pkg/runner/`, and `pkg/status/` MUST NOT change in signatures or exported symbols. This is an internal formatting fix.
- No new external dependencies.
- See `docs/configuration.md` for the canonical `.dark-factory.yaml` schema; the new `project:` field is documented there.
- The grep pattern `"dark-factory-gen-"` must be removed from production code paths under `pkg/`. Test files may retain it only to assert it is NOT produced.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection | Reversibility |
|---------|-------------------|----------|-----------|---------------|
| `.dark-factory.yaml` has `project:` set to an empty string | dark-factory rejects the config at load time with a clear error citing the field | Operator sets a non-empty value or removes the field to fall back to directory basename | Daemon refuses to start; stderr names the field | Reversible (edit config) |
| Project root directory basename contains characters outside `[a-zA-Z0-9_.-]` | dark-factory sanitizes per the existing `ContainerName.Sanitize()` rules before using the value | None needed; sanitization is silent and deterministic | Resulting container name visible in `docker ps`; matches sanitized form | N/A |
| External tooling greps for the old `dark-factory-gen-` prefix | Old prefix never appears for new runs; external tooling sees zero matches | Tooling owner updates to grep `-gen-` or `<project>-gen-` per the CHANGELOG migration note | External tool reports no matches where it used to find them | Reversible at the tooling side |
| Two host projects share the same directory basename and run dark-factory concurrently | Container names collide on the `<project>-gen-<spec>` segment if the spec basename also matches; Docker rejects the second `docker run` with name-in-use error | Operator sets distinct `project:` values in each project's `.dark-factory.yaml` | Docker daemon error surfaces in the dark-factory log with the failing container name | Reversible (config edit) |
| Existing container still running under the old `dark-factory-gen-*` name when the upgrade lands | dark-factory does not attempt to rename live containers; the running container completes under its old name; the next spawn uses the new schema | None needed | `docker ps` shows the legacy name until that container exits | N/A (intentional) |
| Status resume on daemon restart looks up a container by the old name | Resume logic uses the name recorded at spawn time, not a re-derived one, so legacy containers resume correctly under their legacy names | None needed; lookup is by recorded name | Resume succeeds; logs show legacy name | N/A |

## Security / Abuse Cases

- The `project:` config field is user-controlled text that flows into a Docker container name. Validation MUST go through the same sanitization path as the existing prompt `ContainerName.Sanitize()` to prevent injection of Docker-name-illegal characters or shell metacharacters. No new sanitization rule is introduced; this is a reuse requirement.
- Empty or whitespace-only `project:` values are rejected at config load (failure-mode row above) to prevent producing container names that start with a `-` or are otherwise ambiguous.

## Acceptance Criteria

- [ ] `grep -rn 'dark-factory-gen-' pkg/` returns zero lines in non-test files — evidence: shell command output (exit code 1 from grep, zero matches). Test files may reference the string only inside assertions that it is NOT produced.
- [ ] Generation spawn site produces container name matching `^<project>-gen-<spec-basename>$` — evidence: Ginkgo unit test in `pkg/generator/` captures the container-name argument passed to the runtime and asserts the exact string for a fixture project name and spec basename.
- [ ] Execution spawn site produces container name matching `^<project>-exec-<prompt-basename>$` — evidence: Ginkgo unit test in `pkg/processor/` (or `pkg/executor/`) captures the container-name argument and asserts the exact string for a fixture project name and prompt basename.
- [ ] `.dark-factory.yaml` schema documents the optional `project:` field with default = basename of project root — evidence: diff in `docs/configuration.md` shows the new field row and default semantics.
- [ ] Loading a config with `project: ""` returns a non-nil error whose message names the `project` field — evidence: Ginkgo unit test in `pkg/config/` (or wherever config validation lives) asserts the error type and message substring `project`.
- [ ] Loading a config with no `project:` field uses the project-root directory basename — evidence: Ginkgo unit test asserts the resolved project name equals the basename of a fixture path.
- [ ] `pkg/status/` filters and parses container names by the new schema; the hardcoded `docker ps --filter name=dark-factory-gen-` filter is updated to a project-aware filter — evidence: file diff in `pkg/status/status.go` and unit test asserting the new filter string for a fixture project.
- [ ] `make precommit` in the dark-factory repo exits 0 — evidence: exit code.
- [ ] `CHANGELOG.md` `## Unreleased` section contains a migration note describing the rename `dark-factory-gen-X` to `<project>-gen-X` and `<project>-X` to `<project>-exec-X`, and warning external tooling that greps for `dark-factory-gen-` to update — evidence: file diff in `CHANGELOG.md`.
- [ ] Smoke test in the maintainer project: queue draft spec `smoke-naming-fixture` and approved prompt `smoke-naming-fixture-prompt`; `docker ps --format '{{.Names}}'` shows one container named `maintainer-gen-smoke-naming-fixture` and one named `maintainer-exec-smoke-naming-fixture-prompt` simultaneously; both run to completion under those names; the final status report references the new names — evidence: captured `docker ps` output plus dark-factory status output (file artifact attached to the verification run).

## Verification

```
cd ~/Documents/workspaces/dark-factory
make precommit
grep -rn 'dark-factory-gen-' pkg/ | grep -v _test.go
```

Expected: `make precommit` exits 0; the filtered `grep` returns no lines (exit code 1).

Smoke test (rung-2 equivalent, no k8s involved):

1. Stop any running dark-factory daemons.
2. Build and install the new dark-factory binary: `make install` from the dark-factory repo root.
3. In the maintainer project, place draft spec `smoke-naming-fixture` and approved prompt `smoke-naming-fixture-prompt` in the inbox.
4. Start the dark-factory daemon.
5. Run `docker ps --format '{{.Names}}'` while both containers are alive.
6. Assert one name matches `^maintainer-gen-smoke-naming-fixture$` and the other matches `^maintainer-exec-smoke-naming-fixture-prompt$`.
7. Wait for both to complete; assert dark-factory's final status output references the same names.

## Do-Nothing Option

The current naming works — generation and execution containers run, complete, and are tracked correctly. The cost of doing nothing is purely operator-facing friction: every `docker ps` audit requires the operator to remember that `dark-factory-gen-*` is not actually a dark-factory-owned workload and to manually correlate execution containers to projects without a role infix. This friction compounds as more host projects (agent, maintainer, trading) run dark-factory concurrently. Not doing this leaves a permanent mental-overhead tax on every operations session that touches multiple projects.
