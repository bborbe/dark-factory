# Fix go-git push authentication

## Goal

`gitPush()` and `gitPushTag()` in `pkg/git/git.go` fail with "authentication required: No anonymous write access" because go-git doesn't use the system git credential helper. The old subprocess `git push` worked because it inherited the system credential config.

## Current Behavior (BUG)

```go
func gitPush(ctx context.Context) error {
    repo, err := gogit.PlainOpen(".")
    err = repo.Push(&gogit.PushOptions{})  // FAILS: no auth
}
```

go-git requires explicit auth in `PushOptions`. The system `git` command reads `~/.gitconfig` credential helpers automatically.

## Option A: Fall back to subprocess for push only

Keep go-git for local operations (add, commit, tag) but use `exec.Command("git", "push")` for network operations. This is the simplest fix — push is the only operation that needs credentials.

```go
func gitPush(ctx context.Context) error {
    cmd := exec.CommandContext(ctx, "git", "push")
    cmd.Dir = "."
    return cmd.Run()
}
```

## Option B: Use go-git with credential helper

Read the git credential helper from config and pipe through it:
- Parse `~/.gitconfig` for `credential.helper`
- Execute the helper to get username/password
- Pass as `http.BasicAuth` in `PushOptions`

This is more complex and fragile.

## Recommendation

**Option A** — subprocess for push only. Simple, reliable, uses existing credentials. The reason for go-git was better tag handling and avoiding `git describe` bugs — push doesn't benefit from go-git.

## Implementation

1. Replace `gitPush()` and `gitPushTag()` in `pkg/git/git.go` with `exec.CommandContext`
2. Keep all other functions using go-git (add, commit, tag, getNextVersion)
3. Update tests if needed

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Coverage ≥80% for changed packages
