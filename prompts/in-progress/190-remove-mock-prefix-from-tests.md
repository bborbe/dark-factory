---
status: approved
created: "2026-03-11T17:44:51Z"
queued: "2026-03-11T18:25:03Z"
---

<summary>
- Test variable names follow the project convention — no `mock` prefix on Counterfeiter fakes
- Variable names like `mockExecutor` become `executor`, `mockManager` becomes `manager`
- Consistent naming across all 15+ test files
- Both `var` declarations and all usages are renamed within each file
- Import conflicts (e.g., `processor` variable vs `processor` package) are handled with disambiguated names
</summary>

<objective>
Remove the `mock` prefix from all Counterfeiter fake variable names in test files. The project convention is to name fakes after the type they implement (e.g., `executor` not `mockExecutor`), since the Counterfeiter-generated type already signals it is a test double.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read each test file before editing. The pattern to fix is:
```go
// Old:
mockExecutor = new(mocks.Executor)
// New:
executor = new(mocks.Executor)
```
Rename both the declaration and all usages within the same file.
</context>

<requirements>
1. In each of the following test files, rename all `mock`-prefixed variables by removing the `mock` prefix (lowercase the first letter of the remaining name):

   - `pkg/processor/processor_test.go`: `mockExecutor` → `executor`, `mockManager` → `manager`, `mockReleaser` → `releaser`, `mockVersionGet` → `versionGet`, `mockBrancher` → `brancher`, `mockPRCreator` → `prCreator`, `mockCloner` → `cloner`, `mockPRMerger` → `prMerger`, `mockAutoCompleter` → `autoCompleter`, `mockSpecLister` → `specLister`
   - `pkg/runner/runner_test.go`: `mockManager` → `manager`, `mockLocker` → `locker`, `mockWatcher` → `watcher`, `mockProcessor` → `processor`, `mockServer` → `server`
   - `pkg/runner/oneshot_test.go`: `mockManager` → `manager`, `mockLocker` → `locker`, `mockProcessor` → `processor`
   - `pkg/cmd/status_test.go`: `mockChecker` → `checker`, `mockFormatter` → `formatter`
   - `pkg/cmd/approve_test.go`: `mockPromptManager` → `promptManager`
   - `pkg/cmd/prompt_verify_test.go`: `mockPromptManager` → `promptManager`, `mockReleaser` → `releaser`, `mockBrancher` → `brancher`, `mockPRCreator` → `prCreator`
   - `pkg/cmd/combined_list_test.go`: `mockLister` → `lister`, `mockCounter` → `counter`
   - `pkg/cmd/combined_status_test.go`: `mockChecker` → `checker`, `mockFormatter` → `formatter`, `mockLister` → `lister`, `mockCounter` → `counter`
   - `pkg/cmd/spec_show_test.go`: `mockCounter` → `counter`
   - `pkg/cmd/spec_status_test.go`: `mockLister` → `lister`, `mockCounter` → `counter`
   - `pkg/cmd/spec_list_test.go`: `mockLister` → `lister`, `mockCounter` → `counter`
   - `pkg/server/server_test.go`: `mockStatusChecker` → `statusChecker`
   - `pkg/server/queue_action_handler_test.go`: `mockPromptManager` → `promptManager`
   - `pkg/generator/generator_test.go`: `mockExecutor` → `executor`
   - `pkg/git/collaborator_fetcher_test.go`: `mockRepoNameFetcher` → `repoNameFetcher`, `mockCollaboratorLister` → `collaboratorLister`

2. For each variable, rename both the `var` declaration and ALL usages throughout the file. Use find-and-replace within each file.

3. If a renamed variable conflicts with a package import (e.g., `processor` variable vs `processor` package), use a disambiguated name like `proc` for the variable.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Do not change any test logic — only rename variables.
- Handle import conflicts carefully — check if the test file imports a package with the same name as the new variable.
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
