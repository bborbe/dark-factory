---
description: Generate code-review prompts for services that produce fix prompts when executed
argument-hint: "[service-path|group|<empty for all>]"
allowed-tools: [Read, Write, Glob, Grep, Bash, AskUserQuestion]
---

Generate dark-factory prompts that, when executed by YOLO, review services using `/coding:code-review full` and output fix prompts for findings.

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

**Exact service path** (e.g., `core/closing`) → if `$ARGUMENTS/go.mod` exists, single service.

The distinction: if `$ARGUMENTS/go.mod` exists, treat as single service. Otherwise, search for `go.mod` files under the prefix.

If no services found, error: "No go.mod found under $ARGUMENTS"

Store the list of service paths.

## Step 2: Read Config

**Read `.dark-factory.yaml`** to determine the prompts inbox directory:
```yaml
prompts:
  inboxDir: prompts   # default if not set
```
Store as `$INBOX_DIR`. If file doesn't exist or `inboxDir` is not set, default to `prompts`.

## Step 3: Generate One Prompt Per Service

For each service path, derive a slug (replace `/` with `-`, e.g., `core/worker` → `core-worker`).

Scan the service files using Glob with pattern `**/*.go` and path set to the service directory. If no `.go` files found, skip this service with a warning.

Write a prompt file to `$INBOX_DIR/code-review-<slug>.md` using the template below.

### Prompt Template

```markdown
---
status: draft
created: "<current UTC timestamp in ISO8601>"
---

<summary>
- Service reviewed using full automated code review with all specialist agents
- Fix prompts generated for each Critical or Important finding
- Each fix prompt is independently verifiable and scoped to one concern
- No code changes made — review-only prompt that produces fix prompts
- Clean services produce no fix prompts
</summary>

<objective>
Run a full code review of <service-path> and generate a fix prompt for each Critical or Important finding.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/dod.md` for Definition of Done criteria (if exists).

Read 3 recent completed prompts from the prompts completed directory (highest-numbered) to understand prompt style and XML tag structure.

Service directory: `<service-path>/`
</context>

<requirements>

## 1. Read Config

Read `.dark-factory.yaml` to find `prompts.inboxDir` (default: `prompts`). Use this as the output directory for fix prompts.

## 2. Run Code Review

Run `/coding:code-review full <service-path>` to get a comprehensive review with all specialist agents.

Collect the consolidated findings categorized as:
- **Must Fix (Critical)** — will generate fix prompts
- **Should Fix (Important)** — will generate fix prompts
- **Nice to Have** — skip, do NOT generate prompts

## 3. Generate Fix Prompts

For each Critical or Important finding (or group of related findings in the same file/package), write a prompt file to the prompts inbox directory.

**Filename:** `review-<slug>-<fix-description>.md`

Each fix prompt must follow this exact structure:

```
---
status: draft
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
- list specific files with line numbers as hints
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
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
cd <service-path> && make precommit
</verification>
```

**Grouping rules:**
- One concern per prompt (e.g., "fix error wrapping in package X")
- Group coupled findings that must change together
- Split unrelated findings into separate prompts
- If order matters, prefix filenames with `1-`, `2-`, `3-`

## 4. Summary

Print a summary of findings and generated prompt files.

</requirements>

<constraints>
- Do NOT modify any source code — this is a review-only prompt
- Only write files to the prompts inbox directory
- Never write to `in-progress/` or `completed/` subdirectories
- Never number prompt filenames — dark-factory assigns numbers on approve
- Repo-relative paths only in generated prompts (no absolute, no `~/`)
- If no findings at Critical/Important level → report clean bill of health, generate no prompts
</constraints>

<verification>
After generating fix prompts, list them:
ls $INBOX_DIR/review-<slug>-*.md
</verification>
```

Replace `<service-path>` and `<slug>` with actual values throughout.

## Step 4: Report

Tell the user:
- How many services discovered
- How many had findings vs clean
- List of prompt files created
- Remind: audit with `/dark-factory:audit-prompt` then approve with `dark-factory prompt approve`
