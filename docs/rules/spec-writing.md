# Spec Writing Guide

A spec is a behavioral contract for a feature or fix. It describes what the system should do, not how the code should look.

## When to Write a Spec

**This document does NOT decide direct vs prompt vs spec.** That decision lives in [../choosing-a-flow.md](../choosing-a-flow.md) — the single source of truth. Read it first to confirm a spec is the right flow.

In short: write a spec when the change carries a business-level "why" that deserves its own durable in-repo document, OR when it's a bug fix (`kind: bug`) where the reproduction + regression-lock deserves the spec format (see [../bug-workflow.md](../bug-workflow.md)). Otherwise prefer a prompt or direct edit — see the canonical decision doc.

## What a spec gives you

Once you've decided a spec is the right flow, it provides:

- **Acceptance criteria + a verification phase** (`/dark-factory:verify-spec`) that proves the feature or fix actually works at runtime — not just that `make precommit` passes. Prompts produce code; specs produce evidence.
- **Decomposition into prompts** — the daemon auto-generates prompts from the approved spec; you audit each before execution.
- **Bug-specific verification** — see [../bug-workflow.md](../bug-workflow.md) for the `kind: bug` extensions (reproduction section, regression-lock rules).

## Creating a Spec

Use the Claude Code command:

```text
/dark-factory:create-spec
```

Or create manually. Location depends on status:

| Status | Directory | Purpose |
|--------|-----------|---------|
| `idea` | `specs/ideas/` | Rough concepts, not ready for approval |
| `draft` | `specs/` (inbox) | Complete specs, ready for review and approval |

```bash
# Idea — park it for later
touch specs/ideas/my-feature.md

# Draft — ready for review
touch specs/my-feature.md
```

Use lowercase-kebab-case. Never number filenames — dark-factory assigns numbers on approve.

## Spec Structure

**Write the proof before the behavior.** Specs are authored top-down for readability (Summary → Problem → Goal → Non-goals), but the load-bearing pivot happens immediately after Non-goals: write the **Acceptance Criteria** (with evidence shape per AC) and the **Verification** commands *before* you write Desired Behavior. The proof anchors the rest of the spec — Desired Behavior is then what work has to happen for those checks to fire, not a free-standing wishlist that the AC pass tries to retrofit.

If you can't describe how you'd verify the goal — concretely, with an evidence shape — you don't yet know what the spec is asking for. Stop and answer the verification question before drafting behaviors. This is the cheapest moment to catch wrong-target specs: at write time, before the auditor sees them, before any prompt-creator wastes research cycles, before `spec-verifier` rejects the verification phase at the most expensive point in the pipeline.

The section order below reflects this: Acceptance Criteria and Verification sit immediately after Non-goals; Desired Behavior follows.

### Frontmatter

```yaml
---
status: draft
---
```

Only use `status`, `created`, and optionally `issue` (Jira key). Dark-factory adds the rest.

### Sections

Fill these sections, answering four questions:

1. **What is the end state?** → Goal section
2. **What must not change?** → Constraints section
3. **What can go wrong?** → Failure Modes section
4. **Should we do this at all?** → Do-Nothing Option

Fill the sections in this order — verification-first (proof anchors behavior):

- **Summary** — 3-5 bullet points, plain language, no code references
- **Problem** — one paragraph, why this matters
- **Goal** — the finished system, not the changes
- **Non-goals** — what this work will NOT do
- **Acceptance Criteria** — binary, testable checkboxes; each AC declares its evidence shape
- **Verification** — exact commands (`make precommit`)
- **Desired Behavior** — numbered observable outcomes (3-8) — *what makes the AC checks fire*
- **Constraints** — interfaces, tests, config format, behavior that must not change
- **Failure Modes** — table: trigger → expected behavior → recovery
- **Security / Abuse** — if HTTP, files, or user input involved
- **Suggested Decomposition** — required for multi-layer specs or > 5 Desired Behaviors
- **Do-Nothing Option** — cost of not doing this

**The ratio:** 70% what/why/constraints, 30% how.

### Reference Docs

When a spec needs technical detail (API endpoints, protocol formats) that would make it too implementation-level:

- Put it in `docs/` and reference from the spec
- **Spec** stays behavioral — what the system does
- **Doc** holds implementation context — API references, code examples
- **Prompts** reference both

## Scope Check

- **Desired behaviors > 8?** Look for a natural split
- **Desired Behaviors × Acceptance Criteria > 50?** The spec is multiplying its own surface — strong signal to split into 2-3 specs along natural seams (publisher fix vs new classifier vs new background goroutine, etc.).
- **Touches > 3 code layers** (e.g. publisher + classifier + CRD + sweeper + tests)? Same signal — each layer is its own concern. The prompt-creator will need to research each one independently; a single spec forces it to hold the whole graph in memory at once.
- **Two features with different do-nothing arguments?** Split into separate specs
- **Contains struct names or file paths that aren't frozen constraints?** Too implementation-level — push details to prompts

When in doubt, split. A 6-DB / 5-AC spec generates prompts in minutes; a 10-DB / 10-AC spec spent 30 min in research without writing on its first attempt (real example: agent zombie detection). The cost of splitting is one extra `approve` round; the cost of not splitting is open-ended.

## Suggested Decomposition (mandatory for multi-layer specs)

When a spec touches > 1 code layer or has > 5 Desired Behaviors, include a `## Suggested Decomposition` section enumerating how the work should split into prompts. This encodes the human's mental model so the prompt-creator doesn't have to re-derive it.

**Shape:**

```markdown
## Suggested Decomposition

Prompts should be generated in this order — each row is a single prompt with a clear scope.

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Doctrine-correct publishers | 1, 2 | 1-3 | — |
| 2 | Pod-state classifier | 3, 8, 9 | 4 | prompt 1 (uses publishers) |
| 3 | CRD knobs + admission validation | 5, 6 | 6, 8 | — |
| 4 | Deadline sweeper goroutine | 4, 7 | 5, 7 | prompts 1, 3 |
| 5 | envtest for ImagePullBackOff | — | 9 | prompt 2 |

Rationale: prompt 1 establishes the publishing contract; prompts 2 and 3 can run independently after; prompt 4 needs both; prompt 5 is a test-only addition on top.
```

**Rules:**

- One row per prompt — typically 3-6 total.
- Each row maps to specific DB / AC numbers so coverage is auditable.
- "Depends on" makes ordering explicit. The daemon executes prompts in filename order (`1-`, `2-`, …) — your decomposition determines that order.
- A spec without a Suggested Decomposition section is acceptable for single-layer single-behavior specs; the prompt-creator may produce a single prompt or split as it sees fit.
- The auditor flags multi-layer specs missing this section as Should-Fix.

## Evidence Shape per Acceptance Criterion

Every Acceptance Criterion must declare **what the verifier will observe to confirm pass.** This is what `spec-verifier` will demand — making it explicit in the AC lets the verifier be mechanical and lets you avoid the rewrite cycle.

### Acceptable evidence shapes

| Shape | Example phrasing in AC |
|---|---|
| **Exit code** | "`make precommit` exits 0" |
| **Stdout / stderr match** | "stdout contains `processed: 42`" |
| **Log line** | "log line at INFO level: `request_id=<uuid> status=ok`" |
| **File presence** | "`ls path/to/file` succeeds" |
| **File content (diff or grep)** | "`grep -n 'pattern' file.md` returns line ≥1" |
| **HTTP response** | "`GET /api/x` returns 200 with body matching `{...}`" |
| **Kafka message** | "topic `foo` receives one message with key `K` and payload matching `{...}`" |
| **Metric value (positive)** | "Prometheus counter `foo_total{label=x}` increments by 1" |
| **Metric value (delta)** | "Counter `bar_total{label=y}` increments by exactly N after action" |
| **Cluster state** | "`kubectl get pod X` returns Running with container ready=true" |
| **Vault / file artifact** | "task file under `tasks/` has frontmatter field `Y: Z`" |
| **State transition** | "frontmatter `status` transitions `prompted → verifying`" / "Jira ticket transitions `In Progress → Done`" / "BigQuery row count for table T increases by N" — captures the **delta** with before/after framing |
| **Negative evidence** | "`git diff path/to/file.go` is empty after action" / "`grep ERROR run.log` returns 0 lines" / "no kafka message on topic `Z` during the window" — captures the **absence** of an artifact |

### Negative ACs — write them, don't skip them

Specs that only assert positives miss invariants. For any AC of the form "X is NOT changed" / "Y is NOT logged" / "Z is NOT published" / "feature off ⇒ pre-spec behavior", declare a negative evidence shape: an explicit grep / diff / probe that returns zero results in the asserted-empty window.

- ❌ "config Y is not mutated" (how do you verify?)
- ✅ "`git diff config/Y.yaml` returns empty after the action — `git status` shows no modified files"
- ❌ "no extra errors are logged"
- ✅ "`grep -c ERROR run.log` returns 0 during the test window"

### Bad ACs (no evidence shape)

- ❌ "Unit test covers this" — that's the test plan, not the evidence
- ❌ "It works" / "Functionality verified" — narration
- ❌ "Tests pass" without naming what specific behavior is asserted
- ❌ "Correctness is established" — narration

### Good ACs (evidence shape declared)

- ✅ "After invocation, `cat tasks/<id>.md` shows `phase: completed` in frontmatter"
- ✅ "`kubectl -n dev logs <pod> | grep 'job spawned'` returns ≥1 match"
- ✅ "`gh pr view 2 --json reviews` lists one review with `state: APPROVED` by login `pr-review-of-ben`"

The point is not to inline test scripts — it's to make the AC's *observable target* unambiguous.

### Post-Deploy ACs — declare the deployment freshness check

Any AC that observes a *deployed* system (k8s pod, running daemon, live HTTP endpoint, in-cluster log) MUST mark itself with the prefix `**Post-Deploy (Rung-N):**` (N is 2 or 3 per the project's verification ladder) and declare two extra evidence-shape lines: `deploy_check:` and `deploy_target:`.

The `spec-verifier` agent runs Phase 0.5 before the AC walk: it executes every `deploy_check:`, compares stdout against the resolved `deploy_target:`, and refuses verification upfront if any environment is pre-fix. This catches stale deploys before the operator burns time on Phase 4 anti-evidence checks.

**Shape:**

```markdown
- [ ] **Post-Deploy (Rung-2):** the new gate fires in dev — evidence: `kubectlquant -n dev logs <executor-pod> --since=15m | grep spawn_suppressed` returns ≥1 line referencing the task id.
  - `deploy_check:` `kubectlquant -n dev get deploy/agent-task-executor -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `$(git rev-parse --short HEAD)`
```

**Rules:**

- The marker `**Post-Deploy (Rung-N):**` is positional — it MUST be the first token of the AC body. The verifier extracts on this exact substring.
- `deploy_check:` runs via `bash -lc` from the spec's host-repo root. It may `cd` internally but must not assume a specific CWD. Exit non-zero on failure.
- `deploy_target:` is treated as a literal string after `bash -c` expansion. Compare cleanly: prefer short SHAs (`git rev-parse --short HEAD`), exact semver tags, or substrings that the deploy-check command emits verbatim. The verifier does NOT do semver-aware comparison.
- One `deploy_check:` per environment. If a spec has Rung-2 (dev) AND Rung-3 (prod) ACs, write two ACs with two checks. Each AC is independently gated.
- If the spec's AC body mentions `kubectlquant`, `make buca`, `--version`, or any pod-image query but the marker is absent, the `spec-auditor` flags it as a spec-format violation. Add the marker and the two evidence lines.

**When NOT to use Post-Deploy markers:**

- Build-time ACs (`make precommit` exits 0, `grep` of source files, unit-test row names). These run in CI/local, not against a deployed system.
- File-presence / doc-content / CHANGELOG ACs. The artifact lives in the repo, not in a cluster.
- Scenario ACs that build a fresh binary in `/tmp/`. The scenario harness is its own freshness mechanism (see `docs/releasing-dark-factory.md`).

## Adversarial Laziness Test

Before finalizing, read your spec assuming the implementer intends the **laziest implementation that still passes every Acceptance Criterion.** Ask:

> If I tried to satisfy every AC with the minimum possible work, what would I do?

If the answer is a no-op, a hardcoded fake, or a trivial refactor that doesn't deliver the Goal — your ACs are under-specified.

### Examples

| AC | Laziest impl | Verdict |
|---|---|---|
| "File `docs/foo.md` exists" | `touch docs/foo.md` | Under-specified — needs minimum content / sections |
| "Unit test for X exists" | `func TestX(t *testing.T) {}` | Under-specified — needs assertion type |
| "Verdict is binary (approve / request-changes)" | always return `approve` | Under-specified — needs per-input-class assertion |
| "On 403, escalate to human_review" | hardcoded escalation | Adequately specified — escalation is the observable |

### Fix pattern

Replace artifact-existence ACs with behavior ACs. Instead of "doc X exists," write "doc X contains section Y with content matching Z."

## Hedge Words to Avoid

Specs that defer decisions to implementation time create unbounded interpretation surface. The auditor will flag these; pre-empt the rework.

**Words to scrutinise for deferrals** (flag only when they defer a decision — see distinction below): `should`, `appropriate`, `reasonable`, `as needed`, `where applicable`, `if necessary`, `proper`, `correct`, `sensible`, `suitable`, `relevant`, `adequately`, `sufficiently`, `etc.`, `and so on`, `among others`

### Flag only deferrals, not descriptive English

These same words are legitimate when describing existing state or identifying artifacts. Flag only when the word **defers a decision** the implementer would otherwise make.

**Flag (deferral):**
- ❌ "the agent **should** retry appropriately" — defers retry policy
- ❌ "use a **reasonable** timeout" — defers timeout value
- ❌ "log **relevant** fields" — defers field list

**Don't flag (descriptive):**
- ✅ "the daemon **should** be running when the cursor is read" — state assumption
- ✅ "the **relevant** config file" — identifying an existing artifact
- ✅ "the **correct** review state" — describing the expected outcome

For each hit, ask: "Does this word leave a decision, or does it describe state?" If decision → flag.

### Resolution rules (when flagged)

Each flagged hedge must either:

1. **Resolve to a concrete rule** — "appropriate retry policy" → "retry once with 5s + jitter backoff"
2. **Be explicitly marked "agent decides at impl time"** — acceptable ONLY when truly local AND reversible:
   - ✅ "log level for the new debug statement (INFO vs DEBUG) — agent decides at impl time"
   - ✅ "exact error message wording — agent decides at impl time"
   - ❌ "retry policy — agent decides at impl time" (cross-cutting, affects external contract)
   - ❌ "schema field name — agent decides at impl time" (persistent state, irreversible)

## Failure Modes — Optional Columns for Non-Trivial Specs

The minimum table is `Trigger | Expected behavior | Recovery`. For specs touching network I/O, persistent state, external systems, or shared resources, prefer the extended format.

| Column | Add when… | Question it answers |
|---|---|---|
| **Detection** | Spec has async / out-of-band failure modes (kafka publish, background job, timeout) | How does the operator know the failure occurred? |
| **Reversibility** | Spec writes to external state (GitHub PR, kafka topic, database) | Is the failure reversible / irreversible / partial? Irreversible failures demand stronger pre-checks. |
| **Concurrency** | Spec touches shared state (cursor file, lock, frontmatter status) | What if two instances do this simultaneously, or one crashes mid-action? |

**Don't add all four columns blanket** — that's bureaucracy. Only when the dimension is real for this spec.

### Categories to check (at least once each for non-trivial specs)

For specs that add real-world side effects, the Failure Modes section should cover at least one row per:

- External system unavailability
- Schema / version drift
- Partial progress / crash mid-action
- Rate limiting / throttling
- Resource exhaustion (disk, memory, connections)
- Clock skew / timezone

### Recovery rows follow the same evidence-shape vocabulary

ACs declare evidence shape; Recovery actions should too. "Operator inspects diagnostics" is vague; "operator runs `dark-factory prompt retry <id>` and confirms `phase: in_progress` in the vault task file" is verifiable. Without observable recovery evidence, the verifier can confirm the failure was reached but not that the recovery path was exercised.

## Preflight Checklist

Before approving, verify the spec answers all of these:

- [ ] What problem are we solving?
- [ ] What is the final desired behavior?
- [ ] What assumptions are we making?
- [ ] What are the alternatives (including "do nothing")?
- [ ] What could go wrong?
- [ ] What must not regress?
- [ ] How will we know it's done?
- [ ] **Default: no new scenario.** Most specs are satisfied by unit + integration tests in the implementation prompt. Add a scenario only when (a) unit/integration tests genuinely cannot reach the behavior, (b) the behavior is load-bearing for an essential user journey, (c) no existing scenario covers it, and (d) the regression risk is concrete and named. See `docs/rules/scenario-writing.md` for the full rule. If unsure: NO scenario.

If the spec can't answer these in under a page, it's underdesigned or too large.

## Test-layer responsibilities

Specs drive three defense layers. Keep them scoped correctly:

| Layer | Belongs to | Catches | Default coverage |
|---|---|---|---|
| Unit contract test | Prompt | Single-function library validator, parser, marshaller on a new value | **Always** — bulk of test coverage |
| Integration test | Prompt | Dispatch-path round-trip, registry lookup, serialization through real code | **Most specs** — covers what unit can't |
| End-to-end scenario | Spec + scenario | Real deployment behavior, multi-service interactions, boundaries no test harness can fake | **Rare** — only when the bottom two layers genuinely cannot reach the behavior |

The pyramid: broad base of unit tests, smaller layer of integration tests, narrow tip of E2E scenarios. Most specs ship with prompt-level tests only. A scenario is justified only when integration tests can't reach the behavior — see `docs/rules/scenario-writing.md` for the four-condition test.

**Example where a scenario IS justified:** spec 068 — the clone workflow crashed with `exit 128` at runtime after the clone was deleted. Unit tests passed. The bug was a control-flow ordering issue that no test double could catch. The scenario locks it down.

**Example where a scenario is NOT justified:** a new config field whose handler is unit-tested and whose effect is unit-tested. The field reaches runtime via the same loader path 200 other fields use; reproducing that with a scenario adds no signal.

## Audit and Approve

Always audit before approving:

```text
/dark-factory:audit-spec specs/my-feature.md
```

Then approve via CLI (never manually edit frontmatter):

```bash
dark-factory spec approve my-feature
```

This moves the spec from `specs/` to `specs/in-progress/`, assigns a number, and sets `status: approved`. The daemon then auto-generates prompts from the approved spec.

## Spec Status Lifecycle

| Status | Directory | Meaning | How it happens |
|--------|-----------|---------|----------------|
| `idea` | `specs/ideas/` | Rough concept, no full sections | Human creates file |
| `draft` | `specs/` | All sections filled, ready for review | Human/AI writes spec |
| `approved` | `specs/in-progress/` | Ready for prompt generation | `dark-factory spec approve` |
| `prompted` | `specs/in-progress/` | Prompts generated | Auto (dark-factory) |
| `verifying` | `specs/in-progress/` | All linked prompts completed | Auto (dark-factory) |
| `completed` | `specs/completed/` | Acceptance criteria verified | `/dark-factory:verify-spec` (preferred — see [spec-verification.md](../spec-verification.md)) or manual `dark-factory spec complete` |

Completed specs are immutable. If behavior changes later, create a new spec.

## Next Steps

- Prompts are auto-generated from the spec by the daemon
- Or write prompts manually: [prompt-writing.md](prompt-writing.md)
- Run the pipeline: [running.md](../running.md)
- Bug reports: [bug-workflow.md](../bug-workflow.md)
