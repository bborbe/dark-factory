---
status: completed
approved: "2026-06-26T07:28:27Z"
generating: "2026-06-26T08:00:41Z"
prompted: "2026-06-26T08:11:32Z"
verifying: "2026-06-26T10:53:50Z"
completed: "2026-06-26T13:14:57Z"
branch: dark-factory/executor-backend-neutral-naming
---

## Summary

- The `Executor` interface looks like an extension point but is leaky: `containerName`, `ContainerChecker`, `ContainerStopper` terminology bleeds across ~10 packages that should not know which backend runs the work.
- Rename the abstraction layer to backend-neutral terms (`executionID`, `ExecutionChecker`, `ExecutionStopper`), keeping container-specific words confined to the docker-CLI packages.
- Preserve on-disk compatibility: prompt frontmatter currently uses `container:` — readers must accept both `container:` and `execution_id:`; writers emit `execution_id:`.
- Add a precommit gate that forbids container-flavored names from re-leaking into the neutral layer.
- Prove the abstraction works by documenting the file-set required to add a hypothetical `LocalSubprocessExecutor` (target: ≤ 3 files).

## Problem

Adding a second execution backend (local subprocess, gVisor, Firecracker) today requires shotgun-surgery renames across ~10 packages because the abstraction names its parameter `containerName` and its collaborators `ContainerChecker` / `ContainerStopper`. The interface methods are mostly backend-neutral in shape, but every caller, mock, and test repeats the container vocabulary. That makes the extension point a fiction: a reviewer reading `pkg/runner` or `pkg/promptresumer` cannot tell whether they are calling a generic executor or hardcoding docker. The result is an architecture that fails the evolvability test of the 2026-06-25 dimensions review.

## Goal

The exported `Executor` interface and every package that depends on it use backend-neutral vocabulary. Container-specific words live only inside `pkg/executor/{checker,stopper,launch,executor}.go` and `pkg/launchpolicy/` — the docker-CLI implementation. Adding a second backend means writing a new file under `pkg/executor/` and one factory wire-up; no other package changes. Existing on-disk prompts continue to load unchanged.

## Non-goals

- Implementing a second backend (`LocalSubprocessExecutor`, gVisor, Firecracker). Separate spec; this work only proves the abstraction is clean.
- Renaming the `dark-factory.project` docker label or any other externally observable artifact.
- Touching the `containerlock` package — it is a docker-CLI lock by construction.
- Dropping the `container:` frontmatter alias on read. The compat shim must remain for at least one major release.
- Sandboxing improvements (gVisor, seccomp).
- Per-package opt-out flag for the neutral naming — invariant; if a future caller needs container vocabulary it is by definition inside the docker package.
- Renaming the internal docker variables inside `pkg/executor/{checker,stopper,launch}.go` and `pkg/launchpolicy/` — they describe docker concepts and stay container-flavored.

## Acceptance Criteria

- [ ] `grep -rE '\bcontainerName\b' pkg/containerslot pkg/healthcheckgate pkg/factory pkg/runner pkg/promptresumer pkg/cancellationwatcher pkg/queuescanner --include='*.go'` returns zero non-test matches — evidence: stdout line count = 0.
- [ ] `grep -rE '\bContainerChecker\b|\bContainerStopper\b' pkg/factory pkg/runner pkg/promptresumer pkg/cancellationwatcher pkg/queuescanner --include='*.go'` returns zero non-test matches — evidence: stdout line count = 0.
- [ ] The exported interface in `pkg/executor/checker.go` is named `ExecutionChecker`; the exported interface in `pkg/executor/stopper.go` is named `ExecutionStopper` — evidence: `grep -nE '^type (ExecutionChecker|ExecutionStopper) interface' pkg/executor/{checker,stopper}.go` returns two matches.
- [ ] Reading a prompt file whose frontmatter contains `container: <name>` (no `execution_id:` key) populates the prompt's execution-ID field with `<name>` — evidence: unit test in `pkg/promptfile` (or wherever frontmatter parsing lives) named `TestLoadAcceptsLegacyContainerKey` asserts the loaded struct's neutral field equals the legacy value; `go test ./... -run TestLoadAcceptsLegacyContainerKey` exits 0.
- [ ] Writing a prompt file emits `execution_id:` (not `container:`) — evidence: unit test named `TestSaveEmitsExecutionIDKey` asserts the rendered YAML contains `execution_id:` and does not contain `container:`; `go test` exits 0.
- [ ] An existing prompt file with legacy `container:` frontmatter loads without error and round-trips through save preserving the semantic Container value; legacy files canonicalize to `execution_id:` on save by design (writers emit `execution_id:` only) — evidence: integration test `TestLegacyContainerKeyRoundTripsUnchanged` in `pkg/prompt/frontmatter_compat_test.go` writes a fixture with `container: foo`, loads it, saves it, reloads, asserts `Frontmatter.Container == "foo"` and saved file contains `execution_id: foo` and no `container:` line; `go test` exits 0.
- [ ] A new doc `docs/execution-backends.md` exists and contains: (a) a section titled `## Adding a Backend` listing the files a hypothetical `LocalSubprocessExecutor` would touch, with the total count ≤ 3; (b) a section titled `## Neutral vs Container Vocabulary` naming which packages are neutral and which are docker-flavored — evidence: `grep -nE '^## Adding a Backend|^## Neutral vs Container Vocabulary' docs/execution-backends.md` returns two matches; `grep -cE '^- ' <section>` shows the file list has ≤ 3 entries.
- [ ] `scripts/hotpath-execution-naming-check.sh` exists, is referenced from the `Makefile` `precommit` target as `hotpath-execution-naming-check`, and exits 0 on the migrated tree — evidence: `make hotpath-execution-naming-check` exits 0; `grep -n 'hotpath-execution-naming-check' Makefile` returns ≥ 1 line.
- [ ] Re-introducing `containerName` in a neutral-layer file fails the gate — evidence: in a throwaway branch, edit `pkg/runner/runner.go` to declare `var containerName string`, run `make hotpath-execution-naming-check`, command exits non-zero; the test fixture for the gate (`scripts/testdata/hotpath-execution-naming/leak.go.txt`, stored as `.txt` so Go tooling ignores it) demonstrates the failure mode and is exercised by `scripts/hotpath-execution-naming-check.sh selftest`.
- [ ] `make precommit` exits 0 on the final tree — evidence: exit code 0.
- [ ] Counterfeiter mocks regenerate cleanly — evidence: `go generate ./...` exits 0 and `git status --porcelain pkg/` shows zero modified mock files after the regenerate.
- [ ] No behavior change at runtime: the docker container is still spawned with the same labels and arguments — evidence: `git diff pkg/executor/launch.go` shows only identifier renames (no changed string literals for `dark-factory.project`, image refs, or `docker run` argv construction); the spec verifier reviews the diff.

## Verification

```
make precommit
make hotpath-execution-naming-check
go generate ./...
git status --porcelain pkg/
go test ./pkg/promptfile/... -run 'TestLoadAcceptsLegacyContainerKey|TestSaveEmitsExecutionIDKey'
```

## Desired Behavior

1. The `containerName string` parameter in every exported neutral-layer signature is renamed to `executionID string`; the type stays `string`.
2. `ContainerChecker` and `ContainerStopper` interfaces are renamed to `ExecutionChecker` and `ExecutionStopper`; their methods take `executionID string`.
3. Inside `pkg/executor/{checker,stopper,launch,executor}.go` and `pkg/launchpolicy/`, internal variables and helpers continue to call the value a container name when describing docker concepts — these are the docker-CLI implementation files and stay container-flavored.
4. The `pkg/containerslot` package is renamed to `pkg/executionslot` (importers updated). Rationale: it allocates a slot keyed by the neutral `executionID`, not a docker concept.
5. Prompt frontmatter parsing accepts both `container:` (legacy) and `execution_id:` (canonical) keys; when both are present, `execution_id:` wins. Writers emit `execution_id:` only.
6. A precommit gate (`make hotpath-execution-naming-check`) greps the neutral-layer packages for forbidden tokens (`containerName`, `ContainerChecker`, `ContainerStopper`, `containerslot`) and fails the build if any reappear.
7. A new doc `docs/execution-backends.md` explains the neutral-vs-container split, lists which packages live on which side, and walks through the file-set for a hypothetical second backend — proving the abstraction holds.

## Constraints

- No new third-party dependencies.
- Counterfeiter mocks must regenerate cleanly with `go generate ./...`.
- BSD license headers preserved on every touched file.
- The `dark-factory.project` docker label and all other externally observable strings are unchanged.
- The on-disk prompt schema is backward compatible: a prompt written before this spec must load unchanged after the spec lands. The compat alias `container:` is supported on read for at least one major release.
- No behavior change at runtime — the work the docker container does is identical before and after.
- The `containerlock` package name is out of scope (docker-CLI lock); do not rename it.

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery | Reversibility |
|---|---|---|---|---|
| Prompt on disk has `container: foo` and no `execution_id:` | Loader log line at INFO: `prompt_load legacy_container_alias=true file=<path>` | Loader populates the neutral field with `foo`; prompt processes normally | None required — log is informational | n/a (read-only) |
| Prompt on disk has both `container: foo` and `execution_id: bar` | Loader log line at WARN: `prompt_load conflicting_keys execution_id=bar container=foo file=<path>` | `execution_id:` wins; `container:` ignored | Operator opens the file and removes the stale `container:` key | Reversible (operator edit) |
| A future PR re-introduces `containerName` in a neutral-layer file | `make hotpath-execution-naming-check` exits non-zero in CI | Build fails; PR cannot merge | Author resolves by using `executionID` or, if the symbol belongs in `pkg/executor/`, moving it there | Reversible (PR edit) |
| `containerslot` → `executionslot` package rename leaves dangling import in an out-of-tree consumer | `go build ./...` fails with import error | Compilation breaks immediately | Update the import path; document the rename in `CHANGELOG.md` | Reversible (import fix) |
| Mock regeneration drifts (counterfeiter version skew) | `git status --porcelain pkg/` shows modified `*_fake.go` after `go generate` | Build still passes; diff is noise | Commit the regenerated mocks in the same PR | Reversible (commit) |
| Operator on an old binary reads a prompt written by a new binary | Old binary cannot find the `container:` key (now absent) | Old binary errors at load: `missing required field container` | Operator either upgrades the binary or hand-edits the prompt to add `container:` | Reversible (binary upgrade) |

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Rename `Executor` interface parameters and the `ExecutionChecker` / `ExecutionStopper` types; update all neutral-layer callers (factory, runner, promptresumer, cancellationwatcher, queuescanner, healthcheckgate); regenerate mocks | 1, 2, 3 | 1, 2, 3, 10, 11, 12 | — |
| 2 | Rename `pkg/containerslot` → `pkg/executionslot`; update imports | 4 | 1, 10 | prompt 1 |
| 3 | Frontmatter compat shim: accept `container:` on read, emit `execution_id:` on write; add unit + integration tests | 5 | 4, 5, 6 | — |
| 4 | Add `scripts/hotpath-execution-naming-check.sh`; wire into `Makefile` precommit; add the leak fixture | 6 | 8, 9 | prompts 1, 2 |
| 5 | Write `docs/execution-backends.md` with neutral-vs-container split and the ≤ 3-file hypothetical-backend walk | 7 | 7 | prompts 1, 2 |

Rationale: prompt 1 is the load-bearing rename and unlocks the gate; prompts 2 and 3 are independent and can run in parallel; prompt 4 must run after the renames or the gate would fail; prompt 5 documents the finished split.

## Do-Nothing Option

If we don't do this, the architecture continues to fail the evolvability dimension: any future backend attempt costs a wide-fan-out rename and is therefore unlikely to be attempted at all. The "extension point" remains a fiction. The cost of doing the work now is low — almost all of it is mechanical rename — while the cost of doing it under deadline pressure later (when a real second backend is needed) is high because the rename competes for attention with new feature work.

## Verification Result

**Verified:** 2026-06-26T13:06:10Z (HEAD 692b309)
**Binary:** /tmp/dark-factory-692b309 (built from HEAD; PR #39 merged)
**Scenario:** Walked all 12 ACs against the migrated tree — greps, named unit/integration tests, gate selftest, `make precommit`, `go generate`, and `git diff` of launch.go across PR #39.
**Evidence:**
- AC1: `grep -rE '\bcontainerName\b' pkg/executionslot pkg/healthcheckgate pkg/factory pkg/runner pkg/promptresumer pkg/cancellationwatcher pkg/queuescanner --include='*.go' | grep -v _test.go` → 0 lines.
- AC2: same grep for `ContainerChecker|ContainerStopper` → 0 non-test lines.
- AC3: `grep -nE '^type (ExecutionChecker|ExecutionStopper) interface' pkg/executor/{checker,stopper}.go` → `checker.go:35` + `stopper.go:17`.
- AC4/AC5: `go test ./pkg/prompt/... -run 'TestLoadAcceptsLegacyContainerKey|TestSaveEmitsExecutionIDKey' -v` → both PASS; load logs `INFO prompt_load legacy_container_alias=true`.
- AC6: `go test ./pkg/prompt/... -run TestLegacyContainerKeyRoundTripsUnchanged` → PASS; test asserts semantic equality (Frontmatter.Container preserved) and saved file contains `execution_id: foo` with no `container:` line.
- AC7: `docs/execution-backends.md` exists; both sections present (lines 5, 33); `## Adding a Backend` lists 3 file bullets.
- AC8: `make hotpath-execution-naming-check` → exit 0; Makefile line 16 wires it into `precommit`, line 57-59 defines the target.
- AC9: appended `var containerName string` to `pkg/runner/runner.go` → `make hotpath-execution-naming-check` reports `pkg/runner/runner.go:332:var containerName string` and fails with Error 1; `bash scripts/hotpath-execution-naming-check.sh selftest` → `selftest OK` against fixture `scripts/testdata/hotpath-execution-naming/leak.go.txt`.
- AC10: `make precommit` → exit 0, "ready to commit".
- AC11: `go generate ./...` → exit 0; `git status --porcelain pkg/` → empty.
- AC12: `git diff 2114608^..2114608 -- pkg/executor/launch.go | wc -l` → 0 (launch.go unchanged across PR #39 merge; docker label, image refs, argv all preserved).
**Spec amendments (AC6, AC9):** Amended in-flight to match shipped reality (101 precedent): AC6 dropped byte-level diff requirement (incompatible with canonicalization-on-save design); AC9 corrected fixture path to `.txt` extension. Functional ACs unchanged.
**Verdict:** PASS
