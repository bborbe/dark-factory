---
status: completed
container: dark-factory-012-fix-renumber-includes-completed
---



# Fix: NormalizeFilenames must include completed/ numbers when assigning new numbers

## Bug

`prompt.NormalizeFilenames()` only scans `prompts/` for already-used numbers.
It does not scan `prompts/completed/`. So when assigning a number to an
unnumbered file, it picks the lowest free number starting from 001 — but
001-011 already exist in `completed/`.

Result: `fix-double-commit.md` was renamed to `001-fix-double-commit.md`
instead of `012-fix-double-commit.md`.

## Fix

In `pkg/prompt/prompt.go`, update `NormalizeFilenames()` to also scan the
`completed/` subdirectory when collecting used numbers:

```go
// collect used numbers from prompts/ root
usedNumbers := collectUsedNumbers(dir)

// also collect from completed/
completedDir := filepath.Join(dir, "completed")
for n := range collectUsedNumbers(completedDir) {
    usedNumbers[n] = true
}

// now assign next free number above all used ones
```

The next free number must be higher than the maximum used number in both
`prompts/` and `prompts/completed/`.

## Constraints

- Run `make precommit` for validation only — do NOT commit, tag, or push (dark-factory handles all git operations)
