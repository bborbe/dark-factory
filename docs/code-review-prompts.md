# Code Review Prompts

Generate dark-factory prompts that review services and produce fix prompts for findings.

## How It Works

```
/dark-factory:generate-code-review-prompts [scope]
        │
        ▼
  Discover services (go.mod)
        │
        ▼
  Generate one review prompt per service
        │
        ▼
  Daemon executes each review prompt:
    1. Runs /coding:code-review full <service>
    2. Collects Must Fix + Should Fix findings
    3. Writes fix prompts to inbox
        │
        ▼
  Daemon executes fix prompts → commits
```

Each review prompt delegates to `/coding:code-review full`, which runs 13+ specialist agents (quality, security, coverage, factory patterns, etc.). Findings are categorized as Critical/Important/Nice-to-Have. Only Critical and Important generate fix prompts.

## Usage

```bash
# Review ALL services in the repo
/dark-factory:generate-code-review-prompts

# Review services under a group
/dark-factory:generate-code-review-prompts core

# Review a sub-group
/dark-factory:generate-code-review-prompts core/candle

# Review a single service
/dark-factory:generate-code-review-prompts core/closing
```

## What Gets Generated

For a service `core/worker`, the command creates:

```
prompts/code-review-core-worker.md    ← review prompt (runs code-review)
```

When the daemon executes that prompt, it may produce:

```
prompts/review-core-worker-fix-error-wrapping.md
prompts/review-core-worker-add-missing-tests.md
prompts/review-core-worker-fix-factory-pattern.md
```

These fix prompts are then picked up by the daemon and executed sequentially.

## Workflow

1. Run the command to generate review prompts
2. Audit generated prompts: `/dark-factory:audit-prompt prompts/code-review-<slug>.md`
3. Approve: `dark-factory prompt approve prompts/code-review-<slug>.md`
4. Start daemon: `/dark-factory:daemon`
5. Daemon executes reviews, generates fix prompts, executes fixes

## Scale Considerations

- Each review prompt spawns 13+ agents and may generate multiple fix prompts
- For large repos (50+ services), scope to a group rather than all
- Fix prompts run sequentially, so a full-repo review can take hours
- Start with one service to validate findings quality before scaling up

## Finding Categories

The `/coding:code-review full` skill categorizes findings:

| Category | Action | Examples |
|----------|--------|---------|
| Must Fix (Critical) | Fix prompt generated | Security issues, context violations, bare `return err`, raw goroutines |
| Should Fix (Important) | Fix prompt generated | Missing tests, architectural violations, factory pattern issues |
| Nice to Have | Skipped | Style, naming, minor docs |

Review criteria come from the `/coding:code-review` skill — not hardcoded in the command. When the skill evolves, reviews automatically improve.
