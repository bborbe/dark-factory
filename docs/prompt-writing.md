# Prompt Writing Guide

A prompt is a markdown file that tells the YOLO agent what to build. Each prompt produces one atomic, verifiable change.

## When to Write Prompts Manually

- **Simple standalone task** (1-2 prompts, no spec needed) — write directly
- **Spec exists** — daemon auto-generates prompts from approved specs, but you can also write them manually for finer control

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
spec: ["030"]
status: draft
created: "2026-03-11T10:00:00Z"
---
```

- `spec` must be a YAML array: `spec: ["030"]` not `spec: "030"`
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

### Specificity over brevity

Longer, more specific prompts succeed more often. Include:
- Exact file paths (repo-relative)
- Function/type names as primary anchors
- Line numbers only as optional hints (`~line 176`)
- Old → new code patterns for find-and-replace
- Library import paths

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

**Exception**: Files under the mounted claude config use the in-container path `~/.claude/plugins/...`, NOT the host path `~/.claude-yolo/plugins/...`. See [yolo-container-setup.md](yolo-container-setup.md#project-workspace-mount).

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

Exception: if the repo commits `vendor/`, document that deviation in the prompt. See `go-build-args-guide.md#vendor-handling`.

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

- Run the pipeline: [running.md](running.md)
