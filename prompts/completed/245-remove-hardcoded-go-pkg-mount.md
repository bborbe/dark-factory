---
status: completed
summary: Removed hardcoded Go pkg cache mount from executor, updated tests and documentation
container: dark-factory-245-remove-hardcoded-go-pkg-mount
dark-factory-version: v0.94.0
created: "2026-04-03T00:00:00Z"
queued: "2026-04-03T12:48:40Z"
started: "2026-04-03T12:52:17Z"
completed: "2026-04-03T12:59:01Z"
---

<summary>
- The hardcoded `$HOME/go/pkg:/home/node/go/pkg` Docker volume mount is removed from the executor
- Users who need the Go module cache mounted must add it explicitly via `extraMounts` in `.dark-factory.yaml`
- This fixes the duplicate mount error when an extraMount targets `/home/node/go/pkg`
- The hardcoded mount was wrong for users with custom GOPATH anyway
- Documentation and example config are updated to show the recommended extraMount
</summary>

<objective>
Remove the hardcoded Go package cache mount from the Docker executor. The mount assumed `$HOME/go/pkg` which is incorrect for users with custom GOPATH. Users now configure this explicitly via `extraMounts` with `${GOPATH}/pkg`, which resolves correctly via env var expansion.
</objective>

<context>
Read CLAUDE.md for project conventions.

Key files:
- `pkg/executor/executor.go` — `buildDockerCommand()` at ~line 330. Line 149-150 logs the mount, line 349 adds the hardcoded `-v home+"/go/pkg:/home/node/go/pkg"` flag. Remove both.
- `pkg/executor/executor_internal_test.go` — line 280 asserts the mount exists (`ContainElement("/home/user/go/pkg:/home/node/go/pkg")`). This assertion must be removed.
- `docs/configuration.md` — Extra Mounts section (~line 232). Add a migration note.
- `example/.dark-factory.yaml` — already has `${GOPATH}/pkg` extraMount from previous prompt.

Problem: when a user adds `extraMounts: [{src: "${GOPATH}/pkg", dst: "/home/node/go/pkg"}]`, Docker fails with "Duplicate mount point: /home/node/go/pkg" because the hardcoded mount already targets the same destination.
</context>

<requirements>
1. **Remove hardcoded mount from `pkg/executor/executor.go`**:

   Remove the `-v` line at ~line 349:
   ```go
   "-v", home+"/go/pkg:/home/node/go/pkg",
   ```

   Also remove the debug log at ~line 149-150:
   ```go
   "goPkgMount", home+"/go/pkg:/home/node/go/pkg")
   ```

   Keep the other three standard mounts (prompt file, workspace, claude config).

2. **Update tests in `pkg/executor/executor_internal_test.go`**:

   Remove the assertion at ~line 280:
   ```go
   Expect(cmd.Args).To(ContainElement("/home/user/go/pkg:/home/node/go/pkg"))
   ```

   Update any mount count assertions that expect 4 standard mounts — they should now expect 3.

3. **Update `docs/configuration.md`**:

   In the Extra Mounts section, add a note about the Go module cache:

   ```markdown
   **Go module cache:** The Go module cache is no longer mounted by default. Add it explicitly if your project uses Go:

   ```yaml
   extraMounts:
     - src: ${GOPATH}/pkg
       dst: /home/node/go/pkg
   ```
   ```

4. **Update CHANGELOG.md** with a breaking change note:

   ```
   - **breaking:** Remove hardcoded `$HOME/go/pkg` mount — add `extraMounts` with `${GOPATH}/pkg` for Go projects
   ```
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- All existing tests must pass
- `make precommit` must pass
- Use `github.com/bborbe/errors` for error wrapping (never `fmt.Errorf`)
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm hardcoded mount is gone
grep -n "go/pkg" pkg/executor/executor.go
# Should return 0 lines

# Confirm test updated
grep -n "go/pkg" pkg/executor/executor_internal_test.go
# Should only show extraMount test lines, not the old hardcoded assertion
```
</verification>
