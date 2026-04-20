# Spec Verification Guide

When all prompts linked to a spec complete, dark-factory auto-transitions the spec to `verifying`. The spec sits in `specs/in-progress/` waiting for a human to confirm the acceptance criteria actually hold before it moves to `specs/completed/`.

This guide is the checklist for that confirmation step.

## Why Verify

Prompt completion means "Claude said done and precommit passed on a per-prompt basis". It does not mean:

- All acceptance criteria checkboxes are actually true
- The cross-prompt behaviour composes correctly
- Docs and CHANGELOG were updated
- Dead code from earlier specs was removed (common when the new spec supersedes an older one)
- **The feature actually works when you run it** — green tests ≠ working feature

Verification catches the gap between "prompts ran" and "spec is satisfied".

## What to Verify — Three Layers

Every verification touches three dimensions. Skip one and the bug surfaces in production instead.

| Layer | Verifies | Evidence | Cheapest to catch here because… |
|---|---|---|---|
| **Technical** | Code matches spec | file:line greps, unit tests, `make precommit` | Mechanical; compiler and tests do most of the work |
| **Business / feature** | Spec matches intent | Goal + Desired Behavior hold end-to-end; docs/CHANGELOG reflect user-visible change | Catches scope creep and non-goal violations before they ship |
| **Scenarios** | Reality matches spec | Each Failure Modes row reproducible or test-covered; manual smoke test on dev; Security/Abuse cases exercised | Integration bugs hide until real traffic hits them |

**Rule of thumb:** technical asks "did we build it right?", business asks "did we build the right thing?", scenarios ask "does it survive the real world?". All three must pass.

Map each acceptance criterion to the layer it belongs to:

- "Writer does not modify X" → technical
- "Operator-visible behaviour Y no longer happens" → business
- "Pod eviction during spawn does not inflate counter" → scenario

If all your ACs cluster in one layer, the spec is probably underspecified — flag to the author.

## When to Verify

| Trigger | Action |
|---|---|
| `dark-factory status` shows spec in `verifying` | Start verification |
| Human review requested in daemon notification | Start verification |
| After a batch of related specs land | Verify each in order of approval |

Do not verify while prompts are still queued or running — wait for auto-transition.

## Verification Procedure

Run these six checks in order. Stop at the first failure.

### 1. Read the Spec

Open `specs/in-progress/<id>-<name>.md`. Focus on three sections:

- **Acceptance Criteria** — the binary checkboxes you must confirm
- **Verification** — the exact commands the spec author committed to
- **Constraints** — especially "must not regress" and "must not change"

If the spec supersedes another (look for "supersedes" in Summary), also open the superseded spec and confirm its load-bearing code is actually removed, not just bypassed.

### 2. Confirm Code Matches Each Acceptance Criterion

For each checkbox, grep or read the file(s) it references. Prefer targeted grep over full-file reads.

- Interface/behaviour criterion (e.g. "writer does not modify X") → grep the implementation, read ±20 lines
- Test criterion (e.g. "unit test covers Y") → grep the test file for the described case
- Doc criterion (e.g. "CHANGELOG notes Z") → grep the doc
- Removal criterion (e.g. "guard from spec N is gone") → grep should return no hits; if hits remain, verification fails

Record each criterion as confirmed or failed. Do not mark verified by vibes — cite file:line.

### 3. Run the Spec's Verification Commands

Run exactly what the spec's `## Verification` section lists. Do not substitute equivalents.

Typical pattern for Go specs in this repo:

```bash
cd <module> && go test ./...
# or
cd <module> && make precommit
```

Delegate long-running commands to `simple-bash-runner` so the main context doesn't drown in output.

If the spec has a manual smoke test (e.g. on dev cluster), either:

- Run it yourself and record the result, or
- Explicitly note that manual smoke is deferred and mark the spec `verified-minus-smoke` in the verification summary

### 4. Find Live Evidence the Feature Actually Works

Tests passing ≠ feature works. Tests prove the code does what the author thought; they don't prove the deployed system does what the user needs. Before marking complete, look for evidence from a running system.

Preferred sources, strongest first:

- **Run it live.** Trigger the feature in dev/staging with a realistic input. Watch it happen.
- **Logs from an already-running instance.** If the feature has shipped to dev (via `make buca` or similar), pull logs and find the log lines that prove the new behaviour fired. Grep for the code path added by this spec.
- **Metrics / dashboards.** If the spec added counters or gauges, query them — non-zero values from a real run are strong evidence.
- **Git history of real task runs.** For dark-factory itself: check `.dark-factory.log`, prompt logs, or vault commit history for a run that exercised the new path.
- **Before/after comparison.** Same input, old version vs new version — the diff in behaviour is the evidence.

What to record for each AC that needs live evidence:

- The command or trigger you ran
- The log line, metric value, or output that proves it worked
- Timestamp + environment (dev/staging cluster, local, etc.)

If the feature is not yet deployed anywhere you can observe, either:

- Deploy it to dev now and verify, or
- Explicitly mark the AC `verified-minus-live` and create a follow-up task to confirm after deploy

Do not skip this step for features that change runtime behaviour. "Unit tests pass" is not the same as "the feature does the thing in production". Most spec misses hide here — the code compiles, tests go green, and the deployed system still does the wrong thing because of configuration, ordering, or integration the tests didn't cover.

### 5. Check Supersession Hygiene (if applicable)

If the spec says it supersedes an earlier one:

- The superseded code path must be removed, not just bypassed
- The superseded spec's acceptance criteria must not regress
- CHANGELOG should mention both the new behaviour and the removal

Common miss: a new spec adds a replacement path and leaves the old one as "dead code" reachable via a flag. Dead code is not removed code — flag this.

### 6. Summarise and Ask to Mark Complete

Report back to the human inviting completion:

- Each AC with ✅/❌ and the file:line evidence
- Test run result per module
- Any deferred items (manual smoke, follow-ups)
- "Mark completed?" as a yes/no question

Never run `dark-factory spec complete` without explicit human confirmation. Mark-complete is the point of no return — the spec file moves to `specs/completed/` and is treated as immutable.

## Marking Complete

Once the human confirms:

```bash
dark-factory spec complete <spec-id>
```

This moves the spec to `specs/completed/` and stamps `status: completed`. The file is now immutable — future behaviour changes need a new spec.

## Failure Handling

If verification fails:

1. Do NOT run `dark-factory spec complete`
2. Report the failing AC with file:line or test output
3. Options:
   - Fix in place with a one-off prompt or manual edit (small gaps)
   - Create a follow-up spec (significant scope miss)
   - Revert the spec's changes and re-run prompts (fundamental issue)

The spec stays in `verifying` until verification passes. Re-run verification after each fix.

## What Not to Do

- Do not edit acceptance criteria to match what was built. If the build missed an AC, the build is wrong.
- Do not skip `## Verification` commands because "it probably works". Run them.
- Do not mark complete with open TODOs in the code referencing the spec.
- Do not verify multiple specs in parallel — they share codebase state and failures cross-contaminate.
- Do not mark complete on "tests pass" alone for a feature that changes runtime behaviour. Find live evidence — run it, read logs, check metrics.

## Reference

- Spec lifecycle: [spec-writing.md](spec-writing.md#spec-status-lifecycle)
- Running the pipeline: [running.md](running.md)
- Definition of done: [dod.md](dod.md)
