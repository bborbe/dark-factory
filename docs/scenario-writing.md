# Scenario Writing Guide

Scenarios are end-to-end checklists that prove a feature works as specified. They live in the project's `scenarios/` directory.

## When to Write a Scenario

| Situation | Scenario needed? |
|-----------|-----------------|
| New workflow or mode | Yes |
| Spec with 3+ acceptance criteria | Yes |
| Bug fix for regression | Yes — prevents recurrence |
| Config change, version bump | No |
| Internal refactor (same behavior) | No — existing scenarios cover it |

## Format

Three sections only: Setup → Action → Expected.

```markdown
# Scenario: [what this proves in one line]

## Setup
- [ ] Precondition 1
- [ ] Precondition 2

## Action
- [ ] Step 1
- [ ] Step 2

## Expected
- [ ] Observable outcome 1
- [ ] Observable outcome 2
- [ ] Observable outcome 3
```

## Writing Rules

**1. Observable outcomes only.** Test what a human can see — files on disk, git state, CLI output, HTTP responses. Never test internal structs or function calls.

**2. Self-contained.** Each scenario sets up its own preconditions. No dependency on another scenario having run first.

**3. One journey per file.** Don't combine happy path and failure path — split them into separate scenarios.

**4. Name files for the journey.** `workflow-pr.md`, `config-invalid-rejected.md`, `multi-prompt-ordering.md`. No numbers — order doesn't matter for manual execution.

**5. Keep it short.** A scenario with 20+ checkboxes is too large. Split it.

## Example

```markdown
# Scenario: PR workflow creates branch, opens PR, cleans up

## Setup
- [ ] Git repo with at least one commit
- [ ] `.dark-factory.yaml` with `workflow: pr`
- [ ] One prompt in inbox

## Action
- [ ] Approve the prompt: `dark-factory prompt approve my-feature`
- [ ] Start daemon: `dark-factory daemon`
- [ ] Wait for processing to complete

## Expected
- [ ] Feature branch `dark-factory/*` pushed to remote
- [ ] PR opened on GitHub
- [ ] Prompt status is `completed`
- [ ] Prompt moved to `prompts/completed/`
- [ ] Master branch has no new commits
- [ ] Log exists at `prompts/log/NNN-my-feature.log`
```

## Running a Scenario

1. Open the scenario file
2. Walk through **Setup** — ensure all preconditions are met
3. Execute the **Action** steps
4. Verify every **Expected** checkbox
5. All checked = pass. Any unchecked = fail

**When to run:**
- After completing a spec (before marking it done)
- After major refactors (run all scenarios)
- When in doubt about a change

## Best Practices

- **Scenarios accumulate** — new feature = new scenario, never delete passing ones
- **Scenarios complement specs** — spec says what to build, scenario proves it works
- **Use a temp copy** for destructive scenarios — never run against your working repo
- **Schedule scenario runs** after every 10-15 prompts or after completing a spec

## Location

```
your-project/
├── scenarios/
│   ├── workflow-direct.md
│   ├── workflow-pr.md
│   ├── failure-recovery.md
│   └── multi-prompt-ordering.md
├── specs/
└── prompts/
```
