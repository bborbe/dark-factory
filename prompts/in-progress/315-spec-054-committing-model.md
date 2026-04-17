---
status: executing
spec: [054-committing-status-git-retry]
container: dark-factory-315-spec-054-committing-model
dark-factory-version: v0.122.0-6-g6b02e84
created: "2026-04-17T14:00:00Z"
queued: "2026-04-17T14:18:31Z"
started: "2026-04-17T14:18:33Z"
branch: dark-factory/committing-status-git-retry
---

<summary>
- A new `committing` prompt status is added between `executing` and `completed`
- Prompts with `committing` status remain in `prompts/in-progress/` (they are not queued for re-execution)
- A `MarkCommitting()` method is available on `PromptFile` to transition to this status
- `ListQueued` skips files with `committing` status, just as it skips `executing` and `completed`
- A `FindCommitting()` package-level function scans a directory and returns paths of all files with `committing` status
- All existing tests continue to pass
</summary>

<objective>
Introduce `committing` as a first-class prompt status in `pkg/prompt/prompt.go`. This status represents a prompt that has completed container execution successfully but whose git commit is still pending. It is the precondition for prompt 2 (git retry + recovery) and prompt 3 (status display). No orchestration logic is implemented here — only the status type, the PromptFile method, and the directory scanner.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-enum-type-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Key files to read before editing:
- `pkg/prompt/prompt.go` — status constants (lines 49–107), `AvailablePromptStatuses` (line 76), `MarkCompleted()` / `MarkApproved()` methods (near line 357), `ListQueued` function (lines 632–686), `ResetExecuting` function (lines 688+)
- `pkg/prompt/prompt_test.go` — existing tests for status constants and `ListQueued`
</context>

<requirements>

## 1. Add `CommittingPromptStatus` constant

In `pkg/prompt/prompt.go`, add after the `CancelledPromptStatus` constant (currently the last one, around line 72):

```go
// CommittingPromptStatus indicates the container succeeded but the git commit is still pending.
// The prompt stays in in-progress/ until the commit succeeds.
CommittingPromptStatus PromptStatus = "committing"
```

## 2. Add to `AvailablePromptStatuses`

In the `AvailablePromptStatuses` variable (lines 76–86), append `CommittingPromptStatus` as the last entry:

```go
var AvailablePromptStatuses = PromptStatuses{
    IdeaPromptStatus,
    DraftPromptStatus,
    ApprovedPromptStatus,
    ExecutingPromptStatus,
    CompletedPromptStatus,
    FailedPromptStatus,
    InReviewPromptStatus,
    PendingVerificationPromptStatus,
    CancelledPromptStatus,
    CommittingPromptStatus,  // <-- add this
}
```

## 3. Add `MarkCommitting()` method to `PromptFile`

Locate the existing `MarkCompleted()` and `MarkFailed()` methods (around lines 357–367). Add `MarkCommitting()` nearby:

```go
// MarkCommitting sets the status to "committing" — container succeeded, awaiting git commit.
func (pf *PromptFile) MarkCommitting() {
	pf.Frontmatter.Status = string(CommittingPromptStatus)
}
```

## 4. Update `ListQueued` to skip `committing` status

In the `ListQueued` function (around line 660), the skip condition currently is:

```go
if fm.Status == string(ExecutingPromptStatus) ||
    fm.Status == string(CompletedPromptStatus) ||
    fm.Status == string(FailedPromptStatus) ||
    fm.Status == string(InReviewPromptStatus) ||
    fm.Status == string(PendingVerificationPromptStatus) {
```

Add `CommittingPromptStatus` to this list:

```go
if fm.Status == string(ExecutingPromptStatus) ||
    fm.Status == string(CommittingPromptStatus) ||
    fm.Status == string(CompletedPromptStatus) ||
    fm.Status == string(FailedPromptStatus) ||
    fm.Status == string(InReviewPromptStatus) ||
    fm.Status == string(PendingVerificationPromptStatus) {
```

## 5. Add `FindCommitting` package-level function

After `ListQueued` (or near `ResetExecuting`), add a new package-level function that scans a directory for prompt files in `committing` status:

```go
// FindCommitting returns the paths of all .md files in dir whose status is "committing".
// Files that cannot be read are skipped with a warning.
func FindCommitting(
    ctx context.Context,
    dir string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) ([]string, error) {
    entries, err := os.ReadDir(dir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, nil
        }
        return nil, errors.Wrap(ctx, err, "read directory")
    }

    var paths []string
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
            continue
        }
        path := filepath.Join(dir, entry.Name())
        fm, err := readFrontmatter(ctx, path, currentDateTimeGetter)
        if err != nil {
            slog.Warn("skipping prompt in FindCommitting", "file", entry.Name(), "error", err)
            continue
        }
        if fm.Status == string(CommittingPromptStatus) {
            paths = append(paths, path)
        }
    }
    return paths, nil
}
```

Note: `readFrontmatter` is a private function already used by `ListQueued` — use it here too.

## 6. Update `Manager.FindCommitting` method

Add the corresponding method on `*Manager` that delegates to the package-level function:

```go
// FindCommitting returns paths of all prompt files in in-progress/ with status "committing".
func (pm *Manager) FindCommitting(ctx context.Context) ([]string, error) {
    return FindCommitting(ctx, pm.inProgressDir, pm.currentDateTimeGetter)
}
```

Place this near the other `Manager` delegation methods (e.g., near `ListQueued` on `*Manager`).

## 7. Write tests

In `pkg/prompt/prompt_test.go`, add tests for the new functionality. Follow existing patterns (Ginkgo/Gomega, external test package `prompt_test`):

### Test 7a: `CommittingPromptStatus` is in `AvailablePromptStatuses`

```go
It("CommittingPromptStatus is in AvailablePromptStatuses", func() {
    Expect(prompt.AvailablePromptStatuses.Contains(prompt.CommittingPromptStatus)).To(BeTrue())
})
```

### Test 7b: `MarkCommitting` sets correct status

```go
It("MarkCommitting sets status to committing", func() {
    pf := prompt.NewPromptFile("test.md", prompt.Frontmatter{Status: "executing"}, nil, libtime.NewCurrentDateTimeGetter())
    pf.MarkCommitting()
    Expect(pf.Frontmatter.Status).To(Equal("committing"))
})
```

### Test 7c: `ListQueued` skips committing files

Add a test case in the existing `ListQueued` Describe block (alongside the existing `executing`, `completed` skip tests) that creates a temp `.md` file with `status: committing` in the scanned dir and verifies it is not returned by `ListQueued`.

### Test 7d: `FindCommitting` returns committing files

Write a test that:
1. Creates a temp directory
2. Creates two `.md` files: one with `status: committing`, one with `status: approved`
3. Calls `FindCommitting(ctx, dir, currentDateTimeGetter)`
4. Expects only the `committing` file path to be returned

## 8. Write CHANGELOG entry

Add an `## Unreleased` section at the top of `CHANGELOG.md` (above the latest versioned section) if it does not exist, then append:

```
- feat: add `committing` prompt status for git-persistence phase between container exit and completed
```

## 9. Run `make test`

```bash
cd /workspace && make test
```

Must pass before proceeding to `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- `committing` is an internal status — do NOT add it to any user-facing CLI or documentation in this prompt (that is prompt 3's job)
- All 9 existing prompt statuses must remain valid — this only ADDS a new one
- Use `errors.Wrapf` / `errors.Wrap` from `github.com/bborbe/errors` for all error wrapping (no `fmt.Errorf`)
- External test package (`package prompt_test`) — do not use internal test package
- Follow the `go-enum-type-pattern.md` for the new status constant (GoDoc comment, `PromptStatus` type)
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `grep -n "CommittingPromptStatus" pkg/prompt/prompt.go` — at least 3 matches (const, AvailablePromptStatuses, MarkCommitting)
2. `grep -n "committing" pkg/prompt/prompt.go` — covers ListQueued skip condition and FindCommitting
3. `go test ./pkg/prompt/...` — passes
</verification>
