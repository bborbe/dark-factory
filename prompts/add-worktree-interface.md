# Add Worktree interface and WorkflowWorktree config

## Goal

Add a `Worktree` interface in `pkg/git/` and a `WorkflowWorktree` constant in `pkg/config/`, alongside existing code. Nothing changes for `direct` or `pr` workflows.

## Implementation

### 1. Add `WorkflowWorktree` to `pkg/config/workflow.go`

Add a new constant and include it in `AvailableWorkflows`:

```go
const (
    WorkflowDirect   Workflow = "direct"
    WorkflowPR       Workflow = "pr"
    WorkflowWorktree Workflow = "worktree"
)

var AvailableWorkflows = Workflows{WorkflowDirect, WorkflowPR, WorkflowWorktree}
```

### 2. Update README.md configuration docs

Add `worktree` as a valid workflow value in the configuration section.

### 3. Create `pkg/git/worktree.go`

Follow the same pattern as `pkg/git/brancher.go` (Interface → Constructor → Struct → Methods).

```go
// Worktree handles git worktree operations.
//
//counterfeiter:generate -o ../../mocks/worktree.go --fake-name Worktree . Worktree
type Worktree interface {
    Add(ctx context.Context, path string, branch string) error
    Remove(ctx context.Context, path string) error
}
```

Implementation uses `git worktree add <path> -b <branch>` and `git worktree remove <path>` via `exec.CommandContext`, same as `brancher.go`.

### 4. Generate counterfeiter mock

Run `go generate ./pkg/git/...` to create `mocks/worktree.go`.

### 5. Tests for `pkg/git/worktree.go`

Create `pkg/git/worktree_test.go` following `pkg/git/brancher_test.go` pattern (Ginkgo v2/Gomega). Test in a temp git repo:

- `Add` creates a worktree at the given path with the given branch
- `Add` returns error for invalid path
- `Remove` removes an existing worktree
- `Remove` returns error for non-existent worktree

### 6. Test for workflow validation

Add test case in `pkg/config/config_test.go` that `WorkflowWorktree` is valid:

```go
It("succeeds for worktree workflow", func() {
    cfg := config.Config{
        Workflow: config.WorkflowWorktree,
        // ... same as other valid config tests
    }
    err := cfg.Validate(ctx)
    Expect(err).NotTo(HaveOccurred())
})
```

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Coverage ≥80% for changed packages
- Follow existing patterns in `pkg/git/brancher.go` exactly
- Use `exec.CommandContext` for git commands (not go-git library) — same as brancher
