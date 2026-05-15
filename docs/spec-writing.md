# Spec Writing Guide

A spec is a behavioral contract for a multi-prompt feature. It describes what the system should do, not how the code should look.

## When to Write a Spec

| Situation | Spec needed? |
|-----------|-------------|
| Multi-prompt feature (3+ prompts) | Yes |
| Unclear edge cases or failure modes | Yes |
| Touching shared interfaces | Yes |
| Bug report with reproduction | Yes — see [bug-workflow.md](bug-workflow.md) |
| Single-file fix, obvious change | No — write a prompt directly |
| Config change, version bump | No — write a prompt directly |

For bugs specifically, see [bug-workflow.md](bug-workflow.md) — adds `kind: bug` frontmatter, mandatory Reproduction section, and verification rules that go beyond standard spec verification.

## Creating a Spec

Use the Claude Code command:

```
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

Then fill the remaining sections:

- **Summary** — 3-5 bullet points, plain language, no code references
- **Problem** — one paragraph, why this matters
- **Goal** — the finished system, not the changes
- **Non-goals** — what this work will NOT do
- **Desired Behavior** — numbered observable outcomes (3-8)
- **Constraints** — interfaces, tests, config format, behavior that must not change
- **Failure Modes** — table: trigger → expected behavior → recovery
- **Do-Nothing Option** — cost of not doing this
- **Security / Abuse** — if HTTP, files, or user input involved
- **Acceptance Criteria** — binary, testable checkboxes
- **Verification** — exact commands (`make precommit`)

**The ratio:** 70% what/why/constraints, 30% how.

### Reference Docs

When a spec needs technical detail (API endpoints, protocol formats) that would make it too implementation-level:

- Put it in `docs/` and reference from the spec
- **Spec** stays behavioral — what the system does
- **Doc** holds implementation context — API references, code examples
- **Prompts** reference both

## Scope Check

- **Desired behaviors > 8?** Look for a natural split
- **Two features with different do-nothing arguments?** Split into separate specs
- **Contains struct names or file paths that aren't frozen constraints?** Too implementation-level — push details to prompts

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
| **Metric value** | "Prometheus counter `foo_total{label=x}` increments by 1" |
| **Cluster state** | "`kubectl get pod X` returns Running with container ready=true" |
| **Vault / file artifact** | "task file under `tasks/` has frontmatter field `Y: Z`" |

### Bad ACs (no evidence shape)

- ❌ "Unit test covers this" — that's the test plan, not the evidence
- ❌ "It works" / "Functionality verified" — narration
- ❌ "Tests pass" without naming what specific behavior is asserted

### Good ACs (evidence shape declared)

- ✅ "After invocation, `cat tasks/<id>.md` shows `phase: completed` in frontmatter"
- ✅ "`kubectlquant -n dev logs <pod> | grep 'job spawned'` returns ≥1 match"
- ✅ "`gh pr view 2 --json reviews` lists one review with `state: APPROVED` by login `pr-review-of-ben`"

The point is not to inline test scripts — it's to make the AC's *observable target* unambiguous.

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

**Flagged words:** `should`, `appropriate`, `reasonable`, `as needed`, `where applicable`, `if necessary`, `proper`, `correct`, `sensible`, `suitable`, `relevant`, `adequately`, `sufficiently`, `etc.`, `and so on`, `among others`

### Resolution rules

Each occurrence must either:

1. **Resolve to a concrete rule** — replace "appropriate retry policy" with "retry once with 5s + jitter backoff"
2. **Be explicitly marked "agent decides at impl time"** — acceptable ONLY when the decision is truly local (one-line scope) and reversible (no schema, no persistent state, no external contract)

### Good vs bad

- ❌ "Agent retries appropriately on network failure"
- ✅ "Agent retries once after 5s + jitter on network failure; on second failure, escalates"
- ✅ "On retry, the agent decides the backoff jitter at impl time (local, reversible)"

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

## Preflight Checklist

Before approving, verify the spec answers all of these:

- [ ] What problem are we solving?
- [ ] What is the final desired behavior?
- [ ] What assumptions are we making?
- [ ] What are the alternatives (including "do nothing")?
- [ ] What could go wrong?
- [ ] What must not regress?
- [ ] How will we know it's done?
- [ ] **Default: no new scenario.** Most specs are satisfied by unit + integration tests in the implementation prompt. Add a scenario only when (a) unit/integration tests genuinely cannot reach the behavior, (b) the behavior is load-bearing for an essential user journey, (c) no existing scenario covers it, and (d) the regression risk is concrete and named. See `docs/scenario-writing.md` for the full rule. If unsure: NO scenario.

If the spec can't answer these in under a page, it's underdesigned or too large.

## Test-layer responsibilities

Specs drive three defense layers. Keep them scoped correctly:

| Layer | Belongs to | Catches | Default coverage |
|---|---|---|---|
| Unit contract test | Prompt | Single-function library validator, parser, marshaller on a new value | **Always** — bulk of test coverage |
| Integration test | Prompt | Dispatch-path round-trip, registry lookup, serialization through real code | **Most specs** — covers what unit can't |
| End-to-end scenario | Spec + scenario | Real deployment behavior, multi-service interactions, boundaries no test harness can fake | **Rare** — only when the bottom two layers genuinely cannot reach the behavior |

The pyramid: broad base of unit tests, smaller layer of integration tests, narrow tip of E2E scenarios. Most specs ship with prompt-level tests only. A scenario is justified only when integration tests can't reach the behavior — see `docs/scenario-writing.md` for the four-condition test.

**Example where a scenario IS justified:** spec 068 — the clone workflow crashed with `exit 128` at runtime after the clone was deleted. Unit tests passed. The bug was a control-flow ordering issue that no test double could catch. The scenario locks it down.

**Example where a scenario is NOT justified:** a new config field whose handler is unit-tested and whose effect is unit-tested. The field reaches runtime via the same loader path 200 other fields use; reproducing that with a scenario adds no signal.

## Audit and Approve

Always audit before approving:

```
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
| `completed` | `specs/completed/` | Acceptance criteria verified | `/dark-factory:verify-spec` (preferred — see [spec-verification.md](spec-verification.md)) or manual `dark-factory spec complete` |

Completed specs are immutable. If behavior changes later, create a new spec.

## Next Steps

- Prompts are auto-generated from the spec by the daemon
- Or write prompts manually: [prompt-writing.md](prompt-writing.md)
- Run the pipeline: [running.md](running.md)
- Bug reports: [bug-workflow.md](bug-workflow.md)
