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
