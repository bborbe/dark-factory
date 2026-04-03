---
status: completed
summary: Added os.ExpandEnv to extraMounts src path resolution, added tests for $VAR/${VAR}/undefined expansion, updated docs and example config
container: dark-factory-244-env-var-expansion-in-extra-mounts
dark-factory-version: v0.93.0
created: "2026-04-03T00:00:00Z"
queued: "2026-04-03T12:26:47Z"
started: "2026-04-03T12:32:11Z"
completed: "2026-04-03T12:39:33Z"
---

<summary>
- Extra mount `src` paths now expand environment variables like `$GOPATH`, `$HOME`, `${VAR}`
- This allows portable mount configurations that work across different machines and setups
- Expansion happens before tilde expansion and path resolution, using the same priority: env vars → tilde → relative path
- Missing or empty env vars result in the literal string remaining, which will fail the `os.Stat` check and be skipped with a warning
</summary>

<objective>
Add environment variable expansion to `extraMounts` `src` paths so users can write `$GOPATH/pkg` instead of hardcoding absolute paths. Uses `os.ExpandEnv` — no shell execution, no security risk.
</objective>

<context>
Read CLAUDE.md for project conventions.

Key file: `pkg/executor/executor.go` — the `buildDockerCommand` method (~line 330) processes `extraMounts` at lines 365-381. Current path resolution order:
1. Tilde expansion: `~/` → `home + "/"`
2. Relative path resolution: non-absolute → `filepath.Join(projectRoot, src)`
3. Existence check: `os.Stat(src)` — missing paths logged and skipped

The fix adds env var expansion as step 0, before tilde expansion.

Also read:
- `pkg/executor/executor_internal_test.go` — existing tests for extraMounts (~line 500+)
- `pkg/config/config.go` — `ExtraMount` struct (~line 66), `IsReadonly()` (~line 73)
- `docs/configuration.md` — Extra Mounts section (~line 222)
</context>

<requirements>
1. **Add `os.ExpandEnv(src)` in `pkg/executor/executor.go`**:

   In `buildDockerCommand`, immediately after `src := m.Src` (line 366), add env var expansion before the existing tilde check:

   ```go
   src := m.Src
   src = os.ExpandEnv(src)
   if strings.HasPrefix(src, "~/") {
   ```

   This is one line. `os.ExpandEnv` replaces `$VAR` and `${VAR}` with their values. Undefined vars become empty string.

2. **Add tests in `pkg/executor/executor_internal_test.go`**:

   Add test cases in the existing extraMounts test section:

   - `src: "$HOME/docs"` with `HOME=/Users/test` → resolves to `/Users/test/docs`
   - `src: "${GOPATH}/pkg"` with `GOPATH=/opt/go` → resolves to `/opt/go/pkg`
   - `src: "$UNDEFINED_VAR/docs"` → resolves to `/docs`, fails `os.Stat`, skipped with warning
   - `src: "~/docs"` still works (tilde after env expansion)
   - `src: "$HOME"` alone → resolves to home dir

   The test file uses Ginkgo/Gomega (`It()`, `Expect()`), not stdlib `*testing.T`. Use `os.Setenv`/`os.Unsetenv` with `DeferCleanup()` to manage env vars. Follow the existing test patterns in the file.

3. **Update `docs/configuration.md`**:

   In the Extra Mounts section (~line 222), update the `src` field description in the table:

   ```
   | `src` | yes | — | Host path. Environment variables (`$VAR`, `${VAR}`) are expanded. `~/` expanded to home. Relative paths resolved from project root. |
   ```

   Add examples after the existing YAML block showing real-world use cases:

   ```yaml
   # Go module cache (uses GOPATH env var)
   extraMounts:
     - src: ${GOPATH}/pkg
       dst: /home/node/go/pkg

   # Python uv cache
   extraMounts:
     - src: ~/.cache/uv
       dst: /home/node/.cache/uv
   ```

4. **Update example `.dark-factory.yaml`**:

   In `example/.dark-factory.yaml`, replace `extraMounts: []` with a real example:

   ```yaml
   extraMounts:
     - src: ${GOPATH}/pkg
       dst: /home/node/go/pkg
   ```
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- All existing tests must pass
- `make precommit` must pass
- Use `os.ExpandEnv` (stdlib) — no shell execution, no `exec.Command`
- Env expansion must happen BEFORE tilde expansion (so `$HOME` expands but `~/` still works independently)
- Use `github.com/bborbe/errors` for error wrapping (never `fmt.Errorf`)
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm os.ExpandEnv is used
grep -n "ExpandEnv" pkg/executor/executor.go

# Confirm tests exist
grep -n "GOPATH\|ExpandEnv\|HOME.*docs" pkg/executor/executor_internal_test.go

# Confirm docs updated
grep -n "environment\|\\$VAR\|\\$GOPATH" docs/configuration.md
```
</verification>
