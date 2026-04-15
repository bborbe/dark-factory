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
