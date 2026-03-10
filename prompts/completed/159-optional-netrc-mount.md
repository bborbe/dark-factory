---
status: completed
summary: Added netrcFile config field with read-only container mount, bumped default image to v0.2.8, and updated all related tests
container: dark-factory-159-optional-netrc-mount
dark-factory-version: v0.33.1
created: "2026-03-10T12:02:40Z"
queued: "2026-03-10T12:02:40Z"
started: "2026-03-10T12:02:48Z"
completed: "2026-03-10T12:13:14Z"
---

<summary>
- Projects can configure a credentials file for authenticating to private git hosts
- When configured, the container can resolve private Go modules during dependency verification
- Works with any HTTPS-authenticated git host (Bitbucket Server, GitLab, Gitea, etc.)
- Existing projects without the setting continue to work unchanged
- Default container image updated to v0.2.8 (adds Bitbucket Server network access)
</summary>

<objective>
Add a `netrcFile` field to `.dark-factory.yaml` so the YOLO container can authenticate to private git hosts via `.netrc`. When configured, the file is mounted read-only at `/home/node/.netrc`. This enables `go mod tidy` / `go mod verify` for projects with private modules on Bitbucket Server, GitLab, or any HTTPS-authenticated git host.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/config/config.go` — `Config` struct (line ~39), `Defaults()` (line ~62), `Validate()` (line ~93), `resolveEnvVar()` (line ~171, handles `${VAR}` curly-brace form only).
Read `pkg/config/loader.go` — `partialConfig` struct (line ~53) and `mergePartial()` (line ~109). IMPORTANT: any new Config field MUST also be added to partialConfig and mergePartial, otherwise the YAML value is silently dropped (this bug just happened with `defaultBranch` in v0.33.0).
Read `pkg/executor/executor.go` — `Execute()` (line ~108) is the only caller of `buildDockerCommand()` (line ~182). The netrcFile value must flow: `Config.NetrcFile` → `dockerExecutor` struct → `Execute()` → `buildDockerCommand()`.
Read `pkg/executor/executor_internal_test.go` — existing tests for `buildDockerCommand`.
Read `pkg/config/config_test.go` — existing config validation and loader tests.
</context>

<requirements>
1. Add `NetrcFile string \`yaml:"netrcFile"\`` field to `Config` struct in `pkg/config/config.go`. No default value (empty string means no mount).

2. Add `NetrcFile *string \`yaml:"netrcFile"\`` to `partialConfig` in `pkg/config/loader.go`.

3. Add merge logic in `mergePartial()` in `pkg/config/loader.go`:
   ```go
   if partial.NetrcFile != nil {
       cfg.NetrcFile = *partial.NetrcFile
   }
   ```

4. Add config validation in `Validate()` in `pkg/config/config.go`: if `NetrcFile` is non-empty, expand `${VAR}` references using `resolveEnvVar()`, then check `os.Stat()` — return error if file does not exist. Use the existing `validation.Name()` pattern.

5. Add `netrcFile string` field to `dockerExecutor` struct in `pkg/executor/executor.go`. Update `NewDockerExecutor()` to accept and store it.

6. Update `buildDockerCommand()` in `pkg/executor/executor.go` to use the struct field. When non-empty, add volume mount after existing mounts, before `e.containerImage`:
   ```go
   if e.netrcFile != "" {
       args = append(args, "-v", e.netrcFile+":/home/node/.netrc:ro")
   }
   ```

7. Update `Execute()` — no change needed if `buildDockerCommand()` reads from `e.netrcFile` struct field.

8. Update all callers of `NewDockerExecutor()` in `pkg/factory/factory.go` to pass `cfg.NetrcFile`.

9. Update default container image from `v0.2.7` to `v0.2.8` in `Defaults()` in `pkg/config/config.go`. Update all test references in `pkg/config/config_test.go` accordingly (replace all occurrences of `v0.2.7` with `v0.2.8`).

10. Add tests:
    - Config validation: netrcFile pointing to existing file passes; netrcFile pointing to nonexistent file fails with clear error
    - Loader: config with `netrcFile: /tmp/test.netrc` loads correctly; config without netrcFile leaves it empty
    - Executor: `buildDockerCommand` with netrcFile includes `-v ...:ro` mount; without netrcFile has no .netrc mount
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Mount MUST be `:ro` (read-only) — container must never modify the netrc file
- Do NOT hardcode any path — use only the configured value
- Do NOT mount anything when netrcFile is empty (default behavior unchanged)
- The netrcFile path supports `${HOME}` expansion (curly-brace form, consistent with existing `resolveEnvVar()`)
</constraints>

<verification>
```
make precommit
```
Must pass with no errors.
</verification>

<success_criteria>
- `netrcFile` field exists in Config struct, partialConfig, and mergePartial
- Empty netrcFile = no mount (default, backwards compatible)
- Configured netrcFile = read-only mount at /home/node/.netrc
- Nonexistent netrcFile path fails config validation with clear error
- All existing tests pass
- New tests cover configured, unconfigured, and validation error paths
</success_criteria>
