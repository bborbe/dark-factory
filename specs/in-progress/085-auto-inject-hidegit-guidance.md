---
status: generating
tags:
    - dark-factory
    - spec
approved: "2026-05-21T21:33:31Z"
generating: "2026-05-21T21:33:31Z"
branch: dark-factory/auto-inject-hidegit-guidance
---

## Summary

- When `hideGit=true` is active, the container masks `/workspace/.git` as a character device so the worktree pointer cannot be followed back into the parent repo.
- Modern LLM agents inspect `.git` early, see the char-device mask, and heuristically conclude "this is not a git repo — skip `make precommit`" — bypassing the project's authoritative success gate.
- This was observed concretely on 2026-05-21 (maintainer spec 033 prompt 127): agent skipped precommit, reported `status: partial`, and the host-run gate then surfaced two real `errcheck` violations that would have been caught had the gate actually run.
- Fix: when `hideGit=true` is in effect for a prompt-execution container, dark-factory prepends a constant guidance fragment (via the existing `promptenricher` pathway) that explains the mask is intentional and instructs the agent to run the gate regardless.
- The fragment is wired through one new constructor parameter on `pkg/promptenricher` and one expression in `pkg/factory/factory.go` mirroring the `hideGit` resolution already at line 891. No new config field, no per-project customization.

## Problem

When dark-factory runs prompts in a container with `hideGit=true` (the canonical worktree pattern), `/workspace/.git` appears as a character device (`/dev/null`) inside the container. Agents running these prompts frequently stat `.git` early, observe the char-device shape, and conclude "this is not a git repository — I cannot run `make precommit` or similar git-touching commands." They skip the project's success gate on that heuristic, even though `GOFLAGS=-buildvcs=false` is typically already set so `go build` skips VCS stamping, and `go test`, `errcheck`, `gosec`, `golangci-lint`, `python -m pytest`, etc. — almost every precommit tool — only read source files, not git metadata. Skipping precommit means real lint/test failures slip through to the operator, who then reproduces them by hand on the host.

Concrete repro (2026-05-21): maintainer spec 033 prompt 127 (`lib/githubapp`) completed all code work, achieved 95.5% coverage, and reported `status: partial` with a `blockers:` entry stating "make precommit cannot run: git repository not properly set up in container". Running `make precommit` on the host immediately surfaced two real `errcheck` violations (`w.Write` return values unchecked) — exactly what the gate is supposed to catch. The agent's heuristic skip cost a manual round-trip.

## Goal

When `hideGit=true` is in effect for a prompt-execution container, every prompt the agent sees begins with a short, constant guidance fragment that:

1. Names the `/workspace/.git` character-device shape and states it is intentional dark-factory behavior, not a broken repo.
2. States that `GOFLAGS=-buildvcs=false` is typically already set so `go build` skips VCS stamping; other precommit tools only read source files.
3. Instructs the agent to run the project's precommit / validation gate regardless of `.git`'s appearance.
4. Names the failure pattern explicitly so the agent does not infer "not a git repo → skip" from `.git` being a char device.

When `hideGit=false`, behavior is unchanged — no fragment is prepended.

## Non-goals

- Customizing the fragment text per project. The fragment is a single constant shipped with dark-factory; updates ride dark-factory releases, not per-project config.
- Adding a new config field to enable/disable the fragment. `hideGit` is the single switch.
- Spec-generation containers. They do not honor `hideGit` today (sibling spec 084 fixes that); once 084 lands the fragment naturally applies to spec gen because both paths go through the same enricher.
- Per-language guidance customization (e.g. Python-specific advice). The fragment is intentionally language-agnostic.
- Other heuristics agents might use to skip the gate ("the test command says skipped", agent fatigue, etc.). This spec addresses the `.git`-char-device heuristic only.
- Doc rewrites beyond a short note in `docs/troubleshooting.md` or `docs/configuration.md` mentioning the fragment exists and what triggers it.

## Assumptions

- The existing `promptenricher` is the single chokepoint through which every prompt's instructions flow before being handed to the container; adding the fragment there reaches every prompt without touching prompt sources or per-project config.
- The `hideGit` value resolved at `pkg/factory/factory.go:891` (`workflow == config.WorkflowWorktree || hideGit`) is the canonical "is hideGit active for this prompt-execution container?" answer. The enricher's new parameter receives the same resolved value.
- The agent reads instructions sequentially and gives weight to the leading content; placing the fragment after `additionalInstructions` (which already prepends) and before the prompt body preserves project-specific instruction primacy while still appearing well before the prompt body.
- Adding ~200 words of constant guidance to every prompt is acceptable token cost. The cost of one skipped precommit gate (one operator round-trip) exceeds the per-prompt token delta many times over.
- Sibling spec 084 (fail-fast-on-worktree-without-hidegit) addresses the spec-generation executor's hardcoded `hideGit=false`. This spec does not depend on 084 landing first; it operates wherever `hideGit=true` is plumbed today.

## Desired Behavior

1. When the prompt-executor is constructed with `hideGit=false` (the default), the enricher prepends nothing new. The prompt content the agent sees is byte-identical to today.
2. When the prompt-executor is constructed with `hideGit=true`, the enricher prepends the fragment to every prompt's content. The fragment appears AFTER `additionalInstructions` (if any) and BEFORE the prompt body, so project-specific instructions remain at the top.
3. The fragment text is a single constant defined once in the `pkg/promptenricher` package. Updating it is a single-line edit; no per-project configuration intervenes.
4. The fragment text:
   - Names the char-device shape of `/workspace/.git` explicitly.
   - States the mask is intentional and not a signal that the workspace is "not a git repo".
   - States that `GOFLAGS=-buildvcs=false` is typically already set, and that `go test`, `errcheck`, `gosec`, `golangci-lint`, etc. work without git metadata.
   - Instructs the agent to run `make precommit` (or the project's equivalent gate) regardless of `.git`'s appearance.
5. The factory passes the resolved `hideGit` value (matching the expression already used at `pkg/factory/factory.go:891`) into `promptenricher.NewEnricher` for both the daemon path and the run path. No second resolution code path is introduced.
6. The existing `Enricher.Enrich(ctx, content) string` method signature is unchanged. The new behavior is additive — callers that already use `Enrich` see the fragment automatically when `hideGit=true`.

## Constraints

- The `hideGit` config field, its YAML key, its CLI override path, and its default value (`false`) do not change.
- The `Enricher` interface signature (`Enrich(ctx context.Context, content string) string`) does not change. The change is constructor-only.
- The fragment text is defined once in the `pkg/promptenricher` package (either inline in `enricher.go` or a sibling file). No duplication.
- The factory wiring expression for the enricher's `hideGit` parameter must match the existing resolution at `pkg/factory/factory.go:891` (`workflow == config.WorkflowWorktree || hideGit`) or its functional equivalent — both the daemon and `run` paths must pass the same resolved value.
- Existing `Enrich` behavior (prepending `additionalInstructions`, appending completion / changelog / test-command / validation suffixes) is preserved exactly. The new fragment is the only additive behavior.
- The fragment is prepended only when `hideGit=true`. There is no other condition (no per-project flag, no env var, no CLI override) that turns it on or off.
- All existing tests in `pkg/promptenricher/...` and `pkg/factory/...` must continue to pass without modification.
- Documentation update is part of scope: `docs/troubleshooting.md` (or `docs/configuration.md` if the troubleshooting doc does not exist at implementation time) gains a short subsection naming the fragment and the trigger condition.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---------|-------------------|----------|-----------|
| `hideGit=true` and `additionalInstructions` is set | Fragment is prepended AFTER `additionalInstructions` so project-specific instructions remain at the top; both appear before the prompt body | None required — by design | Inspecting an emitted prompt shows `additionalInstructions` first, then fragment, then prompt body |
| `hideGit=true` and `additionalInstructions` is empty | Fragment is prepended directly before the prompt body | None required | Inspecting an emitted prompt shows fragment first, then prompt body |
| `hideGit=false` (default) | Fragment is NOT prepended; emitted prompt is byte-identical to today's output | None required | A diff of emitted prompts before/after this change shows zero delta when `hideGit=false` |
| Factory passes `hideGit=true` to enricher but the executor's `hideGit` is somehow `false` (wiring mismatch) | Regression: fragment appears even though `.git` is NOT masked. Agent still runs precommit (the fragment instructs it to), so impact is benign. | Audit the wiring expression to confirm enricher and executor receive the same resolved value | A unit test asserts the factory passes the same expression to both, catching divergence at CI time |
| Factory passes `hideGit=false` to enricher but the executor's `hideGit` is somehow `true` (wiring mismatch) | Regression: fragment does NOT appear even though `.git` IS masked. Agent may hit the original bug (skip precommit). | Same audit; same unit test | Same — the test catches divergence in either direction |
| Fragment text becomes stale (e.g. dark-factory stops setting `GOFLAGS=-buildvcs=false` by default) | Operator updates the constant in `pkg/promptenricher` — one-line edit | Update the constant; ship a new dark-factory release | No automated detection; relies on operator awareness |
| Operator wants to opt out of the fragment for a specific project | Operator sets `hideGit=false` in that project's config; fragment disappears (along with the `.git` mask) | None — opting out of the fragment means opting out of `hideGit` entirely, which is the intended coupling | n/a |
| Operator wants the fragment but does NOT want `hideGit` masking (impossible state) | Not supported; this spec intentionally couples fragment to `hideGit=true` | n/a — the fragment only makes sense when the mask is active | n/a |

## Security / Abuse Cases

Not applicable. The fragment is a constant string compiled into the dark-factory binary. No untrusted input, no network surface, no trust-boundary crossing. The fragment is shown only to the LLM agent inside the operator-controlled container.

## Acceptance Criteria

**Rung 1 — Enricher implementation:**

- [ ] `pkg/promptenricher.NewEnricher` gains a new `hideGit bool` constructor parameter — evidence: `grep -nE 'NewEnricher\(' pkg/promptenricher/enricher.go` shows the signature with a `hideGit bool` parameter, and `grep -nE '\thideGit\s+bool' pkg/promptenricher/enricher.go` returns ≥1 line on the struct definition.
- [ ] The fragment text is defined exactly once in the package — evidence: `grep -rnE 'character device|hideGit=true active' pkg/promptenricher/` returns matches in exactly one file (`enricher.go` or a sibling file in the same package).
- [ ] When the enricher is constructed with `hideGit=true`, `Enrich` prepends the fragment after `additionalInstructions` and before the prompt body — evidence: a unit test constructs the enricher with `hideGit=true` and `additionalInstructions="PROJECT_HEADER"`, calls `Enrich(ctx, "PROMPT_BODY")`, and asserts the returned string contains the substrings `PROJECT_HEADER`, the fragment's distinctive marker (e.g. `hideGit=true active`), and `PROMPT_BODY` in that order (use `strings.Index` for ordering assertion).
- [ ] When the enricher is constructed with `hideGit=false`, `Enrich` does NOT prepend the fragment — evidence: a unit test constructs the enricher with `hideGit=false`, calls `Enrich(ctx, "PROMPT_BODY")`, and asserts the returned string does NOT contain the fragment's distinctive marker.
- [ ] The `Enrich` method signature is unchanged — evidence: `grep -nE 'Enrich\(ctx context.Context, content string\) string' pkg/promptenricher/enricher.go` returns ≥1 line.
- [ ] `make precommit` exits 0 — evidence: exit code 0.

**Rung 2 — Factory wiring:**

- [ ] `pkg/factory/factory.go` passes the resolved `hideGit` value to `promptenricher.NewEnricher`, matching the expression at line 891 (`workflow == config.WorkflowWorktree || hideGit`) or its functional equivalent — evidence: `grep -nE 'NewEnricher\(' pkg/factory/factory.go` shows the call site, and the expression passed for `hideGit` is observably the same as the one passed to `executor.NewDockerExecutor` at the prompt-executor construction site (manual inspection AND a unit test that constructs the factory in both daemon and run modes, asserting the enricher receives the same value as the executor in each).
- [ ] The single centralised call site at `pkg/factory/factory.go:954` passes the resolved `hideGit` value matching the expression at line 891 — evidence: `grep -nE 'promptenricher\.NewEnricher\(' pkg/factory/factory.go` returns exactly 1 line at line 954, and the expression passed for the new `hideGit` parameter is observably identical to the prompt-executor's `workflow == config.WorkflowWorktree || hideGit`. Both daemon and run paths flow through this single site, so no path bypasses the new parameter.
- [ ] Integration test confirms a prompt emitted with `hideGit=true` contains the fragment text and a prompt emitted with `hideGit=false` does not — evidence: `go test ./pkg/factory/... -run TestHideGitFragment -v` (or the equivalent test name) exits 0 and asserts both branches; the test does NOT require running a real container.
- [ ] The misleading skip-precommit failure pattern from spec-033 prompt 127 cannot recur on a prompt emitted with `hideGit=true` — evidence: the integration test above asserts the emitted prompt contains both (a) the fragment's distinctive marker AND (b) explicit text instructing the agent to run the project's precommit gate regardless of `.git`'s appearance.
- [ ] `make precommit` exits 0 — evidence: exit code 0.

**Documentation:**

- [ ] `docs/troubleshooting.md` (or `docs/configuration.md` if the troubleshooting doc does not yet exist) names the fragment and the trigger condition — evidence: `grep -niE 'hideGit.*guidance|guidance.*hideGit|character device' docs/troubleshooting.md docs/configuration.md` returns ≥1 line in at least one of the files, AND the matching section explicitly names `hideGit=true` as the trigger.

**Scenario coverage:** None. The contract is "fragment appears iff `hideGit=true`" — a string assertion on the enricher's output. Unit and integration tests in `pkg/promptenricher/` and `pkg/factory/` cover it. No new dark-factory scenario.

## Verification

```
make precommit
go test ./pkg/promptenricher/... -v
go test ./pkg/promptenricher/... -run TestEnrich_HideGitFalse -v
go test ./pkg/factory/... -run TestHideGitFragment -v
grep -nE 'NewEnricher\(' pkg/promptenricher/enricher.go
grep -nE 'hideGit\s+bool' pkg/promptenricher/enricher.go
grep -rnE 'character device|hideGit=true active' pkg/promptenricher/
grep -niE 'hideGit.*guidance|guidance.*hideGit|character device' docs/troubleshooting.md docs/configuration.md
```

## Related

- `084-fail-fast-on-worktree-without-hidegit.md` — addresses the spec-generator executor's hardcoded `hideGit=false`. Once 084 lands, this fragment naturally extends to spec-generation containers because both paths go through the same enricher.
- `pkg/promptenricher/enricher.go` lines 27-43 (constructor) and 55-82 (`Enrich`) — the existing prepend mechanism this spec extends.
- `pkg/executor/executor.go` lines 60-95 — `hideGit` is a constructor parameter on `dockerExecutor`.
- `pkg/factory/factory.go:891` — the wiring expression `workflow == config.WorkflowWorktree || hideGit` this spec mirrors at the enricher construction site.
- `pkg/config/config.go:91` — the `HideGit bool` field on `Config`.

## Do-Nothing Option

Without this fragment, every agent in every project that uses `hideGit=true` will keep hitting the same heuristic: stat `/workspace/.git`, see a char device, conclude "not a git repo", skip `make precommit`, and report `status: partial` with a blocker about the missing git repo. The operator then runs the gate by hand, hits real lint/test violations, fixes them, and re-prompts. This was observed concretely on 2026-05-21 with maintainer spec 033 prompt 127 — exit cost: one manual `make precommit` run + two `errcheck` fixes + one re-prompt. The cost compounds with every project that adopts the worktree pattern. The fix is a single constant string plus a constructor-parameter pass-through — bounded, durable, and removes a class of operator interventions that currently happens silently inside the prompt-completion path.
