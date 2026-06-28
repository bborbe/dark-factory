---
status: completed
summary: Added unconditional --add-host=host.docker.internal:host-gateway emission to BuildDockerRunArgs with a Ginkgo spec asserting presence and a CHANGELOG entry
execution_id: dark-factory-add-host-exec-486-add-host-docker-internal
dark-factory-version: v0.187.11
created: "2026-06-28T11:30:00Z"
queued: "2026-06-28T11:18:52Z"
started: "2026-06-28T11:18:59Z"
completed: "2026-06-28T11:22:53Z"
---

<summary>
- Add `--add-host=host.docker.internal:host-gateway` to every container `docker run` argv that `BuildDockerRunArgs` produces.
- Unblocks raw Linux `dockerd` users from routing dark-factory containers through a host-side `claude-code-router` (the alias isn't auto-provided there).
- Safe on macOS Docker Desktop / OrbStack / Rancher Desktop: those runtimes already provide the alias; `--add-host` overwrites with the same value as a no-op.
- 5-line change in `pkg/executor/launch.go` mirroring the existing `appendXxx` helper pattern, plus one Ginkgo spec asserting the flag appears in the produced argv.
- No other call-site changes needed — every spawn path goes through `BuildDockerRunArgs`.
</summary>

<objective>
Make dark-factory's container spawner unconditionally emit `--add-host=host.docker.internal:host-gateway` so containers can reach the host's `host.docker.internal` regardless of which Docker runtime the operator uses.
</objective>

<context>
Read first (in this order):
- `/workspace/CLAUDE.md` — project conventions.
- `/workspace/pkg/executor/launch.go` — `BuildDockerRunArgs` (line 35) is the single source of truth for all spawn argv. Note the existing `appendXxx(args, opts)` helper pattern: `appendSecurityLimits`, `appendExtraLabels`, `appendEnv`, `appendStandardMounts`, `appendExtraMounts`, `buildHideGitArgsForRoot`. The new helper should mirror this style.
- `/workspace/pkg/executor/launch_test.go` (or `launch_argv_test.go` — check both) — existing Ginkgo argv-shape tests are the place to add the new spec.
- `/workspace/pkg/launchpolicy/opts.go` — `ContainerLaunchOpts` struct; check if it already has a `HostAliases` or similar field. If yes, the new helper consumes it; if not, the simpler unconditional emission stays inside `BuildDockerRunArgs` (NO new opts field — keep this prompt's surface small).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-glog-guide.md` — for any logging additions (none expected here).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo + Gomega conventions used in this repo.
</context>

<requirements>

1. **Add the `--add-host` flag unconditionally to `BuildDockerRunArgs`.** Insert it right after the existing `args := []string{...}` literal (lines 36-40), before `appendSecurityLimits` so the network-aliases land in a stable position near the top of the argv. The simplest implementation is a literal append:

   ```go
   args = append(args, "--add-host=host.docker.internal:host-gateway")
   ```

   Do NOT introduce a new `opts.HostAliases` field. Do NOT introduce an `appendHostAliases` helper unless the existing file's style demands it (i.e., every other concern already lives in a helper — in which case mirror the pattern with a small `appendHostAliases(args []string) []string` that returns the appended slice and is called from `BuildDockerRunArgs`).

2. **Add a doc comment above the new line / helper** explaining the always-on rationale:

   ```go
   // host.docker.internal lets containers reach the host machine.
   // Docker Desktop / OrbStack / Rancher Desktop on macOS auto-provide
   // this alias; raw Linux dockerd does not. --add-host is a no-op when
   // the alias already exists (last-writer-wins with the same value)
   // and a real fix on Linux, so emit it unconditionally.
   ```

3. **Add a Ginkgo spec** to the existing argv-shape test file (find via `grep -l 'BuildDockerRunArgs' pkg/executor/*_test.go`). The spec MUST assert presence — the returned argv slice contains the exact string `--add-host=host.docker.internal:host-gateway`. Do NOT pin position in the assertion; argv ordering isn't part of this prompt's contract — unconditional emission is.

   Example shape (match the existing test conventions — use `launchpolicy.ContainerLaunchOpts{}` to mirror the rest of the file, not the `executor.ContainerLaunchOpts` alias):

   ```go
   It("includes --add-host=host.docker.internal:host-gateway in the docker run argv", func() {
       opts := launchpolicy.ContainerLaunchOpts{
           ContainerName:  "df-test",
           ProjectName:    "test-project",
           ContainerImage: "busybox:latest",
       }
       args := executor.BuildDockerRunArgs(opts)
       Expect(args).To(ContainElement("--add-host=host.docker.internal:host-gateway"))
   })
   ```

4. **Existing tests must still pass.** Run `make test` (or `make precommit` if available; check `Makefile`) and verify zero regressions. Any pre-existing argv-shape spec that asserts the FULL argv with a hardcoded slice will need its expected slice updated to include the new flag — fix those mechanically.

5. **CHANGELOG entry.** Append a bullet to `## Unreleased` in `/workspace/CHANGELOG.md`:

   ```markdown
   - **feat(executor): always emit `--add-host=host.docker.internal:host-gateway` in container argv.** Docker Desktop / OrbStack / Rancher Desktop on macOS auto-provide this alias; raw Linux dockerd does not. Unblocks Linux operators from routing dark-factory containers through a host-side service (e.g. `claude-code-router` per [claude-code-router/docs/dark-factory-integration.md](https://github.com/bborbe/claude-code-router/blob/master/docs/dark-factory-integration.md)). No-op on Docker runtimes that already provide the alias.
   ```

   If a `## Unreleased` section already exists, append below its last bullet; if absent, create it directly above the most recent `## vX.Y.Z` header.

6. **Run `make precommit`** in the repo root. Fix any lint / format / addlicense issues. Build must succeed and all tests must pass.

</requirements>

<constraints>

- **No new `opts` field.** Keep the change to the argv builder only. No struct surface area change → no downstream consumer needs to opt in.
- **Unconditional emission, no OS dispatch.** No `runtime.GOOS` check. No new interface, no factory pattern. Per the operator's design decision (option (a) from the task description), KISS wins; if a second host-detected flag ever appears, refactor to strategy then.
- **No new dependencies.** All needed stdlib already imported.
- **Backward-compatible by definition.** Docker overwrites identical `--add-host` aliases as a no-op on runtimes that already provide them; raw Linux gains the missing alias. No operator action required after the upgrade.
- **Do NOT commit.** dark-factory handles git.
- **Do NOT add a new test file unless the existing test file structure demands it.** Add the spec to the file that already tests `BuildDockerRunArgs`.

</constraints>

<verification>

```bash
cd /workspace
make precommit
```

Must pass. Additionally:

```bash
cd /workspace
grep -n "add-host\|host-gateway" pkg/executor/launch.go
```

Expect: at least one match in `BuildDockerRunArgs` (or a new helper called from it). The literal substring `host.docker.internal:host-gateway` should appear in the production code.

```bash
cd /workspace
go test -run 'TestSuite|add.host' ./pkg/executor/... -v -count=1 2>&1 | tail -30
```

Expect: the new alias-presence spec passes, plus any existing argv-shape specs continue to pass (with updated expected slices if they were hardcoded).

</verification>
