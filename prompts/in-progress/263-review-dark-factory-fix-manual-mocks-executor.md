---
status: approved
created: "2026-04-06T00:00:00Z"
queued: "2026-04-06T15:24:28Z"
---

<summary>
- The executor package's internal test file defines two manual stub implementations
- Manual stubs drift from the real interface when new methods are added, causing silent breakage
- The project standard is to use counterfeiter-generated mocks exclusively
- The fix moves the tests to an external test package and replaces manual stubs with generated mocks
- An export_test.go file exposes any unexported functions that need testing
</summary>

<objective>
Replace the manually-written `fakeCommandRunner` and `multiFailRunner` stubs in `pkg/executor/executor_internal_test.go` with counterfeiter-generated mocks, moving tests to the external `package executor_test` using an `export_test.go` bridge file for any unexported identifiers under test.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` for the counterfeiter pattern.

Files to read before making changes (read ALL first):
- `pkg/executor/executor_internal_test.go` — the full file; understand which unexported functions or types are tested that require internal access
- `pkg/executor/executor.go` — the `commandRunner` interface; confirm its method signatures
- `pkg/runner/export_test.go` — the existing export_test.go pattern to follow
- `mocks/` directory — check what executor-related mocks already exist
</context>

<requirements>
1. Read `pkg/executor/executor_internal_test.go` fully to understand:
   a. Which unexported identifiers (`commandRunner`, internal functions) are accessed.
   b. What behavior the `fakeCommandRunner` and `multiFailRunner` stubs provide (what methods, what they return).

2. Add a `//counterfeiter:generate` directive on the `commandRunner` interface in `pkg/executor/executor.go`:
   ```go
   //counterfeiter:generate -o ../../mocks/command-runner.go --fake-name CommandRunner . commandRunner
   ```
   Run `make generate` to produce `mocks/command_runner.go`.

3. Create `pkg/executor/export_test.go` (package `executor`) to expose any unexported identifiers needed by tests:
   ```go
   package executor
   // expose unexported types or vars needed by tests
   var NewDockerExecutorForTest = newDockerExecutor // if an unexported constructor exists
   ```

4. Rewrite the tests from `executor_internal_test.go` in a new or updated `pkg/executor/executor_test.go` using `package executor_test` and `mocks.CommandRunner`:
   - Replace `fakeCommandRunner` with `mocks.NewCommandRunner()` and configure its `Run` stub via `.RunStub`.
   - Replace `multiFailRunner` with a `mocks.CommandRunner` that uses `RunCalls` to fail N times then succeed.

5. Delete `pkg/executor/executor_internal_test.go` once all its test cases have been migrated to the external test package.

6. Ensure `make generate` is run before `make test`.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- All existing test behavior must be preserved — do not remove test cases, only change how stubs are implemented
- Use counterfeiter mocks from `mocks/` package — never hand-write stubs
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
