---
status: created
spec: [054-committing-status-git-retry]
created: "2026-04-17T14:00:00Z"
branch: dark-factory/committing-status-git-retry
---

<summary>
- `dark-factory status` shows `committing` prompts with a distinct label, not as idle/queued/executing/completed
- A new `CommittingPrompts` field in the `Status` struct lists prompt filenames in the `committing` state
- The `docs/prompt-writing.md` status lifecycle table includes a `committing` row
- The `docs/architecture-flow.md` prompt status diagram includes `committing` between `executing` and `completed`
</summary>

<objective>
Make the `committing` status visible in `dark-factory status` output and update the two lifecycle documentation files. This is a display-only and documentation-only prompt — no orchestration logic.

**Precondition:** Prompts 1 and 2 have been executed. `CommittingPromptStatus`, `Manager.FindCommitting()`, and the processor recovery are all in place.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Key files to read before editing:
- `pkg/status/status.go` — `Status` struct (lines 27–56), `GetStatus()` (lines 128–189), `populateExecutingPrompt()` (lines 383–412), `PromptManager` interface in the status package (search for `type PromptManager interface` in `pkg/status/`)
- `docs/prompt-writing.md` — the status table at approximately lines 228–234
- `docs/architecture-flow.md` — the prompt status lifecycle diagram at approximately lines 119–125
</context>

<requirements>

## 1. Add `CommittingPrompts` to the `Status` struct

In `pkg/status/status.go`, add a new field to the `Status` struct after `QueuedPrompts`:

```go
CommittingPrompts []string `json:"committing_prompts,omitempty"`
CommittingCount   int      `json:"committing_count,omitempty"`
```

## 2. Expose `FindCommitting` on the `PromptManager` interface in `pkg/status/`

Find the `PromptManager` interface used in `pkg/status/status.go` (search for `type PromptManager interface` in `pkg/status/`). Add `FindCommitting`:

```go
FindCommitting(ctx context.Context) ([]string, error)
```

If the interface is in a separate file (e.g., `pkg/status/interfaces.go`), add it there. The `*prompt.Manager` concrete type already implements this method (added in prompt 1).

## 3. Add `populateCommittingPrompts` method

Add a new private method to the `checker` struct:

```go
// populateCommittingPrompts populates CommittingPrompts and CommittingCount in the status.
func (s *checker) populateCommittingPrompts(ctx context.Context, st *Status) error {
    paths, err := s.promptMgr.FindCommitting(ctx)
    if err != nil {
        return errors.Wrap(ctx, err, "find committing prompts")
    }
    for _, p := range paths {
        st.CommittingPrompts = append(st.CommittingPrompts, filepath.Base(p))
    }
    st.CommittingCount = len(paths)
    return nil
}
```

## 4. Call `populateCommittingPrompts` from `GetStatus`

In `GetStatus()`, call `populateCommittingPrompts` after the existing `populateExecutingPrompt` call (and before or after `populateGeneratingSpec` — order does not matter):

```go
// Check for committing prompts (container succeeded, git commit pending)
if err := s.populateCommittingPrompts(ctx, status); err != nil {
    return nil, errors.Wrap(ctx, err, "populate committing prompts")
}
```

## 5. Update `docs/prompt-writing.md`

Find the status table (approximately lines 228–234). It currently looks like:

```markdown
| `idea` | `prompts/ideas/` | Rough concept, needs refinement | Human creates file |
| `draft` | `prompts/` | Complete, ready for review and approval | Human/AI creates file |
| `approved` | `prompts/in-progress/` | Queued for execution | `dark-factory prompt approve` |
| `executing` | `prompts/in-progress/` | YOLO container running | Auto (dark-factory) |
| `completed` | `prompts/completed/` | Done, archived | Auto (dark-factory) |
| `failed` | `prompts/in-progress/` | Needs fix or retry | Auto (dark-factory) |
| `cancelled` | `prompts/in-progress/` | Pulled before/during execution | `dark-factory prompt cancel <id>` |
```

Insert a new row after `executing`:

```markdown
| `committing` | `prompts/in-progress/` | Container succeeded, git commit pending | Auto (dark-factory) |
```

## 6. Update `docs/architecture-flow.md`

Find the prompt status lifecycle diagram (approximately lines 119–125):

```
created → approved → executing → completed
                         │            │
                         └── failed   └── (moved to completed/)
                         │
                         └── partial (validationPrompt criteria unmet)
```

Update it to include `committing` between `executing` and `completed`:

```
created → approved → executing → committing → completed
                         │            │             │
                         └── failed   └── (retry)   └── (moved to completed/)
                         │
                         └── partial (validationPrompt criteria unmet)
```

Also update the directory structure comment at approximately line 149:

```
│   ├── in-progress/
│   │   └── 001-my-change.md    # Queue (status: approved/executing/committing)
```

Change `approved/executing` to `approved/executing/committing`.

## 7. Regenerate mocks if needed

If the `PromptManager` interface in `pkg/status/` has a corresponding mock (check `mocks/` directory for a `status-prompt-manager.go` or similar), regenerate it:

```bash
cd /workspace && go generate ./pkg/status/...
```

OR manually add `FindCommitting` to the relevant mock. Verify with:
```bash
cd /workspace && go build ./...
```

## 8. Write tests

In `pkg/status/status_test.go` (or a relevant test file in `pkg/status/`), add a test for `populateCommittingPrompts`:

1. Set up a mock `PromptManager` that returns `[]string{"/some/path/001-foo.md"}` from `FindCommitting`
2. Call `GetStatus(ctx)`
3. Expect `status.CommittingPrompts` to equal `[]string{"001-foo.md"}`
4. Expect `status.CommittingCount` to equal `1`

Also add a test that `FindCommitting` returning an empty slice results in `CommittingCount == 0` and `CommittingPrompts` being nil or empty.

Follow the existing test patterns in the status package (check how `populateExecutingPrompt` is tested).

## 9. Write CHANGELOG entry

Append to `CHANGELOG.md` under `## Unreleased`:

```
- feat: `dark-factory status` displays `committing` prompts with count and filenames
- docs: add `committing` status to prompt-writing.md lifecycle table and architecture-flow.md diagram
```

## 10. Run `make test`

```bash
cd /workspace && make test
```

Must pass before `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- `committing` is an internal status — it is shown in status output but there is no CLI command to set it manually
- Use `errors.Wrap` from `github.com/bborbe/errors` for all error wrapping
- `CommittingPrompts` must initialize to `nil` (not `[]string{}`) when empty, consistent with other optional status fields
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `grep -n "CommittingPrompts\|CommittingCount" pkg/status/status.go` — at least 2 matches
2. `grep -n "populateCommittingPrompts" pkg/status/status.go` — at least 2 matches (definition + call)
3. `grep -n "committing" docs/prompt-writing.md` — at least 1 match
4. `grep -n "committing" docs/architecture-flow.md` — at least 1 match
5. `go test ./pkg/status/...` — passes
</verification>
