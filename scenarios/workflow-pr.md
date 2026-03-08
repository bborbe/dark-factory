# Scenario: PR workflow creates worktree, opens PR, cleans up

Test repo: `~/Documents/workspaces/dark-factory-sandbox`

## Setup

- [ ] Repo has `workflow: pr` in `.dark-factory.yaml`
- [ ] `GH_TOKEN` set
- [ ] No stale worktrees from previous runs (`git worktree list`)

## Action

- [ ] Create `prompts/toggle-comment.md` with content that toggles `// dark-factory-sandbox: scenario test marker` in `math_abs.go` (add if missing, remove if present)
- [ ] `dark-factory prompt approve toggle-comment`
- [ ] Start dark-factory: `dark-factory run`

## Expected

- [ ] Worktree created during execution at `../dark-factory-sandbox-*`
- [ ] Feature branch `dark-factory/*` pushed
- [ ] PR opened on GitHub
- [ ] Worktree removed after completion
- [ ] Prompt status is `completed`
- [ ] Master branch has no new commits (changes are on the feature branch only)
