# Add Validate() to Prompt and use in processor

## Goal

The Prompt struct should have a `Validate(ctx) error` method using `github.com/bborbe/validation`. The processor calls `Validate()` before executing, rejecting prompts that aren't ready.

This prevents the bug where `split-watcher-processor.md` was processed without a sequence number.

## Implementation

### 1. Add `Validate()` to Prompt struct

Follow `~/.claude-yolo/docs/go-validation.md` pattern:

```go
import "github.com/bborbe/validation"

func (p Prompt) Validate(ctx context.Context) error {
    return validation.All{
        validation.Name("path", validation.NotEmptyString(p.Path)),
        validation.Name("status", p.Status),  // Status.Validate() — see step 2
        validation.Name("filename", validation.HasValidationFunc(func(ctx context.Context) error {
            if !hasNumberPrefix(filepath.Base(p.Path)) {
                return errors.Errorf(ctx, "missing NNN- prefix: %s", filepath.Base(p.Path))
            }
            return nil
        })),
    }.Validate(ctx)
}
```

### 2. Make Status a domain type with Validate()

```go
type Status string

const (
    StatusQueued    Status = "queued"
    StatusExecuting Status = "executing"
    StatusCompleted Status = "completed"
    StatusFailed    Status = "failed"
)

func (s Status) Validate(ctx context.Context) error {
    for _, valid := range []Status{StatusQueued, StatusExecuting, StatusCompleted, StatusFailed} {
        if s == valid {
            return nil
        }
    }
    return errors.Wrapf(ctx, validation.Error, "status(%s) is invalid", s)
}
```

### 3. Add `ValidateForExecution()` to Prompt

Additional checks beyond basic validity — is this prompt ready to execute?

```go
func (p Prompt) ValidateForExecution(ctx context.Context) error {
    return validation.All{
        validation.Name("prompt", p),  // basic Validate()
        validation.Name("status", validation.HasValidationFunc(func(ctx context.Context) error {
            if p.Status != StatusQueued {
                return errors.Errorf(ctx, "expected status queued, got %s", p.Status)
            }
            return nil
        })),
    }.Validate(ctx)
}
```

### 4. Use in processor

```go
for _, prompt := range queued {
    if err := prompt.ValidateForExecution(ctx); err != nil {
        log.Printf("dark-factory: skipping %s: %v", filepath.Base(prompt.Path), err)
        continue
    }
    // execute prompt
}
```

### 5. Add `AllPreviousCompleted()` check to processor

The processor (not the prompt) checks ordering:
```go
if !p.promptManager.AllPreviousCompleted(ctx, prompt.Number()) {
    log.Printf("dark-factory: skipping %s: previous prompt not completed", ...)
    continue
}
```

Add to Manager interface:
```go
AllPreviousCompleted(ctx context.Context, n int) bool
```

### 6. Add dependency

```bash
go get github.com/bborbe/validation
```

### 7. Tests

- `Prompt.Validate()`: valid prompt passes, missing path fails, bad status fails, no number prefix fails
- `Status.Validate()`: all valid statuses pass, unknown status fails
- `Prompt.ValidateForExecution()`: queued passes, executing/completed/failed rejected
- Processor skips invalid prompts with log message
- Processor skips prompts with incomplete predecessors
- `AllPreviousCompleted()`: true when all lower numbers in completed/, false when gap exists

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Follow `~/.claude-yolo/docs/go-validation.md` for validation patterns
- Follow `~/.claude-yolo/docs/go-patterns.md` for interface/struct patterns
- Follow `~/.claude-yolo/docs/go-precommit.md` (linter limits, fix targeted)
- Coverage ≥80% for changed packages
