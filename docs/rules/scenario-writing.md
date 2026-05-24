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

### Test pyramid framework

| Layer | Frequency | Scope | When to write |
|---|---|---|---|
| Unit | nearly always | single function/method on isolated data | all new business logic, algorithms, edge cases, error states; mocks fine |
| Integration | selectively | two or more components, or one component against a real out-of-process dep | repository ↔ DB, client ↔ external API, module-to-module wiring; one happy path + one error path is usually enough |
| End-to-end (scenario) | sparingly | full system as a user would exercise it | critical happy-path user journeys; smoke tests for production-like wiring |

**Directive: push tests as far down the pyramid as possible.** Default is unit. Reach for integration only when a real out-of-process dependency or a contract between modules is involved. Reach for a scenario only when the bottom two layers genuinely cannot exercise the behavior.

### Why scenarios are scarce

Scenarios are slow, brittle, and expensive. Adding one per spec inverts the pyramid: 100 specs → 100 E2E tests → CI takes hours, flaky failures everywhere, every fix is an archaeological dig because E2E tests are bad at root-cause analysis. Most specs are satisfied by unit and integration tests in the implementation prompt.

Scenarios exist for the rare cases where neither unit nor integration tests can exercise the real path: real Docker container behavior, real git remote, real GitHub API rendering, real multi-service orchestration. Use sparingly.

### Default: no new scenario

A spec does NOT need a new scenario when any of these are true:

- The behavior can be exercised by unit tests (per-function correctness)
- The behavior can be exercised by an integration test in the package (real `git` binary, in-process HTTP handler, fake clock — anything you can wire up locally without a sandbox repo or external service)
- An existing scenario already covers the same code path
- The change is a pure refactor, doc edit, error wording, struct-field addition with no runtime consumer, config bump, or version bump
- The spec author and reviewer agree the operator's first-deploy smoke check is sufficient (informal manual verification, not committed)

### Add a scenario only when ALL of these hold

1. **Unit and integration tests genuinely cannot reach the behavior.** Real Docker container output, real `gh pr view` rendering, real `kubectlquant` cluster state — things that need a real external system, not a test double.
2. **The behavior is load-bearing for an essential user journey.** PR opens correctly end-to-end, daemon starts on a green tree, prompt completes the clone workflow without crashing. Not "every config field that flows to runtime."
3. **No existing scenario covers it.** Reuse before adding.
4. **You can name the regression risk.** "If this breaks at runtime and no scenario catches it, an operator hits an `exit 128` for the second time" — concrete, specific. Not "in case something breaks."

If you can't tick all four, do NOT add a scenario. The unit and integration tests in the prompt are sufficient.

### Canonical examples — when a scenario IS justified

- **Spec 068** — clone workflow crashed at runtime with `exit 128` after the clone was deleted. Unit tests passed. The bug was a control-flow ordering issue in the daemon's post-commit pipeline that no test double could catch. Scenario locks it down.
- **Spec 015** — a new Kafka `CommandOperation` constant passed struct-shape tests but was rejected at runtime by the cqrs regex. Real publish through the dev cluster was the only way to surface this.
- **Spec 055** — config field wiring dropped by the loader. Unit tests on the field passed; production didn't see it.

Each one: load-bearing, runtime-only failure mode, no test double can fake the boundary.

### Counter-examples — when a scenario is NOT justified

- A new public method on a struct, with a unit test asserting its return value.
- A new config field whose handler is unit-tested and whose effect is also unit-tested.
- An additional `slog.Info` log line.
- A refactor that splits one function into two; behavior unchanged.
- A bug fix where the original failure was caught by a unit test that simply hadn't existed before — write the unit test, no scenario needed.

If the test pyramid says it should be unit or integration, write it as unit or integration. Don't reach for a scenario because "this touches an integration seam."

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

**2a. Test the code under change, not a stale binary.** When writing scenarios that cover dark-factory itself (meta-scenarios in the dark-factory repo), build a fresh binary in Setup — `go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .` — then invoke `/tmp/new-dark-factory <cmd>` throughout. Plain `go run` does not work because scenarios `cd` into a sandbox with a different go.mod. When writing scenarios for an unrelated project that uses dark-factory, the installed `dark-factory` binary is the right target.

**3. One journey per file.** Don't combine happy path and failure path — split them into separate scenarios.

**4. Number files sequentially.** `001-workflow-direct.md`, `002-workflow-pr.md`, `003-smoke-test-container.md`. Numbers provide stable ordering and reference.

**5. Keep it short.** A scenario with 20+ checkboxes is too large. Split it.

**6. Factor reusable setup into building-block helpers, not full auto-runners.** When the same setup recurs across scenarios — preparing a content directory, starting a server, building a fresh binary — extract it into `scenarios/helper/<verb>-<noun>.sh` (e.g. `setup-content-dir.sh`, `start-http-server.sh`, `stop-server.sh`, `probe-stdio.sh`). The scenario's Setup block then reads one line per helper: `bash scenarios/helper/setup-X.sh`. Action and Expected blocks stay in markdown — assertions are usually scenario-specific, no point factoring them out. Reach for a full `run-NNN-all.sh` auto-runner only when you have ~10+ scenarios and walking by hand has become the bottleneck. Premature auto-runners hide what's actually being tested and lock in orchestration that may not fit later scenarios.

## Example

```markdown
---
status: active
---

# Scenario 002: PR workflow creates branch, opens PR, cleans up

## Setup
- [ ] Git repo with at least one commit
- [ ] `.dark-factory.yaml` with `workflow: clone` and `pr: true`
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
