---
description: Refine a spec by narrowing scope and splitting adjacent concerns (idea→draft)
argument-hint: "<spec-file-path>"
---

Refine the dark-factory spec at $ARGUMENTS.

Resolve the path: if no prefix, look in `specs/` then `specs/ideas/`; append `.md` if missing.

Read the spec. If status is `approved`, `prompted`, `verifying`, or `completed` — refuse and tell the user to file a new spec for further changes.

## Workflow

Run an interactive narrowing dialogue with the user. Use plain conversational prose at every decision point — do NOT use AskUserQuestion. After each question, wait for the user's reply before continuing.

### 1. Anchor on a single sentence

Ask the user:

> "In one sentence, what is this spec really about?"

If the conversation already established a clear single-sentence anchor (e.g. earlier in this thread), skip the question and quote the existing anchor back to confirm. The author's answer is the anchor for every subsequent decision.

### 2. Inventory every distinct concern

Walk the spec's Goal, Desired Behavior, Failure Modes, Acceptance Criteria, Open Questions. Produce a flat bullet list of distinct user-visible behaviors, changes, or edge cases. Show the inventory verbatim.

### 3. Categorize each item against the single sentence

Three buckets:

- **Same problem, different clothes** → keep in v2.
- **Adjacent problem, own "why"** → split into a stub at `specs/ideas/<slug>.md`.
- **Defensive edge case not yet biting** → defer to Non-goals in v2 with a deferral note.

Present the categorization. Ask the user to confirm or move items between buckets.

### 4. Propose 2-3 first-iteration scope cuts and recommend ONE

Each cut: title (the v2 spec name), one-line description of what it delivers, what it explicitly defers.

Recommend ONE clearly with reasoning. Never present a menu of equals.

**Bias toward:**
- Smallest cut that still solves the single-sentence problem.
- Cut where every acceptance criterion is binary and observable from outside.
- Cut that captures one "why" cleanly, not two.

**Bias against:**
- Cuts that bundle two "why"s — those should be two specs.
- Cuts that defer the load-bearing piece (the user's actual pain).
- Cuts that satisfy a checklist but not the single sentence.
- Cuts whose acceptance criteria can only be verified by reading code.

Wait for the user's pick (or acceptance of the recommendation).

### 5. Rewrite the spec to the chosen scope

- Narrow `Goal`, `Desired Behavior`, `Acceptance Criteria` to the single sentence.
- Move adjacent concerns to `Non-goals` with one-line deferral notes.
- Keep `Constraints`, `Failure Modes` (3-column: Trigger | Expected | Recovery), `Verification`.
- Behavioral ratio target: 70% what/why/constraints, 30% how.

**Scenario-trigger check.** Add a scenario acceptance criterion if the change introduces or modifies an integration seam:
- New Kafka operation / schema / topic
- New CRD field, new HTTP route
- New subprocess interface, new external service call
- Config field → runtime behavior wiring
- Host ↔ container, host ↔ git remote boundary

If a seam is touched, the AC reads: "Scenario added under `scenarios/` (number assigned at scenario-write time): [one-line assertion]."

### 6. Create stub specs for split-out concerns

For each bucket-2 item, write a stub at `specs/ideas/<slug>.md`:

```yaml
---
status: idea
---

# <Title>

## Summary

<1-2 sentences extracted from the inventory>

## See also

- `specs/<v2-slug>.md` — split out from this spec
```

Use lowercase-kebab-case slugs.

### 7. Transition status idea → draft

If the spec was `status: idea`:
- Update frontmatter to `status: draft`.
- If file was in `specs/ideas/`, move to `specs/` (`mv specs/ideas/<x>.md specs/<x>.md`).

### 8. Hand off

Print a summary and the next-step command:

```
Refined: <v2-path>
  Single sentence: "<the anchor>"
  Scope cut chosen: <cut-name>

Split into stubs:
  - specs/ideas/<slug-1>.md — <one-line reason>

Deferred to Non-goals:
  - <item> — <one-line reason>

Status transition: idea → draft (file moved if applicable)

Next: /dark-factory:audit-spec <v2-path>
```

## Single-sentence test (before handing off)

After narrowing, re-test against the single sentence:
- Does every Desired Behavior follow from the single sentence?
- Does every Acceptance Criterion verify the single sentence?
- Could the spec's Goal be replaced by the single sentence without loss?

If any answer is no, the cut is wrong. Loop back to step 4.

## Anti-patterns

- **Dumping options without a recommendation** — the user came for opinionated narrowing, not a menu.
- **Silent deletion** — never drop a concern from the original spec without keeping it (bucket 1), splitting it (bucket 2), or explicitly deferring it in Non-goals (bucket 3).
- **Skipping the single-sentence anchor** — without it, narrowing has nothing to test against.
- **Setting status to `approved` or beyond** — refine ends at `draft`. Approval is the CLI's job.
- **Editing the spec before the user has confirmed the scope cut** — refine is a dialogue, not a fait accompli.
- **Inventing requirements not in the original spec** — refine narrows; create-spec captures.
- **Asking the single-sentence question when the conversation already answered it** — quote the existing anchor instead of restarting.
