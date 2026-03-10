<summary>
- Projects can configure a custom git configuration for the YOLO container
- Enables URL rewrites so Go modules use HTTPS instead of SSH for private hosts
- Combines with credentials file support for full private module access (URL rewrite + auth)
- The container can write back to the mounted file (e.g. to record proxy settings)
- Existing projects without the setting continue to work unchanged
</summary>

<objective>
Projects with private Go dependencies that require git URL rewrites need a way to provide custom git configuration to the YOLO container. Add optional gitconfig file support to dark-factory so the container can use per-project git configuration, including URL rewrites for HTTPS-based module resolution. The mount must be writable because the container appends its own settings at startup.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/config/config.go` — `Config` struct (line ~39), `Defaults()` (line ~62), `Validate()` (line ~93), `resolveEnvVar()` (line ~182, handles `${VAR}` curly-brace form only).
Read `pkg/config/loader.go` — `partialConfig` struct (line ~53) and `mergePartial()` (line ~109). CRITICAL: any new Config field MUST also be added to partialConfig and mergePartial, otherwise the YAML value is silently dropped.
Read `pkg/executor/executor.go` — `dockerExecutor` struct (line ~29), `NewDockerExecutor()` (line ~38), `buildDockerCommand()` (line ~185). See existing `netrcFile` pattern — gitconfigFile follows the same approach but WITHOUT `:ro`.
Read `pkg/executor/executor_internal_test.go` — existing tests for `buildDockerCommand`, including netrcFile tests.
Read `pkg/config/config_test.go` — existing config validation and loader tests.
</context>

<requirements>
1. Add `GitconfigFile string \`yaml:"gitconfigFile"\`` field to `Config` struct in `pkg/config/config.go`. No default value (empty string means no mount).

2. Add `GitconfigFile *string \`yaml:"gitconfigFile"\`` to `partialConfig` in `pkg/config/loader.go`.

3. Add merge logic in `mergePartial()` in `pkg/config/loader.go`:
   ```go
   if partial.GitconfigFile != nil {
       cfg.GitconfigFile = *partial.GitconfigFile
   }
   ```

4. Add config validation in `Validate()` in `pkg/config/config.go`: if `GitconfigFile` is non-empty, resolve the path using `resolveFilePath()` (see requirement 4a), then check `os.Stat()` — return error if file does not exist. Follow the existing `netrcFile` validation pattern but use the new resolver.

4a. Add a `resolveFilePath(value string) string` helper in `pkg/config/config.go` that: (1) expands `${VAR}` via the existing `resolveEnvVar()`, (2) expands leading `~/` to `os.UserHomeDir()`. This is needed because `os.Stat("~/.foo")` returns not-found — Go and Docker do not expand tilde. Update the existing `netrcFile` validation to also use `resolveFilePath()` instead of bare `resolveEnvVar()`.

5. Add `gitconfigFile string` field to `dockerExecutor` struct in `pkg/executor/executor.go`. Update `NewDockerExecutor()` to accept and store it.

6. Update `buildDockerCommand()` in `pkg/executor/executor.go`: the `buildDockerCommand` method receives `home string` as a parameter. When `e.gitconfigFile` is non-empty, resolve tilde (`~/` → `home+"/"`), then add volume mount after the netrcFile mount (if any), before `e.containerImage`. NO `:ro` flag — the mount must be writable because the container entrypoint writes to this file:
   ```go
   if e.gitconfigFile != "" {
       resolved := e.gitconfigFile
       if strings.HasPrefix(resolved, "~/") {
           resolved = home + resolved[1:]
       }
       args = append(args, "-v", resolved+":/home/node/.gitconfig")
   }
   ```
   Apply the same tilde resolution to the existing `netrcFile` mount (line ~211-213) — it currently passes the raw path to docker, which does not expand `~`.

7. Update all callers of `NewDockerExecutor()` in `pkg/factory/factory.go` to pass `cfg.GitconfigFile`. There are two call sites:
   - `CreateSpecGenerator()` (~line 186): add `cfg.GitconfigFile` to the `NewDockerExecutor()` call
   - `CreateProcessor()` (~line 249): add `gitconfigFile string` parameter to `CreateProcessor`'s signature (~line 220), pass it to `NewDockerExecutor()` at line ~249, and update both callers of `CreateProcessor` — in `CreateRunner()` (~line 102) and `CreateOneShotRunner()` (~line 156) — to pass `cfg.GitconfigFile`

8. Add tests:
   - Config validation: gitconfigFile pointing to existing file passes; gitconfigFile pointing to nonexistent file fails with clear error
   - Loader: config with `gitconfigFile: /tmp/test.gitconfig` loads correctly; config without gitconfigFile leaves it empty
   - Executor: `buildDockerCommand` with gitconfigFile includes `-v ...:/home/node/.gitconfig` mount (no :ro); without gitconfigFile has no .gitconfig mount
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Mount must NOT be `:ro` — the container entrypoint appends proxy settings via `git config --global`
- Do NOT hardcode any path — use only the configured value
- Do NOT mount anything when gitconfigFile is empty (default behavior unchanged)
- The gitconfigFile path supports tilde expansion (`~/` → home directory) and `${VAR}` expansion (entire-string only, via `resolveEnvVar()`). Typical usage: `~/.claude-yolo/.gitconfig`
</constraints>

<verification>
```
make precommit
```
Must pass with no errors.
</verification>

<success_criteria>
- `gitconfigFile` field exists in Config struct, partialConfig, and mergePartial
- Empty gitconfigFile = no mount (default, backwards compatible)
- Configured gitconfigFile = writable mount at /home/node/.gitconfig (no :ro)
- Nonexistent gitconfigFile path fails config validation with clear error
- All existing tests pass
- New tests cover configured, unconfigured, and validation error paths
</success_criteria>
