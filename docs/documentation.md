# Documentation in Dark Factory Projects

Knowledge lives in four locations. Each has a clear purpose. Putting knowledge in the wrong location causes bloated prompts, stale specs, and repeated mistakes.

## The Four Locations

| Location | Purpose | Lifespan | Audience |
|----------|---------|----------|----------|
| **Spec** | Behavioral contract — what/why/constraints | Dies after implementation | Prompt generator, human reviewer |
| **Prompt** | Implementation instructions — one-off details | Dies after execution | YOLO agent |
| **Project docs** (`project/docs/`) | Project-specific domain knowledge | Lives with the project | Specs, prompts, humans |
| **Yolo docs** (`~/.claude-yolo/docs/`) | Generic coding patterns | Lives across projects | All YOLO agents |

**Container mount paths:**
- `project/docs/` → `/workspace/docs` (repo is mounted at `/workspace`)
- `~/.claude-yolo/docs/` → `/home/node/.claude/docs`

### Spec

What the system should do, not how. Behavioral contract.

**Belongs here:**
- Problem statement, goal, desired behavior
- Constraints (what must not change)
- Failure modes and acceptance criteria
- References to project docs for domain context

**Does NOT belong here:**
- Code examples (unless frozen constraints)
- Library API details → reference yolo docs
- Domain rules that outlive the spec → move to project docs

### Prompt

Implementation instructions for one atomic change. Dies after execution.

**Belongs here:**
- Exact file paths to modify
- One-off code changes (add this env var, add this assertion)
- References to docs for patterns to follow
- Verification commands

**Does NOT belong here:**
- Reusable coding patterns (>10 lines) → reference yolo docs
- Domain knowledge reused across prompts → reference project docs
- Business rules that outlive the prompt → move to project docs

### Project docs (`project/docs/`)

Project-specific domain knowledge that multiple specs and prompts need. Survives beyond any single spec.

**Belongs here:**
- Kafka topic naming and schema design
- Task file format (frontmatter fields, markdown structure)
- Controller architecture (what talks to what, data flow)
- Agent job interface (env vars, lifecycle)
- Deployment topology (StatefulSet vs Deployment, volume mounts)

**Rules:**
- One concept per file
- Keep under 200 lines
- Repo-relative paths in examples
- Include enough detail that a prompt can say "follow `docs/X.md`" without repeating it

### Yolo docs (`~/.claude-yolo/docs/`)

Generic coding patterns (Go, Python, Git, etc.) from shared libraries. Apply across all projects.

**Belongs here:**
- CQRS command handler pattern (`go-cqrs.md`)
- Error wrapping with `bborbe/errors` (`go-error-wrapping.md`)
- Factory pattern (`go-factory-pattern.md`)
- BoltKV setup (`go-boltkv.md`)
- Ginkgo/Gomega test conventions (`go-testing.md`)

**Rules:**
- <=100 lines — every line costs tokens across every prompt
- Structure: Core Pattern → Rules → Good/Bad → Checklist
- Self-contained — no cross-references to docs outside the container
- Code > prose

## Decision Tree

```
Knowledge appears while writing a spec or prompt
  |
  v
Is it behavioral (what/why/constraints)?
  |-- Yes --> Spec
  |-- No  --> Is it a one-off implementation detail?
                |-- Yes --> Prompt (inline)
                |-- No  --> Will other specs/prompts need this?
                              |-- No  --> Prompt (inline)
                              |-- Yes --> Is it project-specific?
                                           |-- Yes --> project/docs/
                                           |-- No  --> ~/.claude-yolo/docs/
```

## The Lifecycle Rule

**Specs and prompts die. Docs live.**

A spec describes "controller consumes commands from Kafka." After implementation, the spec is archived. But the Kafka schema design, the CQRS handler pattern, and the BoltDB setup pattern are still needed by the next spec.

If knowledge will be needed after the spec/prompt is done → it belongs in docs, not inline.

## How to Reference Docs

### From specs

Specs stay behavioral. Reference project docs for domain context:

```markdown
## Constraints
- Consumer group naming follows `docs/kafka-schema-design.md`
- Task file format follows `docs/task-service-design.md`
```

### From prompts

Prompts reference both project docs and yolo docs in `<context>`:

```xml
<context>
Read CLAUDE.md for project conventions.
Read `docs/kafka-schema-design.md` — topic naming and SchemaID pattern.
Read `/home/node/.claude/docs/go-cqrs.md` — CommandObjectExecutorTxFunc pattern.
Read `/home/node/.claude/docs/go-boltkv.md` — BoltDB setup with ChangeOptions.

Key files to read before making changes:
- `pkg/factory/factory.go` — existing factory wiring
</context>
```

Note: `docs/` is repo-relative (mounted at `/workspace/docs`). Yolo docs use the absolute container path `/home/node/.claude/docs/`.

Then `<requirements>` says "follow the pattern from `go-cqrs.md`" instead of inlining 30 lines.

## What Belongs Where — Examples

| Knowledge | Spec | Prompt | Project docs | Yolo docs |
|-----------|------|--------|-------------|-----------|
| "Controller consumes commands" | ✓ | - | - | - |
| "Must not change event topic" | ✓ | ✓ (copy) | - | - |
| Kafka topic naming convention | ref | ref | ✓ | - |
| Task file frontmatter format | ref | ref | ✓ | - |
| CQRS CommandObjectExecutorTxFunc | - | ref | - | ✓ |
| BoltKV OpenDir + ChangeOptions | - | ref | - | ✓ |
| Ginkgo suite bootstrap | - | ref | - | ✓ |
| Factory pattern (zero logic) | - | ref | - | ✓ |
| Error wrapping convention | - | ref | - | ✓ |
| Add TASK_ID env var (one-off) | - | ✓ | - | - |
| Specific test assertion | - | ✓ | - | - |
| K8s StatefulSet volume layout | ref | ref | ✓ | - |

## Auditor Enforcement

### Prompt auditor checks

- **Inline pattern detection** — `<requirements>` contains >10 lines of a reusable pattern → flag: "Extract to doc, reference instead"
- **Missing doc reference** — prompt uses a library pattern but `<context>` doesn't reference the matching yolo doc → flag: "Add `go-X.md` to context"
- **Existing doc ignored** — `project/docs/` has a relevant doc but prompt doesn't mention it → flag: "Reference `docs/X.md` in context"

### Spec auditor checks

- **Undocumented domain knowledge** — spec describes business rules (file format, event flow, naming) not found in `project/docs/` → flag: "Create `docs/X.md` before prompting"
- **Implementation in spec** — spec contains code examples or struct definitions that aren't frozen constraints → flag: "Move to docs, reference from spec"

## Creating a New Doc

### Triggers

- YOLO gets a pattern wrong in 2+ prompts → yolo doc needed
- Spec describes domain logic not captured anywhere → project doc needed
- Prompt inlines >10 lines of reusable pattern → extract to doc
- Auditor flags missing doc reference → create the doc

### Process

1. Check if doc already exists (`project/docs/` or `~/.claude-yolo/docs/`)
2. Write the doc in the correct location
3. Update specs/prompts to reference instead of inline
4. For yolo docs: update `~/.claude-yolo/docs/README.md` index

## Anti-Patterns

**Inline everything** — 200-line prompt with CQRS wiring, BoltDB setup, and test bootstrap inlined. Breaks when library API changes. Prompt 3 of spec-005 had 4 bugs from inlined patterns.

**Wrong location** — project-specific Kafka topic naming in `~/.claude-yolo/docs/`. Now every project sees irrelevant detail.

**Doc exists but not referenced** — `go-cqrs.md` has the exact pattern, but prompt reinvents it differently. Agent follows prompt, not doc.

**Spec as documentation** — spec has 50 lines of API reference. Spec archives after implementation; that knowledge is lost. Should have been in `project/docs/`.

**Over-documenting** — creating a doc for a one-off pattern. If truly used once, inline is fine.

**Knowledge in prompt, not project docs** — prompt describes task file format in detail. Next prompt needs the same knowledge but the previous prompt is archived. Should be in `project/docs/`.
