---
status: completed
container: dark-factory-022-codebase-cleanup
---



# Codebase cleanup: fix all existing code quality issues

## Goal

Run a full code review of the entire codebase and fix all critical and important findings. This is a cleanup pass — no new features, no behavior changes. After this, future feature prompts produce clean diffs without pre-existing issues.

## Process

### 1. Read all YOLO coding guidelines

Read every doc in `/home/node/.claude/docs/go-*.md` before starting.

### 2. Review every Go file

For each `.go` file in `pkg/`, check against the self-review checklist:

**Composition** (`go-composition.md`):
- No `pkg.Function()` calls from business logic — use injected interfaces
- All deps visible in constructor params
- Interfaces are small (1-2 methods)

**Factory** (`go-factory-pattern.md`):
- Factory has zero business logic
- `Create*` prefix for factory functions
- Constructors return interfaces, not concrete types

**Patterns** (`go-patterns.md`):
- Public interface + private struct + `New*` constructor
- Counterfeiter annotations on ALL interfaces
- Errors wrapped with `errors.Wrap(ctx, err, "message")`

**Testing** (`go-testing.md`):
- Coverage ≥80% for every package
- Error paths tested
- Counterfeiter mocks only (never manual mocks)
- External test packages (`package_test`)
- Suite files with `//go:generate`

### 3. Fix all findings

Fix everything in priority order:

**Must fix:**
- Missing error wrapping
- Missing counterfeiter annotations
- Exported test helpers that should be unexported
- Missing test suite files
- Packages below 80% coverage
- Any `context.Background()` outside main.go/tests

**Should fix:**
- GoDoc comments on exported items
- Naming conventions (receivers, packages)
- Test quality (missing error path tests, missing edge cases)

**Skip:**
- Style-only changes with no functional impact
- Things that would change public API (breaking)

### 4. Verify

- `make test` after each package change
- `go test -cover ./pkg/...` to verify ≥80% coverage per package
- `make precommit` once at the end

## Known Issues to Check

These were identified in previous reviews:

1. **pkg/executor/executor.go**: `PrepareLogFile`, `CreatePromptTempFile`, `BuildDockerCommand` are exported but only used internally and in tests — consider if they should be unexported or moved
2. **Missing counterfeiter annotations**: Check every interface has `//counterfeiter:generate` comment
3. **Test coverage gaps**: Some packages may be below 80%
4. **Error messages**: Should be lowercase, no punctuation, wrapped with context

## Constraints

- Pure cleanup — NO behavior changes, NO new features
- Run `make precommit` for validation only — do NOT commit, tag, or push
- If unsure whether something is a bug or intentional, leave it alone
- Follow all docs in `/home/node/.claude/docs/go-*.md`
