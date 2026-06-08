# Choosing a Flow

**Canonical decision: direct vs prompt vs spec.** This is the single source of truth — every other doc that mentions the choice points here.

Dark-factory is **not a self-fulfilling tool**. It earns its place only when ceremony pays for itself. Most edits don't need it. The default for markdown, docs, configs, and small scripts is **direct edits, no dark-factory**.

> *Note: "direct" is overloaded in this project. **This guide's "Direct" = authoring by hand, no YOLO container** — i.e. how the change is written. Whether the resulting commit goes straight to master or through a feature branch + PR is a separate per-repo decision (branch protection, team review norms) outside this guide's scope. Dark-factory's `workflow: direct` config field is a different overload again: it means the container commits to the current branch — read [workflows.md](workflows.md) for those isolation/delivery modes.*

## TL;DR

| Change | Flow |
|---|---|
| Markdown, docs, README, YAML, config, small script | **Direct** — edit by hand, no dark-factory, no YOLO container |
| Code change that does NOT need its own business-why document | **Prompt** — write a prompt, audit, approve, daemon executes |
| Feature where the business-why deserves its own durable document | **Spec → prompts** — spec first, daemon auto-generates prompts, audit each, approve, daemon executes |

## How to decide (two questions, in order)

1. **Is this code that runs in a build / production / CI pipeline?** If no → **direct**. Markdown, READMEs, configs, YAML, agent/skill definition files, operator shell scripts, GoDoc comments — all direct. Stop here.
2. **Does the change carry a business-level "why" that deserves a permanent in-repo document?** If no → **prompt**. If yes → **spec**.

That's the whole decision. Two questions, three outcomes.

## Why the headline reason matters

The reason prompts and specs exist at all is **safe unattended execution**. Prompts run inside a YOLO Claude container with permission checks disabled. You queue work, step away, come back to commits — no "Approve this Bash command?" interruptions. Interactive Claude Code blocks on every tool call; dark-factory removes that friction.

Once you have unattended execution, the other benefits follow: documentation (committed prompts/specs survive context decay), decomposition (specs force edge-case thinking before code exists), token savings (the YOLO container runs Sonnet by default).

**If the change is not worth unattended execution — because it's faster to just edit it yourself than to write a prompt, audit it, approve it, wait for the container — pick direct.** That covers most markdown, doc, and small-script work.

## Concrete examples

### Direct (edit by hand)

- A README.md update — even with fenced code examples (the code is illustrative, not executable)
- A new agent definition (`agents/foo.md`) — markdown + frontmatter
- A new skill (`~/.claude/skills/foo/SKILL.md` + `scripts/foo.sh`, ~50–100 LOC of bash)
- A CLAUDE.md edit
- A YAML config tweak (kubernetes manifest, GitHub Actions workflow, dark-factory config)
- A markdown doc anywhere (`docs/*.md`, `specs/ideas/*.md`)
- A GoDoc comment-only change
- A standalone operator bash script in `bin/` or `scripts/` (~50 LOC, no test harness)
- A typo fix in any file

### Prompt (technical change, no business-why doc warranted)

- Add a method to an existing interface, with tests — no user-visible behavior change beyond the new capability
- Refactor a Go file to extract a helper — pure code-shape work
- Add a missing nil-check + regression test for a bug whose root cause is obvious — no narrative needed
- Bump a dependency that requires accompanying code edits
- Rename a type across a package + update call sites
- Add a new Kafka topic consumer that mirrors an existing pattern — no new business behavior
- Mechanical fan-out (e.g., apply the same change to 20 files) — the prompt IS the change record

### Spec (business-why deserves its own doc)

- A new user-visible feature, even a small one
- A behavior change that affects observable contract (verdict semantics, API shape, downstream consumers)
- A bug fix where the *reproduction* and *regression-lock* deserve permanent record (see [bug-workflow.md](bug-workflow.md))
- A multi-prompt change where the cross-prompt rationale would be lost in any single prompt
- A feature whose verification needs the structured evidence pass that `/dark-factory:verify-spec` enforces — fresh runtime observation, not just "tests pass" (see [spec-verification.md](spec-verification.md))
- Anything that future-you will want to find by grepping `specs/completed/` for "why does this exist"

## Boundary cases (the ones that cause confusion)

| Case | Flow | Why |
|---|---|---|
| Skill `SKILL.md` + bash script in `~/.claude/skills/` | **Direct** | Lives in user's `~/.claude/`, not a dark-factory project; no test harness; iteration is faster by hand |
| Bash operator script in a Go project (`bin/foo.sh`, ~50 LOC) | **Direct** | No precommit test gate worth the prompt ceremony |
| README update that *adds* a code example | **Direct** | Code is illustrative; the README is the artifact |
| Documentation PR that *describes* a system invariant the agents must honor | **Direct** | Doc-only; cite the invariant in the relevant code's docstring or test |
| A test-only PR adding regression coverage for an existing bug | **Spec (kind: bug)** preferred; **Prompt** acceptable if the bug already has a closed spec | If a fix is going through a `kind: bug` spec, the reproduction lives there per [bug-workflow.md](bug-workflow.md). A standalone regression test for a bug that already shipped a fix is a smaller surface — a prompt is fine. |
| Adding a counterfeiter `//go:generate` annotation + regenerating mocks | **Prompt** | Mechanical, but it's code and runs through tests |
| Adding a `## Unreleased` CHANGELOG entry alongside a code change | Part of the **prompt/spec** for the code change | Not its own flow |
| Updating `go.mod` dependency version | **Direct** if the bump is patch-only and `make test` was run by hand; **Prompt** if the bump requires code edits |
| Renaming a Go file (`git mv` only, no content change) | **Direct** | Pure file-system move; no test value from going through dark-factory |
| Generating prompts from an approved spec | **No flow** — the daemon does it automatically |

If a case doesn't appear here and you can't pick from the two-question decision: default to **direct** for anything markdown/doc/script-shaped, **prompt** for anything Go/Python/code-shaped. Specs are reserved for cases where a future reader would benefit from finding the *why* as a discoverable document.

## Anti-patterns

- **Spec-by-default for every code change.** A spec is overhead when there's no business "why" to capture. A 5-line refactor doesn't warrant a spec.
- **Prompt-by-default for every markdown edit.** Writing a prompt to edit a README is ceremony for nothing — no test runs, no business-why, no unattended-execution payoff.
- **Direct edits for production code changes.** Direct is for low-stakes textual work. Production code goes through the daemon so tests run and the artifact survives.
- **Conflating "Direct authoring" with "direct push to master."** Authoring by hand says nothing about whether the resulting commit needs a PR. Branch-protection rules and per-repo workflow are a separate concern — out of scope for this guide.
- **Writing a spec because "this might be big."** Size is not the criterion. Business-why is. A 50-prompt mechanical refactor stays prompts; a 1-prompt feature with a real "why" may warrant a spec.
- **Skipping the spec on a bug fix to "save time."** Bugs get specs (`kind: bug`) because the reproduction + regression-lock is exactly what the spec format captures. See `bug-workflow.md`.

## What gets committed

| Flow | Artifact in repo | Audience for the artifact |
|---|---|---|
| Direct | Just the diff | Code reviewers |
| Prompt | Prompt + diff | Future reader asking "what was the technical task that produced this code?" |
| Spec → prompts | Spec + prompts + diff | Future reader asking "why does this feature exist?" + "what was the technical task?" |

The artifact persists in the repo. Choose the flow whose artifact a future reader (or future-you) will actually want to find.

## Related

- Why dark-factory exists at all → [architecture-flow.md](architecture-flow.md) § "Why the prompt flow exists at all"
- How to write a spec (after deciding to write one) → [rules/spec-writing.md](rules/spec-writing.md)
- How to write a prompt (after deciding to write one) → [rules/prompt-writing.md](rules/prompt-writing.md)
- Bug-specific spec rules → [bug-workflow.md](bug-workflow.md)
- Verifying a spec actually works at runtime → [spec-verification.md](spec-verification.md)
- Running the daemon, lifecycle → [running.md](running.md)
