# Add pre-execution validation in processor

## Goal

The processor should validate a prompt before executing it. Currently it picks up any file with `status: queued` — even if the filename has no number prefix, or the previous prompt hasn't completed yet. This caused `split-watcher-processor.md` to be processed without a sequence number.

## Validation Rules

Before executing a prompt, the processor must verify ALL of:

1. **Correct filename format**: matches `NNN-name.md` pattern (3+ digit prefix)
   - Skip files without number prefix (watcher hasn't renamed yet)
2. **Correct status**: frontmatter has `status: queued`
3. **No other prompt executing**: no file with `status: executing` exists
4. **Previous prompt completed**: all prompts with lower numbers are in `completed/` or `status: completed`
5. **Frontmatter present**: file has valid YAML frontmatter

If any check fails, the processor should skip the file and log why:
```
dark-factory: skipping foo.md: missing number prefix (waiting for watcher)
dark-factory: skipping 030-bar.md: prompt 029 not yet completed
dark-factory: skipping 031-baz.md: another prompt is executing
```

## Implementation

### 1. Add validation to processor

Create a `validate(ctx, prompt) error` method on the processor that checks all rules before execution.

### 2. Validation checks

```go
// ValidateForExecution checks if a prompt is ready to execute.
func (p *processor) validateForExecution(ctx context.Context, prompt Prompt) error {
    // 1. Check filename format
    if !hasNumberPrefix(prompt.Filename()) {
        return errors.Errorf(ctx, "missing number prefix")
    }

    // 2. Check no other prompt executing
    if p.promptManager.HasExecuting(ctx) {
        return errors.Errorf(ctx, "another prompt is executing")
    }

    // 3. Check all lower-numbered prompts completed
    if !p.promptManager.AllPreviousCompleted(ctx, prompt.Number()) {
        return errors.Errorf(ctx, "previous prompt not completed")
    }

    return nil
}
```

### 3. Add `AllPreviousCompleted` to prompt Manager interface

```go
// AllPreviousCompleted returns true if all prompts with numbers < n are in completed/.
AllPreviousCompleted(ctx context.Context, n int) bool
```

### 4. Processor loop handles validation

```go
for _, prompt := range queued {
    if err := p.validateForExecution(ctx, prompt); err != nil {
        log.Printf("dark-factory: skipping %s: %v", prompt.Filename(), err)
        continue
    }
    // execute prompt
}
```

### 5. Tests

- Prompt without number prefix is skipped
- Prompt with executing sibling is skipped
- Prompt with incomplete predecessor is skipped
- Valid prompt passes all checks
- All previous completed → returns true
- Gap in numbering (e.g., 028, 030 but no 029) → blocks 030

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Follow `~/.claude-yolo/docs/go-patterns.md`
- Follow `~/.claude-yolo/docs/go-precommit.md` (linter limits, fix targeted)
- Coverage ≥80% for changed packages
