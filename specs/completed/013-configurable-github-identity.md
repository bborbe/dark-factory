---
status: completed
---

# Configurable GitHub Identity via GH_TOKEN

## Problem

Dark-factory creates PRs using the current user's `gh` auth. When pr-reviewer reviews under the same user, GitHub rejects it as self-review. To enable autonomous PR creation + review, dark-factory and pr-reviewer need distinct GitHub identities.

## Goal

After completion, dark-factory supports an optional `github.token` config field (referencing an env var). When set, all `gh` CLI calls use that token, so PRs are created under a bot identity. Combined with spec 004 of pr-reviewer, this gives each tool its own GitHub identity.

## Non-goals

- Managing or creating GitHub accounts/apps
- Token rotation
- Per-prompt or per-repo identity (one global identity is sufficient)
- Changing git commit author (only affects `gh` API calls)

## Desired Behavior

### Config

```yaml
# .dark-factory.yaml
github:
  token: ${DARK_FACTORY_TOKEN}  # env var with PAT from bot account

workflow: pr
autoMerge: true
```

When `github.token` is absent or empty, behavior is unchanged (uses default `gh` auth).

### Env Var Resolution

Same pattern as pr-reviewer spec 004:
- `${VAR}` → `os.Getenv("VAR")`
- Empty/unset → fall back to default `gh` auth, log warning
- Literal string without `${}` → used directly

### Affected gh Calls

All `gh` subprocess calls in `pkg/git/`:
- `pr_creator.go` — `gh pr create`
- `pr_merger.go` — `gh pr view`, `gh pr merge` (from spec 013)
- `brancher.go` — `gh repo view` for `DefaultBranch` (from spec 013)

Each sets `cmd.Env` to include `GH_TOKEN=<resolved-token>` when configured.

## Constraints

- Backward compatible — existing configs without `github` work unchanged
- Token never in logs or error messages
- Token never as CLI argument (only via `cmd.Env`)
- Does NOT affect `git` commands (push auth comes from git credential helper, not `gh`)

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `github.token` references unset env var | Fall back to default `gh` auth, log warning | Set env var or remove config |
| Token invalid (401) | `gh` error propagated | Check token |
| Token lacks PR permissions | `gh` error propagated | Check token scopes |

## Acceptance Criteria

- [ ] `github.token: ${DARK_FACTORY_TOKEN}` resolves env var
- [ ] `gh pr create` uses configured token
- [ ] `gh pr view` and `gh pr merge` use configured token
- [ ] `gh repo view` uses configured token
- [ ] Missing env var falls back to default auth with warning
- [ ] Existing configs without `github` work unchanged
- [ ] Token never in log output
- [ ] `make precommit` passes

## Verification

```
make precommit
```

## Do-Nothing Option

Export `GH_TOKEN` in the shell before running dark-factory. Works but couples identity to the shell session rather than the project config.
