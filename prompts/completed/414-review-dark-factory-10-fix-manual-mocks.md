---
status: completed
summary: Replaced 6 manual test fakes with Counterfeiter-generated mocks in pkg/preflightconditions/conditions_test.go and pkg/containerslot/manager_test.go
container: dark-factory-exec-414-review-dark-factory-10-fix-manual-mocks
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T14:51:20Z"
started: "2026-05-25T14:55:56Z"
completed: "2026-05-25T14:59:31Z"
---

<summary>
- Replaced 3 manual fake types with Counterfeiter-generated mocks in pkg/preflightconditions/conditions_test.go
- Replaced 3 manual fake types with Counterfeiter-generated mocks in pkg/containerslot/manager_test.go
- Generated mocks using go generate
</summary>

<objective>
Replace manual test fakes with Counterfeiter-generated mocks in two test files.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-testing-guide.md` for Counterfeiter mock patterns.

Files to read before making changes:
- `pkg/preflightconditions/conditions_test.go` — lines 18-46, manual fakes: fakePreflightChecker, fakeGitLockChecker, fakeDirtyFileChecker
- `pkg/containerslot/manager_test.go` — lines 20-97, manual fakes: stubContainerCounter, fakeContainerLock, fakeContainerChecker
- `pkg/preflight/preflight.go` — line 18 counterfeiter directive
- `pkg/processor/gitlock.go` — line 12 counterfeiter directive
- `pkg/processor/dirty.go` — line 15 counterfeiter directive
- `pkg/executor/checker.go` — lines 19, 82 counterfeiter directives
- `pkg/containerlock/containerlock.go` — line 21 counterfeiter directive
</context>

<requirements>
1. In `pkg/preflightconditions/conditions_test.go`:
   - Run `go generate ./pkg/preflightconditions/...` to generate mocks
   - Replace `fakePreflightChecker` with `&mocks.PreflightChecker{}`
   - Replace `fakeGitLockChecker` with `&mocks.GitLockChecker{}`
   - Replace `fakeDirtyFileChecker` with `&mocks.DirtyFileChecker{}`
   - Set up mock return values using `.CheckReturns()`, `.ExistsReturns()`, `.CountDirtyFilesReturns()`

2. In `pkg/containerslot/manager_test.go`:
   - Run `go generate ./pkg/containerslot/...` to generate mocks
   - Replace `stubContainerCounter` with `&mocks.ContainerCounter{}`
   - Replace `fakeContainerLock` with `&mocks.ContainerLock{}`
   - Replace `fakeContainerChecker` with `&mocks.ContainerChecker{}`
   - Set up mock return values using the appropriate Returns methods

3. Remove the manual fake type definitions from both files.
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
- Counterfeiter mocks must be used instead of manual fakes
</constraints>

<verification>
make precommit
</verification>
