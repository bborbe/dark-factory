---
status: completed
summary: Added git health warnings (GitIndexLock, DirtyFileCount, DirtyFileThreshold) to dark-factory status output — Status struct, GetStatus population, formatter warnings section, NewChecker signature updated, all callers updated, tests added
container: dark-factory-257-status-show-warnings
dark-factory-version: v0.102.1-dirty
created: "2026-04-06T09:54:07Z"
queued: "2026-04-06T09:54:07Z"
started: "2026-04-06T09:54:12Z"
completed: "2026-04-06T10:09:45Z"
---

<summary>
- Status output shows a warning when .git/index.lock exists, alerting the operator that prompts will be skipped
- Status output shows the number of dirty files in the working tree when any exist
- Status output shows a warning when dirty file count exceeds the configured threshold
- Warnings appear in a dedicated section between daemon info and queue count
- JSON output includes the same information as structured fields
- No warnings shown when everything is clean (no visual noise)
</summary>

<objective>
Add git health warnings to `dark-factory status` output so operators can diagnose why the daemon is skipping prompts without reading logs. The daemon already checks for `.git/index.lock` and dirty file threshold but this is invisible to the user running `dark-factory status`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/status/status.go` — find the `Status` struct, `Checker` interface, and `GetStatus` method.
Read `pkg/status/formatter.go` — find the `Format` method that builds human-readable output.
Read `pkg/status/status_test.go` for test patterns (Ginkgo/Gomega). Note: `pkg/status/formatter_test.go` does not exist yet and must be created — use `pkg/status/format_test.go` and `status_test.go` as pattern references.
Read `pkg/processor/processor.go` — find `checkGitIndexLock` and `checkDirtyFileThreshold` to understand how these checks work.
</context>

<requirements>
1. Add two new fields to the `Status` struct in `pkg/status/status.go`:
   - `GitIndexLock bool` with json tag `"git_index_lock,omitempty"`
   - `DirtyFileCount int` with json tag `"dirty_file_count,omitempty"`

2. In `GetStatus` in `pkg/status/status.go`, after the existing `populateLogInfo` call:
   - Check if `.git/index.lock` exists using `os.Stat(filepath.Join(s.projectDir, ".git", "index.lock"))`. If it exists, set `status.GitIndexLock = true`.
   - Count dirty files by running `git status --porcelain` (use `exec.CommandContext`) in `s.projectDir`, count non-empty lines. Set `status.DirtyFileCount` to the count. On error, log at debug level and leave at 0.

3. In `Format` in `pkg/status/formatter.go`, add a warnings section between the "Last log" line and the end of output. Only print this section when there are warnings:
   ```
     Warnings:
       ⚠ .git/index.lock exists — daemon will skip prompts
       ⚠ 42 dirty files (threshold: 30)
   ```
   - The `.git/index.lock` warning appears when `st.GitIndexLock` is true.
   - The dirty file warning appears when `st.DirtyFileCount > 0`. Show threshold only when there is a configured threshold. To show the threshold, add a `DirtyFileThreshold int` field to `Status` (json tag `"dirty_file_threshold,omitempty"`) and populate it from the checker.

4. Update `NewChecker` to accept `dirtyFileThreshold int` as a new parameter. Store it and use it to populate `status.DirtyFileThreshold`.

5. Update all callers of `NewChecker`. There are 3 in `pkg/factory/factory.go` (in `CreateStatusCommand`, `CreateServer`, and `CreateCombinedStatusCommand`) and 6 in `pkg/status/status_test.go`. Add the new `dirtyFileThreshold` parameter to each call — pass the config value where available, 0 otherwise.

6. Add tests in `pkg/status/status_test.go`:
   - Test that `GitIndexLock` is true when `.git/index.lock` file exists in projectDir.
   - Test that `DirtyFileCount` reflects actual dirty files.

7. Create `pkg/status/formatter_test.go` (new file — use `pkg/status/status_test.go` for Ginkgo/Gomega patterns). Add tests:
   - Test that no warnings section appears when both fields are zero/false.
   - Test that `.git/index.lock` warning appears when `GitIndexLock` is true.
   - Test that dirty file warning appears with count and threshold.
   - Test that dirty file warning appears without threshold when `DirtyFileThreshold` is 0.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- The `git status --porcelain` command must run in the project directory, not the current working directory
- Warnings section must only appear when there is at least one warning (no empty "Warnings:" header)
- JSON output must include the new fields automatically via the existing JSON encoder
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
