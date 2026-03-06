---
status: completed
---

# PR Workflow: Branch, Push, Create Pull Request

## Problem

Direct workflow commits to the current branch. For team projects or when review is desired, changes should go through pull requests instead of direct commits.

## Goal

An alternative workflow where each prompt's changes are committed to a feature branch, pushed, and a pull request is created via GitHub CLI. The factory then returns to the original branch for the next prompt.

## Non-goals

- No PR approval polling (fire-and-forget)
- No automatic merge after approval
- No PR template customization
- No support for non-GitHub remotes (GitLab, Bitbucket)
- No combining multiple prompts into one PR

## Desired Behavior

1. When `workflow: pr` is configured:
2. Before execution, save current branch name
3. Create and switch to feature branch: `dark-factory/NNN-slug` (derived from prompt filename)
4. Execute Docker container (makes changes on feature branch)
5. `git add -A`
6. `git commit -m "<prompt title>"`
7. `git push -u origin dark-factory/NNN-slug`
8. `gh pr create --title "<prompt title>" --body "Automated by dark-factory"`
9. Switch back to original branch
10. Move prompt to completed, continue with next

## Constraints

- Requires `gh` CLI installed and authenticated
- Branch naming: `dark-factory/NNN-slug` (matches prompt filename)
- One PR per prompt (no batching)
- Original branch restored even if PR creation fails

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `gh` not installed | Error during PR creation, changes still committed and pushed | Install gh, create PR manually |
| Branch already exists | Error during branch creation | Delete old branch or rename prompt |
| Push fails (permissions) | Changes committed locally but not pushed | Manual push |
| Checkout back fails | Log error; factory may be on wrong branch | Manual `git checkout` |

## Acceptance Criteria

- [ ] Feature branch created with `dark-factory/NNN-slug` naming
- [ ] Changes committed to feature branch, not original branch
- [ ] Branch pushed to remote
- [ ] Pull request created via `gh pr create`
- [ ] Original branch restored after PR creation
- [ ] Processing continues with next prompt on original branch

## Verification

Run `make precommit` — must pass.

## Do-Nothing Option

Use direct workflow only. Works for solo projects but doesn't support team review workflows.
