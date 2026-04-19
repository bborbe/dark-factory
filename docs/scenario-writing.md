# Scenario Writing Guide

Scenarios are end-to-end acceptance tests that define what success looks like from the outside. They come before specs in the workflow: Task → Scenario → Spec → Prompts. They live in the project's `scenarios/` directory.

## Status Lifecycle

```
idea → draft → active → outdated
```

| Status | Meaning | When |
|--------|---------|------|
| `idea` | Planned — title + what it proves, no steps | Feature not yet built |
| `draft` | Being written — steps exist but not yet validated | During/after implementation |
| `active` | Verified, part of regression suite | After first successful run |
| `outdated` | Feature changed/removed, no longer applies | Keep for history, skip during runs |

All scenarios use explicit frontmatter: `status: idea`, `draft`, `active`, or `outdated`. No `failing` status — that's a run result, not a lifecycle state.

## When to Write a Scenario

| Situation | Scenario needed? |
|-----------|-----------------|
| New workflow or mode | Yes |
| Spec with 3+ acceptance criteria | Yes |
| Bug fix for regression | Yes — prevents recurrence |
| Config change, version bump | No |
| Internal refactor (same behavior) | No — existing scenarios cover it |

## Format

```markdown
---
status: draft
---

# Scenario NNN: [what this proves in one line]

Validates that [one sentence describing what aspect this tests].

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

## Cleanup
Teardown steps.
```

Description line right after the title — one sentence starting with "Validates that...". Three core sections: Setup → Action → Expected. Optional Cleanup section. Each item is a checkbox.

## Writing Rules

**1. Observable outcomes only.** Test what a human can see — files on disk, git state, CLI output, HTTP responses. Never test internal structs or function calls.

**2. Self-contained.** Each scenario sets up its own preconditions. No dependency on another scenario having run first.

**2a. Test the code under change, not a stale binary.** When writing scenarios that cover dark-factory itself (meta-scenarios in the dark-factory repo), invoke it via `go run` against the source so scenarios exercise current, uninstalled code — otherwise a passing scenario proves nothing about the changes you're about to ship. When writing scenarios for an unrelated project that uses dark-factory, the installed `dark-factory` binary is the right target.

**3. One journey per file.** Don't combine happy path and failure path — split them into separate scenarios.

**4. Number files sequentially.** `001-workflow-direct.md`, `002-workflow-pr.md`, `003-smoke-test-container.md`. Numbers provide stable ordering and reference.

**5. Keep it short.** A scenario with 20+ checkboxes is too large. Split it.

## Example

```markdown
---
status: active
---

# Scenario 002: PR workflow creates branch, opens PR, cleans up

## Setup
- [ ] Git repo with at least one commit
- [ ] `.dark-factory.yaml` with `workflow: pr`
- [ ] One prompt in inbox

## Action
- [ ] Approve the prompt: `dark-factory prompt approve my-feature`
- [ ] Start dark-factory: `dark-factory run`
- [ ] Wait for processing to complete

## Expected
- [ ] Feature branch `dark-factory/*` pushed to remote
- [ ] PR opened on GitHub
- [ ] Prompt status is `completed`
- [ ] Prompt moved to `prompts/completed/`
- [ ] Master branch has no new commits
- [ ] Log exists at `prompts/log/NNN-my-feature.log`

## Cleanup
- Remove temp directory
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
- **Scenarios come first** — write the scenario before the spec
- **Use a temp copy** for destructive scenarios — never run against your working repo
- **Schedule scenario runs** after every 10-15 prompts or after completing a spec

## Location

```
your-project/
├── scenarios/
│   ├── 001-workflow-direct.md      (active)
│   ├── 002-workflow-pr.md          (active)
│   ├── 003-smoke-test-container.md (active)
│   ├── 004-custom-config-dirs.md   (idea)
│   └── ...
├── specs/
└── prompts/
```
