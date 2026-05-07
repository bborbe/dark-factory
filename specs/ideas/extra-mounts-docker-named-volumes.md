---
status: idea
---

## Summary

- `extraMounts.src` gains a `volume://<name>` prefix that translates to a Docker named volume instead of a host bind mount.
- Aimed at caches that should be linux-native, fast, and isolated from the host (notably `go-build` and `golangci-lint`) on macOS hosts where bind mounts to host directories are slow and rarely cache-hit due to OS keying.
- Volume mounts skip the host-path existence check and are auto-created by Docker on first reference.
- The existing `readOnly` flag continues to apply.
- Module cache (`go/pkg/mod`) intentionally stays a bind mount — sharing with the host is desirable there because entries are source-only and OS-agnostic.
- Idea-stage: volume-name scoping policy is unresolved and blocks moving to draft.

## Problem

`.dark-factory.yaml`'s `extraMounts` only accepts host filesystem paths. On macOS hosts running Linux containers — the common dark-factory case — bind-mounting host cache directories is costly. Docker Desktop routes bind mounts through virtiofs/9p, which is slow on the many-small-files workloads that `go-build` and `golangci-lint` produce, so `make precommit` inside YOLO is noticeably slower than on the host. Worse, the host (darwin) and container (linux) write into the same directory but the cache entries rarely help each other: `go-build` keys on `GOOS/GOARCH`, and `golangci-lint`'s type-check layer inherits that property. Cache size grows; cache hit rate stays low. Operators need a way to give caches a linux-native, container-shared backing store without giving up the bind-mount option for caches that *do* benefit from host sharing.

## Goal

After this work, an operator can write `src: volume://go-build` in `extraMounts` and dark-factory will mount a Docker named volume at the destination path inside the container. Repeated dark-factory runs (and parallel containers, where applicable) reuse the same volume, so the cache survives across runs and is shared by every linux container that mounts it. Bind mounts continue to work unchanged — operators choose per-mount which model fits each cache.

## Non-goals

- No auto-cleanup or garbage collection of named volumes — operators run `docker volume rm` / `docker volume prune` manually.
- No `volume://` support outside `extraMounts.src` (workspace mount, claude config mount, gitconfig, netrc, etc. remain bind-only).
- No change to the default mount strategy — existing bind mounts continue to work; volume mounts are opt-in.
- No support for volume drivers beyond local.
- No conversion of the Go module cache to a volume — it stays a bind mount by design.

## Desired Behavior

1. An `extraMounts` entry whose `src` begins with `volume://` is treated as a Docker named volume reference. The text after the prefix is the volume name.
2. Volume-prefixed entries skip the host-path existence check that bind mounts use; they never produce a "src path does not exist, skipping" warning.
3. Volume-prefixed entries are emitted to `docker run` as `-v <name>:<dst>` and rely on Docker's auto-create behavior (no separate `docker volume create` step).
4. `readOnly: true` on a volume-prefixed entry produces `-v <name>:<dst>:ro`.
5. Bind-mount entries (any `src` not matching the prefix) behave exactly as today, including the existence check and warn-and-skip on missing paths.
6. The configuration documentation explains when to use a volume vs a bind mount, with worked examples for `go-build` and `golangci-lint` (volume) and `go/pkg/mod` (bind).
7. The documentation calls out the inspection trade-off: volume contents are not directly visible on the host filesystem and must be inspected via a throwaway container.

## Constraints

- The Go module cache mount in the example config and docs **must remain a bind mount**. Sharing with the host is intentional there.
- All existing bind-mount behavior is preserved unchanged. No regression in path resolution, env-var expansion, `~/` expansion, relative-path handling, or warn-and-skip on missing src.
- The `readOnly` field semantics are unchanged across both mount kinds.
- The `volume://` prefix is recognized only in `extraMounts.src`, not in any other config field.
- Existing `extraMounts` configs without the prefix continue to load and run identically.
- Reference: existence-check site is `pkg/executor/executor.go` (around the bind-mount argument assembly); tests live in `pkg/executor/executor_test.go` and `pkg/executor/resolve_extra_mount_src_test.go`. These are implementation pointers, not frozen contracts — the prompt phase decides the final structure.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `src: volume://` (empty volume name) | Config validation rejects the entry with a clear error before the container starts | Operator fixes the config |
| `src: volume://name with spaces` or other illegal Docker volume name | Config validation rejects with a clear error referencing Docker volume naming rules | Operator picks a valid name |
| Volume mount destination path collides with a bind mount destination | Same behavior as today's duplicate-destination case (last-wins or error — match existing behavior) | Operator removes the duplicate |
| Docker daemon unavailable or volume creation denied | `docker run` fails; dark-factory surfaces the docker error verbatim | Operator fixes Docker / permissions |
| Operator wants to inspect volume contents | Documented workaround: `docker run --rm -v <name>:/data alpine ls /data` | Documented in configuration.md |

## Security / Abuse Cases

- The volume name comes from operator-controlled config, not external input. Trust boundary is unchanged from today.
- Validation must still reject names that could escape the `-v` argument or inject additional flags (control chars, leading dashes). Docker's own volume-name regex is sufficient if applied.
- No new filesystem paths from the host are exposed; if anything, volumes reduce host exposure compared to bind mounts.

## Open Questions (idea-stage blockers)

These must be resolved before this spec moves to draft. They are listed in priority order.

1. **Volume-name scoping policy.** Should dark-factory auto-namespace volumes per project (e.g. prefix with the project slug) or pass the operator's name through verbatim?
   - Auto-prefix: safer; two projects on the same host cannot accidentally share state.
   - Verbatim: maximum flexibility; multiple dark-factory projects building Go code can legitimately share a single `go-build` volume.
   - Hybrid convention (e.g. `volume://shared/<name>` vs `volume://project/<name>`) adds expressive power but also config-grammar complexity.
   - Decision shape: pick one default and document the trade-off; this choice ripples into every other open question.

2. **Variable expansion in volume names.** `extraMounts.src` already expands `${VAR}` for bind mounts. Should `volume://${PROJECT}-go-build` work? If yes, how does that interact with question 1 (does expansion happen before or after auto-prefixing)?

3. **Read-only confirmation.** Verify that translating to `-v <name>:<dst>:ro` actually delivers read-only semantics for named volumes the way it does for bind mounts (Docker behavior is consistent here, but pin it explicitly so the agent does not have to rediscover).

4. **Destination ownership in YOLO.** YOLO containers run as `--user node`. Confirm a fresh named volume mounted at `/home/node/.cache/go-build` is writable by `node` on first use. (Docker normally chowns to match the destination's pre-existing in-image owner, but this needs verification for the YOLO image specifically.)

5. **Cleanup story.** Ship a `dark-factory volume prune` helper, document the raw `docker volume` commands, or both? Affects scope: a helper means new CLI surface and possibly a separate prompt.

## Acceptance Criteria

_(These will firm up once the open questions are resolved. Listed here as the shape of what verification will look like.)_

- [ ] An `extraMounts` entry with `src: volume://<name>` runs the container with `-v <name>:<dst>` (and `:ro` when `readOnly: true`).
- [ ] No "src path does not exist" warning is emitted for volume-prefixed entries.
- [ ] Bind-mount entries continue to behave identically to today (path resolution, env expansion, missing-path warn-and-skip).
- [ ] Config validation rejects empty and illegal volume names with a clear error.
- [ ] `docs/configuration.md` documents the new prefix, the use-vs-not-use guidance for caches, and the volume-inspection workaround.
- [ ] The existing example/test config keeps `go/pkg/mod` as a bind mount.
- [ ] `make precommit` passes.

**Scenario coverage:** No new scenario expected. The behavior is reachable by unit tests on the mount-arg assembly and integration tests that exercise `docker run` argument generation. No existing essential user journey is changed; volumes are an opt-in alternative for an existing field.

## Verification

```
make precommit
```

## Do-Nothing Option

Operators can keep using bind mounts. On macOS, `make precommit` inside YOLO stays slower than on the host, and cache disk usage grows roughly twice as fast as it needs to (one set of entries per OS) without a matching hit-rate gain. Sufferable but a steady tax on every run; this spec eliminates it for caches that should be linux-only.

## Notes for Spec Author and Future Prompt Writer

Implementation pointers, not frozen constraints:

- `extraMounts` parsing and the `docker run` argument assembly live together in the executor package; the existence check warns and skips at the bind-mount path-resolution site.
- Tests for mount-arg behavior exist in the executor package (`executor_test.go`, `resolve_extra_mount_src_test.go`).
- This is a pure dark-factory feature — no claude-yolo image change required.
- The expected operator-visible win is per-mount: pick volume for caches that are container-only (linux build cache, linter cache) and bind for caches that are OS-agnostic and benefit from host sharing (Go module cache, source-like data).
