# Prompt Writing Guide

A prompt is a markdown file that tells the YOLO agent what to build. Each prompt produces one atomic, verifiable change.

## When to Write a Prompt

**This document does NOT decide direct vs prompt vs spec.** That decision lives in [../choosing-a-flow.md](../choosing-a-flow.md) — the single source of truth. Read it first to confirm a prompt is the right flow.

In short: write a prompt when the change is a code change that does NOT carry a business-level "why" that deserves its own document. Markdown / docs / configs / small operator scripts → direct edits, not prompts. Features with a real business-why → spec first. See the canonical decision doc.

You may also write prompts manually for finer control over decomposition from an approved spec (the daemon does this automatically by default).

## Creating a Prompt

Use the Claude Code command:

```
/dark-factory:create-prompt
```

Or create manually. Location depends on status and project config:

| Status | Directory | Purpose |
|--------|-----------|---------|
| `idea` | `prompts/ideas/` | Rough concepts, not ready for approval |
| `draft` | inbox dir (default: `prompts/`) | Complete prompts, ready for review and approval |

The inbox directory is configurable per project. Run `dark-factory config` to check the actual `inboxDir` path before writing prompts.

```bash
# Check your project's inbox directory
dark-factory config | grep inboxDir

# Idea — park it for later
touch prompts/ideas/my-change.md

# Draft — ready for review (use the inboxDir from config)
touch prompts/my-change.md
```

Use lowercase-kebab-case. Dark-factory assigns execution numbers on approve (e.g., `248-spec-044-model.md`).

**Ordering prefix for dependent prompts.** When multiple prompts must execute in strict order, prefix with a simple number to define the sequence: `1-spec-044-model.md`, `2-spec-044-executor-timeout.md`, `3-spec-044-processor-retry.md`. Approve them in order — dark-factory replaces the prefix with execution numbers. Do NOT number standalone prompts.

## Prompt Structure

### Frontmatter

```yaml
---
status: draft
---
```

When linking to a spec:

```yaml
---
spec: ["030-foo-bar-baz"]
status: draft
created: "2026-03-11T10:00:00Z"
---
```

- `spec` must be a YAML array of strings — single or multiple entries
- **Canonical form is the full slug** (e.g. `["030-foo-bar-baz"]`), not the bare number. Daemon-generated prompts use this form, and `pkg/slugmigrator` rewrites bare numbers to full slugs after each generation cycle so all prompts converge on the canonical form
- Bare numbers (`spec: ["030"]`) are accepted as input — the slug migrator resolves them to the full form at the next daemon iteration
- Only use `spec`, `status`, `created`, `issue` — dark-factory adds the rest
- Valid inbox statuses: `idea` (rough concept, needs refinement) or `draft` (complete, ready for approval)

### Body

```xml
<summary>
5-10 bullet points describing WHAT this prompt achieves, not HOW.
Written for human review — no file paths, no struct names.

GOOD:
- Projects can configure a validation command for all prompts
- The command's exit code determines success or failure
- Existing prompts continue to work unchanged

BAD:
- Adds `validationCommand` field to `Config` in `pkg/config/config.go`
- Calls `exec.Command` in `pkg/executor/executor.go`
</summary>

<objective>
What to build and why (1-3 sentences). State the end state, not the steps.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/config/config.go` — find the `Config` struct and `Validate` method.
</context>

<requirements>
1. Specific, numbered, unambiguous steps
2. Include exact file paths (repo-relative, not absolute)
3. Include function/type names as anchors (not line numbers)
4. Show old → new code patterns where helpful
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- [Copy constraints from spec — agent has no memory between prompts]
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
```

## Writing Rules

### Specificity over brevity — but pick the right *kind* of specificity

Longer, more specific prompts succeed more often *when the specificity is about contracts and anchors*, not when it's about pre-deciding every line of code the agent will write. Spelling out every if/else and every error message ships the author's bugs verbatim and prevents the agent from applying existing project conventions. See [Detail Levels](#detail-levels) below for how to pick the right grain.

Always include:
- Exact file paths (repo-relative)
- Function/type names as primary anchors
- Library import paths
- References to existing files demonstrating each pattern the new code follows

Optional (depends on detail level):
- Line numbers only as hints (`~line 176`) — go stale quickly
- Old → new code patterns for find-and-replace — useful for mechanical refactors, harmful for novel work
- Inlined code bodies — see Detail Levels for when this helps vs. hurts

### Detail Levels

Prompts span a spectrum from "fully scripted" to "intent only." Each level has real trade-offs. Pick deliberately; default to **Level 3 (Medium)** for most work in a mature codebase. Mismatched grain is the most common cause of prompt failure: too much detail ships author bugs, too little produces wrong abstractions.

| Level | Shape | Typical line count |
|-------|-------|-------------------|
| 1 — Very Detailed | Full inlined function bodies, every `if`/`else` written, error messages pre-decided | 500–1500 |
| 2 — Detailed | Function signatures + key error paths inlined; agent fills bodies | 200–500 |
| 3 — Medium *(default)* | Interfaces + sequence + pattern references + failure modes; no function bodies | 80–200 |
| 4 — Soft | End-state contracts + pattern references; agent chooses structure | 40–100 |
| 5 — Very Rough | One-paragraph intent + verification command | 10–40 |

#### Level 1 — Very Detailed (scripted)

**Shape:** Full Go source inlined for every function. Every conditional written out. Test cases enumerated by literal value.

**Pros:**
- Maximum reproducibility across model versions; works on cheap/weak models.
- The exact output is in the prompt — no agent interpretation needed.
- Good for teaching examples or documenting a canonical implementation.

**Cons:**
- The author's bugs ship verbatim (the agent faithfully implements the wrong `fmt.Errorf` instead of the project's `errors.Wrapf`).
- Drifts from project style because the prompt-author writes from memory, not from existing code.
- Audit becomes line-by-line code review of the prompt — slow and error-prone.
- The agent's pattern-matching strength is wasted.
- Any reusable knowledge inside the prompt rots and re-rots across every future prompt that needs the same pattern.

**When to use:**
- Brand-new pattern with no existing exemplar in the codebase.
- Absolute first prompt of a new project (no convention to follow yet).
- Reproducing a published reference implementation that should match an external source line-for-line.

#### Level 2 — Detailed (annotated skeleton)

**Shape:** Function signatures and key error paths spelled out. Imports listed. Bodies left to the agent with hints (`// retry once on transient errors per DB#8`).

**Pros:**
- Still very prescriptive about the contract surface.
- Manageable to audit (200–500 lines, not 1500).
- Less prone to style drift than Level 1 because bodies follow project conventions.

**Cons:**
- The author still owns most of the logic; bugs in the spelled-out error paths still ship.
- The skeleton can subtly bias the agent away from a cleaner structure it would have chosen.

**When to use:**
- Novel logic in a familiar project (the pattern doesn't exist yet, but the style does).
- Complex algorithms where the exact shape of the control flow matters.
- Migrating off a stable external API where the call sequence is fixed.

#### Level 3 — Medium (contracts + references) — **default**

**Shape:** Interface signatures, sequence of operations, pattern references (`see pkg/github/client.go for the errors.Wrapf style`), explicit failure modes copied from the spec, test scenarios named (`table-test the verdict × autoApprove matrix`). No function bodies. No pre-written error messages.

**Pros:**
- Best for translation work — applying an existing project pattern to a new feature.
- Leverages the agent's pattern-matching strength: it reads the referenced file and adopts the style automatically.
- Small enough to audit at a contract level instead of a line-by-line review.
- Prompts age well — the contract changes rarely, the implementation can be regenerated.
- Author bugs in inlined code can't ship because there is no inlined code.

**Cons:**
- Requires at least one good in-tree exemplar for each pattern referenced.
- Weaker models may produce non-uniform code across siblings without explicit shape.
- Reviewer must trust the contract rather than the implementation.

**When to use:**
- Most refactors and feature work in mature codebases (the default).
- Multi-prompt specs where consistency across siblings matters.
- Any case where existing code already demonstrates the conventions the new code should follow.

#### Level 4 — Soft (constraints + goals)

**Shape:** What must be true at the end, plus references and constraints. No interfaces enumerated. Agent decides sequence, structure, and decomposition within the file.

**Pros:**
- Maximum agent agency — it can refactor adjacent code if that produces a cleaner result.
- Very small prompts; fast to write.
- The agent may discover a better abstraction than the author would have specified.

**Cons:**
- Outcomes more variable across runs and model versions.
- Relies on agent capability and self-discipline; weaker models drift.
- Harder to review (less to compare against).
- Multi-prompt coordination breaks down — siblings may make incompatible decisions.

**When to use:**
- Greenfield work with a capable model.
- Rapid iteration when the right shape is uncertain.
- Cleanup tasks where the agent is expected to use judgment about how far to refactor.

#### Level 5 — Very Rough (intent)

**Shape:** One paragraph stating intent. A single verification command. No constraints, no references.

**Pros:**
- Trivial to write — minutes from idea to attempt.
- Forces the agent to do the design thinking.
- Useful as a "first draft" that can be refined into Level 3.

**Cons:**
- Highly variable output; often produces wrong abstractions on first try.
- Usually needs multiple rounds with edits — total wall time is often longer than starting at Level 3.
- Almost never satisfies the audit DoD on first pass.
- No reproducibility — same prompt twice produces different code.

**When to use:**
- Spike / exploration where the answer isn't yet known.
- Throwaway scripts.
- Rapidly evolving requirements where any specification will be out of date by the time it's written.

### Choosing a level

**The Level 3 default assumes documented patterns actually exist in the codebase.** In a messy or greenfield codebase with no stable exemplars, Level 3 is dishonest — the prompt promises "see X for the style" but X doesn't exist, and the agent silently slides back into inlining. If patterns aren't there yet, Level 2 (spelled-out signatures, hinted bodies) is more honest until a pattern is proven, then promote to Level 3.

**Step 0 — Discover existing patterns before writing.** Spend ~2 minutes running searches like:

```bash
# Error-wrapping style in this project
rg -l 'errors\.Wrapf|fmt\.Errorf' pkg/ internal/ | head

# HTTP client construction
rg -l 'http\.Client|net/http' pkg/ | head

# Counterfeiter mock pattern
rg -l 'counterfeiter:generate' pkg/ | head

# Test framework
rg -l 'ginkgo\.|gomega\.|testing\.T' pkg/ | head
```

If a surface returns 5+ matches with a consistent pattern → Level 3 references work. If it returns 0–2 → that pattern is novel for this codebase; choose Level 2 or document the new convention in a project doc first.

**Then ask in order:**

1. **Did Step 0 find ≥5 matches with a consistent pattern for the surface you're touching?**
   - Yes → continue to question 2.
   - No → **patterns are missing or inconsistent.** Continue to question 4 (do NOT silently fall through to Level 3 — that's the trap).
2. **Is this a published external API that must match line-for-line?**
   - Yes → Level 1 (and link the external source).
   - No → **Level 3** (reference the exemplars from Step 0; don't re-inline). This is the common case for translation work in mature codebases.
3. *(unreachable from question 1; kept for symmetry with prior versions)*
4. **(Patterns missing.) Will the agent need to invent a novel structure?**
   - Yes → Level 4 or 5 (let it explore), then promote to Level 3 once the pattern is proven AND document it in `project/docs/`.
   - No (translation work but no exemplar yet) → **Level 2** (spelled-out signatures, hinted bodies) — more honest than fake Level 3 references that don't exist. Promote to Level 3 in a future prompt once the pattern is documented.

**The "fall-through-to-Level-3" trap:** the most common mistake. Author runs no searches, picks Level 3 because it's the "default", references files that don't exist or don't demonstrate the pattern claimed. Agent silently inlines (because it has nothing to reference), and the prompt produces author-style code anyway — back to the original problem. Step 0 + question 1's hard split prevents this.

### What the spectrum does NOT solve

Pattern-anchoring (Level 3) solves **convention-drift bugs** — the agent adopts the project's existing style automatically. It does **not** solve **logic bugs in genuinely novel code**. Examples that no exemplar would catch:

- `io.LimitReader(resp.Body, 500)` placed BEFORE `json.Unmarshal` — truncates the body before parsing, but no existing file demonstrates the right ordering because no existing file has this exact concern.
- A retry classifier that maps "POST returned 200 but review absent in GET" to the wrong error class — the logic itself is wrong, not the style.
- An off-by-one in pagination, an inverted boolean, a swapped argument order.

These need **test cases the agent must satisfy** (spec acceptance criteria, failure-modes table, scenario tests) or **adversarial review** — they cannot be prevented by referring to exemplars. The detail spectrum reduces *one class* of bug; the other class needs spec-level rigor.

### Worked example: bot-identity self-check at three levels

Same surface (verify the GitHub bot account matches `BOT_GITHUB_LOGIN` before posting), three grain levels:

**Level 1 — Very Detailed**

```text
## Step 4 — Implement BotIdentityCheck

Create function `func CheckBotIdentity(ctx context.Context, client HTTPClient, expectedLogin string) error`:

1. Build request:
   req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
   if err != nil {
       return fmt.Errorf("build request: %w", err)
   }
   req.Header.Set("Authorization", "token "+token)
   req.Header.Set("Accept", "application/vnd.github+json")

2. Execute request:
   resp, err := client.Do(req)
   if err != nil {
       return fmt.Errorf("execute request: %w", err)
   }
   defer resp.Body.Close()

[... 60 more lines of fully-written Go ...]
```

**Cost:** The author wrote `fmt.Errorf` from memory. The project uses `errors.Wrapf(ctx, err, ...)`. The agent will faithfully ship `fmt.Errorf` — every test passes; `make precommit` lint fails or worse, the convention drifts silently.

**Level 3 — Medium (default)**

```text
## Bot identity check (DB#4 in spec 027)

Contract:
- Function: CheckBotIdentity(ctx, client HTTPClient, expectedLogin string) error
- Returns nil iff GitHub's GET /user response's `login` field equals expectedLogin
- Returns errors.Wrapf(ctx, ErrBotIdentityMismatch, "got %q, want %q", actual, expectedLogin) on mismatch
- Returns wrapped error on HTTP / parse failure per the retry-policy classification

Pattern references (read before writing):
- `pkg/github/client.go` for HTTP+errors.Wrapf style and Authorization header construction
- `pkg/githubauth/types.go` for the HTTPClient interface shape and Counterfeiter annotation
- `pkg/verdict.go` for sentinel-error pattern (see `ErrUnknownVerdict`)

Test (use DescribeTable):
- happy: 200 + login=expectedLogin → nil
- mismatch: 200 + login=other → wrapped ErrBotIdentityMismatch
- transient: network error → wrapped per pkg/github/client.go classification
- malformed: non-JSON response → wrapped, treated as permanent
```

**Cost:** None of the inlined bugs are possible — the agent reads `pkg/github/client.go` and adopts the existing `errors.Wrapf` style, header construction, and error classification. ~25 lines of prompt vs ~70.

**Level 5 — Very Rough**

```text
## Bot identity check

The agent self-checks that the GitHub PAT belongs to `my-bot-account`
before posting any review. Mismatch → refuse + diagnostic.

verification: make precommit
```

**Cost:** The agent might invent the function in the wrong package, miss the retry-policy integration, or choose a different error sentinel. Works for spike/exploration; needs rewrite for production.

When in doubt, write **Level 3** and let audit suggest tightening to Level 2 if the agent would be likely to drift.

### State the why, not just the what

`<objective>` should answer *why this change matters* in one clause, not just what to build. Models reason better when they know the purpose — same effort, fewer wrong abstractions.

```
BAD:  Add a validation command field to project config.
GOOD: Add a validation command field to project config so projects can enforce repo-specific checks (e.g. `make precommit`) before a prompt is marked complete.
```

The "why" also constrains the agent: if it considers a shortcut that breaks the stated purpose, it has a reason to reject it.

### Title comes from the first body H1

The commit subject for a prompt is derived from the first markdown `# ` heading in
the body (falling back to the filename when there is none). `#`-prefixed lines
inside fenced code blocks (`` ``` ```` or `~~~`) are ignored, so a YAML or shell
comment quoted in a code example will never become the commit subject. To control
the commit subject explicitly, put a real `# Your Title` line at the very top of the
body, above any fenced block.

### Anchor by name, not line number

Line numbers go stale when prior prompts edit the same file:

```
BAD:  "At line 231, wrap the error"
GOOD: "In ProcessQueue, after the autoSetQueuedStatus call, wrap the error"
```

### Copy constraints from spec

The agent has no memory between prompts. Every prompt must repeat the relevant constraints from the spec.

### One concern per prompt

Smaller prompts succeed more often. Group coupled behaviors together, but don't mix unrelated changes.

### Repo-relative paths only

Dark-factory runs prompts in a Docker container. Never use absolute paths (`/Users/...`) or home-relative paths (`~/...`).

**Exception**: Files under the mounted claude config use the in-container path `/home/node/.claude/plugins/...`, NOT the host path `~/.claude-yolo/plugins/...`. See [yolo-container-setup.md](../yolo-container-setup.md#project-workspace-mount).

### Cross-repo references

If a prompt needs to reference code in a sibling repo (e.g., a library at `~/Documents/workspaces/time/`), the file is NOT accessible inside the container. Options:

1. **Point to the vendored copy** — `vendor/github.com/bborbe/time/time_parse-time.go` (preferred when the dep is already vendored).
2. **Add an `extraMount`** in `.dark-factory.yaml` to mount the sibling repo.
3. **Inline the relevant snippet** in the prompt body.

Never leave a host-absolute path hoping the agent will figure it out — the agent will get a "file not found" and fail.

### Test the boundaries the new code crosses

A broad class of bug: new code passes every unit test because the tests verify the **shape** of what was added, but fails at runtime because it crosses a **boundary** (library validator, parser, dispatch registry, serialization, subprocess, external service) and that boundary imposes a constraint the shape tests never exercise.

**Rule:** For every boundary the new code crosses, at least one test must traverse that boundary with the new value. Ask: *what happens to this value after it leaves the code I just wrote?*

**Root cause framing:** the problem is *missing integration tests* — tests that exercise the same code path production traffic takes. The "validator test" pattern below is a cheap, mechanically-enforceable subset.

**Common boundaries and their contracts:**

- **Library validators / parsers** — `Validate()`, `Parse()`, `Check()`, `MustX()` enforce regex / schema / range / format at runtime. The Go compiler does not check the underlying string.
- **Registries and dispatch tables** — adding a handler / operation / route is not enough; the dispatcher must find it via the production lookup path.
- **Serialization round-trips** — JSON / YAML / protobuf tags, nested types, zero-value handling. A round-trip test is the contract.
- **External service contracts** — Kafka operation names, Prometheus label regex, DNS labels, URL schemes, HTTP route patterns.
- **Subprocess interfaces** — argv, env vars, stdin/stdout shape, exit codes. A wrong flag name silently fails.
- **Build-time constraints** — build tags, `go:generate` directives, struct tags read by code generators.

**Two acceptable levels:**

1. **Unit-level contract test** (cheap; use this when a single-function validator/parser exists): call the boundary function directly on the new value — `Validate(ctx)`, `Parse(...)`, marshal+unmarshal. For grouped values of the same library type, a table test enumerating all values in the package is the canonical shape.

   ```go
   // IMPORTANT: every new <TypeName> below MUST also appear in the Validate-all
   // test table in <matching_test.go>. The boundary contract is only enforced
   // at runtime — the test is the only pre-deploy guard.
   ```

2. **Integration test through the real boundary** (more thorough; use this when no single-function validator exists, or when the change introduces a new integration seam — publish path, registry entry, new CLI flag): drive the new value through the real production path in a test harness.

**Scope note:** E2E deployment verification (dev deploy + smoke tests) is a spec/scenario concern, not a prompt concern. Prompts cover unit and integration; specs cover acceptance; scenarios cover end-to-end.

**What does NOT satisfy this rule:**

- Struct equality tests (`Expect(cmd.TaskIdentifier).To(Equal("foo"))`)
- Accessor-default tests (`Expect(fm.TriggerCount()).To(Equal(0))`)
- Constant-value tests (`Expect(string(OpX)).To(Equal("op-x"))`) — these assert what you typed, not what the boundary accepts.

**Why this rule exists:** in a real incident, `IncrementFrontmatterCommandOperation = "increment_frontmatter"` (underscores) passed every unit test but was rejected at runtime by the cqrs validator's regex `^[a-z][a-z-]*$`, causing a silent message-retry loop in dev that took a deploy cycle to diagnose. A six-line table test calling `.Validate(ctx)` on each declared operation would have caught it at `make precommit`.

### Never run `go mod vendor` in a prompt

`vendor/` is a build-time artifact: gitignored, regenerated by `make buca` before `docker build`. `make precommit`'s `ensure` target wipes it and uses `-mod=mod`. Dark-factory doesn't build Docker images, so nothing it does needs `vendor/`.

Dep bump: `go get foo@v1.2.3 && go mod tidy` — that's it.

Exception: if the repo commits `vendor/`, document that deviation in the prompt.

### Add imports before tidying

`go mod tidy` (and `make ensure`, which calls it) removes any direct entry from `go.mod` that no code currently imports. A prompt step that does `go get foo@v1.2.3 && go mod tidy` BEFORE the file that imports `foo` is written will silently demote/remove the dep — the next `make precommit` then fails on the unresolved import.

```
BAD:  Step 1: go get foo && go mod tidy
      Step 2: write code that imports foo
      → Step 1 left go.mod unchanged because nothing imported foo yet
GOOD: Step 1: write code that imports foo
      Step 2: go mod tidy (promotes foo to a direct dep)
```

For prompts that add a new dependency: state the order explicitly — write the import first, then tidy.

### `make ensure` vs `make precommit` mid-implementation

When a prompt says "add a new dependency" or "regenerate vendored code", the intermediate state cannot pass `make precommit` (test+check fail until the code is updated). Use `make ensure` for dependency-only preparation steps, then reserve `make precommit` for final verification.

```
BAD: Step 1: "Run `make precommit` to add the dep."   # fails — tests reference code not yet written
GOOD: Step 1: "Run `make ensure` to add the dep."     # deps only
      Step N (last): "Run `make precommit`."          # full check at the end
```

## Decomposition

When breaking a spec into multiple prompts:

**The grouping test:** "Can these behaviors be verified independently?" If not, group them.

**The verifiability test:** "Can I write a test that distinguishes before vs after?" If not, merge or sharpen.

**Sequencing:** Most foundational prompt first. Each prompt's postconditions become the next prompt's preconditions.

| Feature size | Typical prompts |
|--------------|----------------|
| Config change | 1 |
| Single feature | 2-3 |
| Major feature | 4-6 |
| Full project bootstrap | 8-15 |

## Prompt Definition of Done

A prompt is ready for approval when ALL checks pass:

**Structure:**
- [ ] All sections present: `<summary>`, `<objective>`, `<context>`, `<requirements>`, `<constraints>`, `<verification>`
- [ ] `<summary>` has 5-10 plain-language bullets (no file paths or jargon)
- [ ] `<objective>` states end state in 1-3 sentences
- [ ] `<requirements>` are numbered, specific, unambiguous
- [ ] `<constraints>` copied from spec
- [ ] `<verification>` has a runnable command

**Accuracy:**
- [ ] File paths exist and are correct
- [ ] Function names and signatures match current code
- [ ] No stale references to renamed/moved/deleted code
- [ ] All paths are repo-relative

**Quality:**
- [ ] Independently verifiable
- [ ] Libraries specified with import paths
- [ ] No duplicates with completed prompts
- [ ] Every boundary the new code crosses (library validator, parser, registry, serialization, subprocess, external service) has a test that traverses it with the new value (see "Test the boundaries the new code crosses" above)

## Audit and Approve

Always audit before approving:

```
/dark-factory:audit-prompt prompts/my-change.md
```

Then approve via CLI (never manually edit frontmatter):

```bash
dark-factory prompt approve my-change
```

This moves the prompt from `prompts/` to `prompts/in-progress/`, assigns a number, and sets `status: queued`.

**Direct workflow:** Can approve multiple prompts at once — they execute sequentially on the same branch.

**PR workflow:** Approve one at a time — each depends on the previous being merged.

## Prompt Status Lifecycle

| Status | Directory | Meaning | How it happens |
|--------|-----------|---------|----------------|
| `idea` | `prompts/ideas/` | Rough concept, needs refinement | Human creates file |
| `draft` | `prompts/` | Complete, ready for review and approval | Human/AI creates file |
| `approved` | `prompts/in-progress/` | Queued for execution | `dark-factory prompt approve` |
| `executing` | `prompts/in-progress/` | YOLO container running | Auto (dark-factory) |
| `committing` | `prompts/in-progress/` | Container succeeded, git commit pending | Auto (dark-factory) |
| `completed` | `prompts/completed/` | Done, archived | Auto (dark-factory) |
| `failed` | `prompts/in-progress/` | Needs fix or retry | Auto (dark-factory) |
| `cancelled` | `prompts/in-progress/` | Pulled before/during execution | `dark-factory prompt cancel <id>` |

Completed prompts are immutable. If behavior changes later, create a new prompt.

### Fixing an Approved Prompt

To edit a prompt that has already been approved (or is executing) without recreating it:

```bash
dark-factory prompt cancel <id>     # sets status: cancelled
# edit the .md file to fix issues
dark-factory prompt requeue <id>    # sets status: queued, re-enters the execution queue
```

Note: `unapprove` does NOT work on `cancelled` prompts (error: "only approved prompts can be unapproved"). Use `requeue`.

Never manually edit the `status:` frontmatter — always use the CLI commands.

## Next Steps

- Run the pipeline: [running.md](../running.md)
