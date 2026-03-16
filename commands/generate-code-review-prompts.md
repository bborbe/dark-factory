---
description: Generate code-review prompts for services that produce fix prompts when executed
argument-hint: "[service-path|group|<empty for all>]"
allowed-tools: [Read, Write, Glob, Grep, Bash, AskUserQuestion]
---

Generate dark-factory prompts that, when executed by YOLO, review services and output fix prompts.

## Step 1: Discover Services

A **service** is a directory containing a `go.mod` file (services live at varying depths).

Parse `$ARGUMENTS` to determine scope:

**No argument** → find ALL services:
```bash
find . -name "go.mod" -not -path "./vendor/*" | sed 's|/go.mod||; s|^\./||' | sort
```

**Prefix argument** (e.g., `core`, `core/candle`) → find services under that prefix:
```bash
find ./$ARGUMENTS -name "go.mod" -not -path "./vendor/*" | sed 's|/go.mod||; s|^\./||' | sort
```
This works for both groups (`core` → all core services) and sub-groups (`core/candle` → all candle services).

**Exact service path** (e.g., `core/closing`) → if `$ARGUMENTS/go.mod` exists, single service.

The distinction: if `$ARGUMENTS/go.mod` exists, treat as single service. Otherwise, search for `go.mod` files under the prefix.

If no services found, error: "No go.mod found under $ARGUMENTS"

Store the list of service paths.

## Step 2: Read Config and Context

**Read `.dark-factory.yaml`** to determine the prompts inbox directory:
```yaml
prompts:
  inboxDir: prompts   # default if not set
```
Store as `$INBOX_DIR`. If file doesn't exist or `inboxDir` is not set, default to `prompts`.

Read once (shared across all prompts):
- Project's `CLAUDE.md` — conventions, patterns, design decisions
- Project's `docs/dod.md` — Definition of Done criteria (if exists)
- 3-5 recent completed prompts from `$INBOX_DIR/completed/` (highest-numbered) — to match style

## Step 3: Generate One Prompt Per Service

For each service path, derive a slug (replace `/` with `-`, e.g., `core/worker` → `core-worker`).

Scan the service files using the Glob tool with pattern `**/*.go` and path set to the service directory. Also try `*.go` at the service root (files may not be in subdirectories).

List the discovered `.go` files for use in the prompt's context section. If no `.go` files found, skip this service with a warning.

Write a prompt file to `$INBOX_DIR/code-review-<slug>.md` using the template below.

### Prompt Template

```markdown
---
status: created
created: "<current UTC timestamp in ISO8601>"
---

<summary>
- Service code reviewed against project coding standards and Definition of Done
- Fix prompts generated for each actionable finding
- Each fix prompt is independently verifiable and scoped to one concern
- No code changes made — review-only prompt that produces fix prompts
- Clean services produce no fix prompts
</summary>

<objective>
Review <service-path> against project coding guidelines and generate a fix prompt in /workspace/prompts/ for each actionable finding.
</objective>

<context>
Read `CLAUDE.md` for project conventions and key design decisions.
Read `docs/dod.md` for Definition of Done criteria.

Read 3-5 recent completed prompts from the prompts completed directory (highest-numbered) to understand prompt style and XML tag structure.

Service files to review:
[list key .go files discovered for this service]
</context>

<requirements>

## 1. Read Config and Guidelines

Read `.dark-factory.yaml` to find `prompts.inboxDir` (default: `prompts`). Use this as the output directory for fix prompts.
Read `CLAUDE.md` and `docs/dod.md` before reviewing.

## 2. Read Service Code

Read all Go files in `<service-path>/`. Understand the structure, types, test coverage.

## 3. Review Against Criteria

Apply the review criteria from `CLAUDE.md` and `docs/dod.md`. Classify each finding by severity:

**Must Fix (Critical)** — generate a fix prompt:
- Business logic in factory functions (conditionals, I/O, context.Background())
- Package-level function calls from business logic (should use injected interface)
- Missing error context wrapping
- Architectural violations (wrong package, wrong layer)

**Should Fix (Important)** — generate a fix prompt:
- Constructor returns concrete type instead of interface
- Missing counterfeiter mock annotations on interfaces
- Missing tests for packages with logic
- Inline HTTP handlers that belong in handler package

**Nice to Have** — do NOT generate prompts, skip these:
- GoDoc improvements
- Naming convention tweaks
- Minor style issues

## 4. Generate Fix Prompts

For each Critical or Important finding (or group of related findings), write a prompt file to the prompts inbox directory (from `.dark-factory.yaml`, default `/workspace/prompts/`). Skip Nice to Have findings — mention them in the summary but do not create prompts.

**Filename:** `review-<slug>-<fix-description>.md`

Each prompt must follow this exact format:

```
---
status: created
created: "<current UTC timestamp in ISO8601>"
---

<summary>
5-10 plain-language bullets. No file paths, struct names, or function signatures.
</summary>

<objective>
What to fix and why (1-3 sentences). End state, not steps.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- list specific files with descriptions
</context>

<requirements>
Numbered, specific, unambiguous steps.
Anchor by function/type name (~line N as hint only).
Include function signatures where helpful.
</requirements>

<constraints>
- Only change files in `<service-path>/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Run `make precommit` in `<service-path>/` only (NOT at repo root)
</constraints>

<verification>
cd <service-path> && make precommit
</verification>
```

## 5. Summary

Print a summary table of findings and generated prompt files.

</requirements>

<constraints>
- Do NOT modify any source code — this is a review-only prompt
- Only write files to the prompts inbox directory (from `.dark-factory.yaml`, default `/workspace/prompts/`)
- Never write to `in-progress/` or `completed/` subdirectories
- Never number prompt filenames — dark-factory assigns numbers on approve
- One concern per generated prompt (group coupled findings, split unrelated ones)
- Repo-relative paths only in generated prompts (no absolute, no `~/`)
- If no findings → report clean bill of health, generate no prompts
- Sequence prompts so foundational changes come first (if order matters, prefix with `1-`, `2-`, `3-`)
</constraints>

<verification>
ls /workspace/prompts/code-review-<slug>*.md 2>/dev/null
</verification>
```

Replace `<service-path>` and `<slug>` with actual values throughout.

## Step 4: Report

Tell the user:
- How many services discovered
- List of prompt files created
- Remind: audit with `/dark-factory:audit-prompt` then approve with `dark-factory prompt approve`
